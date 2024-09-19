package app

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/anmitsu/goful/cmdline"
	"github.com/anmitsu/goful/look"
	"github.com/anmitsu/goful/message"
	"github.com/anmitsu/goful/util"
	"github.com/anmitsu/goful/widget"
)

// match shell separators, macros, options and spaces.
var re = regexp.MustCompile(`([;|>&])|(%~?(?:[&mMfFxX]|[dD]2?))|([[:space:]]-[[:word:]-=]+)|[[:space:]]`)

// Shell starts the shell mode.
// The head of variadic arguments is used for cursor positioning.
func (g *Goful) Shell(cmd string, offset ...int) {
	commands, err := util.SearchCommands()
	if err != nil {
		message.Error(err)
	}
	c := cmdline.New(&shellMode{g, commands, false}, g)
	c.SetText(cmd)
	if len(offset) > 0 {
		c.MoveCursor(offset[0])
	}
	g.next = c
}

// ShellSuspend starts the shell mode and suspends screen after running.
// The head of variadic arguments is used for cursor positioning.
func (g *Goful) ShellSuspend(cmd string, offset ...int) {
	commands, err := util.SearchCommands()
	if err != nil {
		message.Error(err)
	}
	c := cmdline.New(&shellMode{g, commands, true}, g)
	c.SetText(cmd)
	if len(offset) > 0 {
		c.MoveCursor(offset[0])
	}
	g.next = c
}

type shellMode struct {
	*Goful
	commands map[string]bool
	suspend  bool
}

func (m *shellMode) String() string { return "shell" }
func (m *shellMode) Prompt() string {
	if m.suspend {
		return "Suspend(쉘실행후종료) $ "
	}
	return "$ "
}
func (m *shellMode) Draw(c *cmdline.Cmdline) {
	c.Clear()
	x, y := c.LeftTop()
	x = widget.SetCells(x, y, m.Prompt(), look.Prompt())
	widget.ShowCursor(x+c.Cursor(), y)
	m.drawCommand(x, y, c.String())
}

func (m *shellMode) drawCommand(x, y int, cmd string) {
	start := 0
	// match is index [start, end, sep_start, sep_end, macro_start, macro_end, opt_start, opt_end]
	for _, match := range re.FindAllStringSubmatchIndex(cmd, -1) {
		s := cmd[start:match[0]]
		if _, ok := m.commands[s]; ok { // as command
			x = widget.SetCells(x, y, s, look.CmdlineCommand())
		} else {
			x = widget.SetCells(x, y, s, look.Cmdline())
		}
		start = match[0]
		s = cmd[start:match[1]]
		if match[2] != -1 { // as shell separator ;|>&
			x = widget.SetCells(x, y, s, look.Cmdline())
		} else if match[4] != -1 { // as macro %& %m %M %f %F %x %X %d2 %D %d2 %D2
			x = widget.SetCells(x, y, s, look.CmdlineMacro())
		} else if match[6] != -1 { // as option -a --bcd-efg
			x = widget.SetCells(x, y, s, look.CmdlineOption())
		} else {
			x = widget.SetCells(x, y, s, look.Cmdline())
		}
		start = match[1]
	}
	// draw the rest
	s := cmd[start:]
	if _, ok := m.commands[s]; ok { // as command
		widget.SetCells(x, y, s, look.CmdlineCommand())
	} else {
		widget.SetCells(x, y, s, look.Cmdline())
	}
}

func (m *shellMode) Run(c *cmdline.Cmdline) {
	if m.suspend {
		m.SpawnSuspend(c.String())
	} else {
		m.Spawn(c.String())
	}
	m.commands = nil
	c.Exit()
}

func (g *Goful) dialog(message string, options ...string) string {
	g.interrupt <- 1
	defer func() { g.interrupt <- 1 }()

	tmp := g.Next()
	dialog := &dialogMode{message, options, ""}
	g.next = cmdline.New(dialog, g)

	for !widget.IsNil(g.Next()) {
		g.Draw()
		widget.Show()
		g.eventHandler(<-g.event)
	}
	g.next = tmp
	return dialog.result
}

type dialogMode struct {
	message string
	options []string
	result  string
}

func (m *dialogMode) String() string { return "dialog" }
func (m *dialogMode) Prompt() string {
	return fmt.Sprintf("%s [%s] ", m.message, strings.Join(m.options, "/"))
}
func (m *dialogMode) Draw(c *cmdline.Cmdline) { c.DrawLine() }
func (m *dialogMode) Run(c *cmdline.Cmdline) {
	for _, opt := range m.options {
		if c.String() == opt {
			m.result = opt
			c.Exit()
			return
		}
	}
	c.SetText("")
}

// Quit starts the quit mode.
func (g *Goful) Quit() {
	g.next = cmdline.New(&quitMode{g}, g)
}

type quitMode struct {
	*Goful
}

func (m quitMode) String() string          { return "quit" }
func (m quitMode) Prompt() string          { return "Quit? 종료? [Y/n] " }
func (m quitMode) Draw(c *cmdline.Cmdline) { c.DrawLine() }
func (m quitMode) Run(c *cmdline.Cmdline) {
	switch c.String() {
	case "Y", "y", "", "q":
		c.Exit()
		m.exit = true
	// case "n":
	default:
		c.Exit()
		// c.SetText("")
	}
}

// Copy starts the copy mode.
func (g *Goful) Copy() {
	c := cmdline.New(&copyMode{g, ""}, g)
	if g.Dir().IsMark() {
		c.SetText(g.Workspace().NextDir().Path)
	} else {
		c.SetText(g.File().Name())
	}
	g.next = c
}

type copyMode struct {
	*Goful
	src string
}

func (m *copyMode) String() string { return "copy" }
func (m *copyMode) Prompt() string {
	if m.Dir().IsMark() {
		return fmt.Sprintf("Copy(복사) %d files -> ", m.Dir().MarkCount())
	} else if m.src != "" {
		return fmt.Sprintf("Copy(복사) %s -> ", m.src)
	} else {
		return "Copy(복사) from "
	}
}
func (m *copyMode) Draw(c *cmdline.Cmdline) { c.DrawLine() }
func (m *copyMode) Run(c *cmdline.Cmdline) {
	if m.Dir().IsMark() {
		dst := c.String()
		src := m.Dir().MarkfilePaths()
		m.copy(dst, src...)
		c.Exit()
	} else if m.src != "" {
		dst := c.String()
		m.copy(dst, m.src)
		c.Exit()
	} else {
		m.src = c.String()
		c.SetText(m.Workspace().NextDir().Path)
	}
}

// Move starts the move mode.
func (g *Goful) Move() {
	c := cmdline.New(&moveMode{g, ""}, g)
	if g.Dir().IsMark() {
		c.SetText(g.Workspace().NextDir().Path)
	} else {
		c.SetText(g.File().Name())
	}
	g.next = c
}

type moveMode struct {
	*Goful
	src string
}

func (m *moveMode) String() string { return "move" }
func (m *moveMode) Prompt() string {
	if m.Dir().IsMark() {
		return fmt.Sprintf("Move(이동) %d files -> ", m.Dir().MarkCount())
	} else if m.src != "" {
		return fmt.Sprintf("Move(이동) %s -> ", m.src)
	} else {
		return "Move(이동) from "
	}
}
func (m *moveMode) Draw(c *cmdline.Cmdline) { c.DrawLine() }
func (m *moveMode) Run(c *cmdline.Cmdline) {
	if m.Dir().IsMark() {
		dst := c.String()
		src := m.Dir().MarkfilePaths()
		m.move(dst, src...)
		c.Exit()
	} else if m.src != "" {
		dst := c.String()
		m.move(dst, m.src)
		c.Exit()
	} else {
		m.src = c.String()
		c.SetText(m.Workspace().NextDir().Path)
	}
}

// Rename starts the rename mode.
func (g *Goful) Rename() {
	src := g.File().Name()
	c := cmdline.New(&renameMode{g, src}, g)
	c.SetText(src)
	c.MoveCursor(-len(filepath.Ext(src)))
	g.next = c
}

type renameMode struct {
	*Goful
	src string
}

func (m *renameMode) String() string          { return "rename" }
func (m *renameMode) Prompt() string          { return fmt.Sprintf("Rename(이름변경) %s -> ", m.src) }
func (m *renameMode) Draw(c *cmdline.Cmdline) { c.DrawLine() }
func (m *renameMode) Run(c *cmdline.Cmdline) {
	dst := c.String()
	if dst == "" {
		return
	}
	m.rename(m.src, dst)
	m.Workspace().ReloadAll()
	c.Exit()
}

// BulkRename starts the bulk rename mode.
func (g *Goful) BulkRename() {
	g.next = cmdline.New(&bulkRenameMode{g, ""}, g)
}

type bulkRenameMode struct {
	*Goful
	src string
}

func (m *bulkRenameMode) String() string          { return "bulkrename" }
func (m *bulkRenameMode) Prompt() string          { return "Rename by regexp(정규식 이름변경) %s/" }
func (m *bulkRenameMode) Draw(c *cmdline.Cmdline) { c.DrawLine() }
func (m *bulkRenameMode) Run(c *cmdline.Cmdline) {
	var pattern, repl string
	patterns := strings.Split(c.String(), "/")
	if len(patterns) > 1 {
		pattern = patterns[0]
		repl = patterns[1]
	} else {
		message.Errorf("Input must be like `regexp/replaced'")
		return
	}
	c.Exit()
	m.bulkRename(pattern, repl, m.Dir().Markfiles()...)
}

// Remove starts the remove mode.
func (g *Goful) Remove() {
	c := cmdline.New(&removeMode{g, ""}, g)
	if !g.Dir().IsMark() {
		c.SetText(g.File().Name())
	}
	g.next = c
}

type removeMode struct {
	*Goful
	src string
}

func (m *removeMode) String() string { return "remove" }
func (m *removeMode) Prompt() string {
	if m.Dir().IsMark() {
		return fmt.Sprintf("Remove permanently(완전삭제)? %d files [Y/n] ", m.Dir().MarkCount())
	} else if m.src != "" {
		return fmt.Sprintf("Remove permanently(완전삭제)? %s [Y/n] ", m.src)
	} else {
		return "Remove permanently(완전삭제): "
	}
}
func (m *removeMode) Draw(c *cmdline.Cmdline) { c.DrawLine() }
func (m *removeMode) Run(c *cmdline.Cmdline) {
	if marked := m.Dir().IsMark(); marked || m.src != "" {
		switch c.String() {
		case "y", "Y", "":
			if marked {
				m.remove(m.Dir().MarkfilePaths()...)
			} else {
				m.remove(m.src)
			}
			c.Exit()
		case "n", "N":
			c.Exit()
		default:
			c.SetText("")
		}
	} else {
		m.src = c.String()
		c.SetText("")
	}
}

// Mkdir starts the make directory mode.
func (g *Goful) Mkdir() {
	g.next = cmdline.New(&mkdirMode{g, ""}, g)
}

type mkdirMode struct {
	*Goful
	path string
}

func (m *mkdirMode) String() string { return "mkdir" }
func (m *mkdirMode) Prompt() string {
	if m.path != "" {
		return "Mode(권한) default 0755: "
	}
	return "Make directory(새폴더): "
}
func (m *mkdirMode) Draw(c *cmdline.Cmdline) { c.DrawLine() }
func (m *mkdirMode) Run(c *cmdline.Cmdline) {
	if m.path != "" {
		mode := c.String()
		if mode != "" {
			if mode, err := strconv.ParseUint(mode, 8, 32); err != nil {
				message.Error(err)
			} else if err := os.MkdirAll(m.path, os.FileMode(mode)); err != nil {
				message.Error(err)
			}
		} else {
			if err := os.MkdirAll(m.path, 0755); err != nil {
				message.Error(err)
			}
		}
		message.Info("Made directory(폴더만듬) " + m.path)
		c.Exit()
		m.Workspace().ReloadAll()
	} else {
		m.path = c.String()
		c.SetText("")
	}
}

// Touch starts the touch file mode.
func (g *Goful) Touch() {
	g.next = cmdline.New(&touchFileMode{g, ""}, g)
}

type touchFileMode struct {
	*Goful
	path string
}

func (m *touchFileMode) String() string { return "touchfile" }
func (m *touchFileMode) Prompt() string {
	if m.path != "" {
		return "Mode(권한) default 0755: "
	}
	return "Touch file(새파일): "
}
func (m *touchFileMode) Draw(c *cmdline.Cmdline) { c.DrawLine() }
func (m *touchFileMode) Run(c *cmdline.Cmdline) {
	if m.path != "" {
		mode := c.String()
		if mode != "" {
			if mode, err := strconv.ParseUint(mode, 8, 32); err != nil {
				message.Error(err)
			} else {
				m.touch(m.path, os.FileMode(mode))
			}
		} else {
			m.touch(m.path, 0644)
		}
		c.Exit()
		m.Workspace().ReloadAll()
	} else {
		m.path = c.String()
		c.SetText("")
	}
}

// Chmod starts the change mode mode.
func (g *Goful) Chmod() {
	c := cmdline.New(&chmodMode{g, nil}, g)
	if !g.Dir().IsMark() {
		c.SetText(g.File().Name())
	}
	g.next = c
}

type chmodMode struct {
	*Goful
	fi os.FileInfo
}

func (m *chmodMode) String() string { return "chmod" }
func (m *chmodMode) Prompt() string {
	if m.Dir().IsMark() {
		return fmt.Sprintf("Chmod(권한변경) %d files -> ", m.Dir().MarkCount())
	} else if m.fi != nil {
		return fmt.Sprintf("Chmod(권한변경) %s %o -> ", m.fi.Name(), m.fi.Mode())
	}
	return "Chmod(권한변경): "
}
func (m *chmodMode) Draw(c *cmdline.Cmdline) { c.DrawLine() }
func (m *chmodMode) Run(c *cmdline.Cmdline) {
	if m.Dir().IsMark() || m.fi != nil {
		mode, err := strconv.ParseUint(c.String(), 8, 32)
		if err != nil {
			message.Error(err)
			c.Exit()
			return
		}
		if m.fi != nil {
			m.chmod(os.FileMode(mode), m.fi.Name())
		} else {
			files := m.Dir().MarkfilePaths()
			m.chmod(os.FileMode(mode), files...)
		}
		c.Exit()
		m.Workspace().ReloadAll()
	} else {
		file := c.String()
		lstat, err := os.Lstat(file)
		if err != nil {
			message.Error(err)
			c.Exit()
			return
		}
		m.fi = lstat
		c.SetText("")
	}
}

// ChangeWorkspaceTitle starts the changing workspace title.
func (g *Goful) ChangeWorkspaceTitle() {
	g.next = cmdline.New(&changeWorkspaceTitle{g}, g)
}

type changeWorkspaceTitle struct {
	*Goful
}

func (m *changeWorkspaceTitle) String() string          { return "changeworkspacetitle" }
func (m *changeWorkspaceTitle) Prompt() string          { return "Change tab title(탭제목변경): " }
func (m *changeWorkspaceTitle) Draw(c *cmdline.Cmdline) { c.DrawLine() }
func (m *changeWorkspaceTitle) Run(c *cmdline.Cmdline) {
	title := c.String()
	if title != "" {
		m.Workspace().SetTitle(title)
	}
	c.Exit()
}

// Chdir starts the change directory mode.
func (g *Goful) Chdir() {
	g.next = cmdline.New(&chdirMode{g}, g)
}

type chdirMode struct {
	*Goful
}

func (m *chdirMode) String() string          { return "chdir" }
func (m *chdirMode) Prompt() string          { return "Chdir(경로변경) to " }
func (m *chdirMode) Draw(c *cmdline.Cmdline) { c.DrawLine() }
func (m *chdirMode) Run(c *cmdline.Cmdline) {
	if path := c.String(); path != "" {
		m.Dir().Chdir(path)
		c.Exit()
	}
}

// Glob starts the glob mode.
func (g *Goful) Glob() {
	g.next = cmdline.New(&globMode{g}, g)
}

type globMode struct {
	*Goful
}

func (m *globMode) String() string          { return "glob" }
func (m *globMode) Prompt() string          { return "Glob pattern(검색): " }
func (m *globMode) Draw(c *cmdline.Cmdline) { c.DrawLine() }
func (m *globMode) Run(c *cmdline.Cmdline) {
	if pattern := c.String(); pattern != "" {
		m.Dir().Glob(pattern)
		c.Exit()
	}
}

// Globdir starts the globdir mode.
func (g *Goful) Globdir() {
	g.next = cmdline.New(&globdirMode{g}, g)
}

type globdirMode struct {
	*Goful
}

func (m *globdirMode) String() string          { return "globdir" }
func (m *globdirMode) Prompt() string          { return "GlobDir pattern(하부검색): " }
func (m *globdirMode) Draw(c *cmdline.Cmdline) { c.DrawLine() }
func (m *globdirMode) Run(c *cmdline.Cmdline) {
	if pattern := c.String(); pattern != "" {
		m.Dir().Globdir(pattern)
		c.Exit()
	}
}
