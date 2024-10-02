package app

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	// "github.com/epainos/gofuli/app" // Removed to fix import cycle and missing metadata issues
	"github.com/epainos/gofuli/cmdline"
	"github.com/epainos/gofuli/look"
	"github.com/epainos/gofuli/menu"
	"github.com/epainos/gofuli/message"
	"github.com/epainos/gofuli/util"
	"github.com/epainos/gofuli/widget"
	"github.com/f1bonacc1/glippy"
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
		return ("Copy(복사) -> ")
		// return fmt.Sprintf("Copy(복사) %s -> ", m.src)
	} else {
		return "Copy(복사) : "
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
		return ("Move(이동) -> ")
		// return fmt.Sprintf("Move(이동) %s -> ", m.src)
	} else {
		return "Move(이동) : "
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

func (m *renameMode) String() string { return "rename" }
func (m *renameMode) Prompt() string { return "Rename(이름변경) -> " }

// func (m *renameMode) Prompt() string          { return fmt.Sprintf("Rename(이름변경) %s -> ", m.src) }
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
		return "Mode(권한) default 755: "
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

//////////////////////////////////////////////////////////////////////////////////////////////////////////////////

func myfGetLastWord(filePath string) string {
	lastIndex := strings.LastIndex(filePath, "/")

	if lastIndex == -1 {
		return filePath
	}
	return filePath[lastIndex+1:]
}

// addMyApp add my app added by user
func (g *Goful) AddMyApp() {

	src := ifElseSting((runtime.GOOS == "windows"), `start `, ifElseSting((runtime.GOOS == "darwin"), `open -a`, "")) + ` '` + g.File().Path() + `' ` + ` %F`
	c := cmdline.New(&addMyAppMode{
		Goful:              g,
		myShortCut:         "",
		myAppName:          "",
		myAppCommand:       src,
		isDoneMyAppCommand: false,
	}, g)
	c.SetText(src)
	g.next = c
}

type addMyAppMode struct {
	*Goful
	myShortCut         string
	myAppName          string
	myAppCommand       string
	isDoneMyAppCommand bool
}

func (m *addMyAppMode) String() string { return "addMyApp" }
func (m *addMyAppMode) Prompt() string {

	if !m.isDoneMyAppCommand {
		return "addMyApp 사용자앱 추가:"
	} else if m.myAppName == "" {
		return "appName 앱이름: "
	} else {
		return "shortCut for '" + m.myAppName + "' " + m.myShortCut + " 단축키: "
	}
}

func (m *addMyAppMode) Draw(c *cmdline.Cmdline) { c.DrawLine() }
func (m *addMyAppMode) Run(c *cmdline.Cmdline) {
	if !m.isDoneMyAppCommand {
		m.myAppCommand = c.String()
		m.isDoneMyAppCommand = true
		c.SetText(strings.ReplaceAll(myfGetLastWord(c.String()), `'`, ``))
	} else if m.myAppName == "" {
		m.myAppName = c.String()
		c.SetText("")
	} else {
		m.myShortCut = c.String()
		if len(m.myShortCut) == 1 {
			writeMyAppToFile(myAppFile, m.myShortCut+" <||> "+m.myAppName+" <||> "+m.myAppCommand+"\n")
			menu.Add("myApp", m.myShortCut, m.myAppName, func() { m.Spawn(m.myAppCommand) })
			m.Workspace().ReloadAll()
			c.Exit()
		} else {
			m.myShortCut = ": (type one character, please)"
			c.SetText("")
		}
	}
}

const myAppFile = "~/.goful/myApp"

func writeMyAppToFile(path string, content string) {

	// file, err := os.Create(util.ExpandPath(path))
	file, err := os.OpenFile(util.ExpandPath(path), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		glippy.Set("생성실패: " + err.Error())

		return
	}
	defer file.Close()

	// 파일 끝에 내용 추가
	if _, err := file.WriteString(content); err != nil {
		glippy.Set("쓰기실패: " + err.Error())
		return
	}

}

func (g *Goful) OpenMyAppList(path string) {
	if path == "" {
		path = "~/.goful/myApp"
	}

	file, err := os.OpenFile(util.ExpandPath(path), os.O_RDONLY, os.FileMode(0644))
	if err != nil {
		fmt.Println("파일 열기 실패:", err)
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		items := strings.Split(line, " <||> ")
		if len(items) == 3 {
			menu.Add("myApp", items[0], items[1], func() { g.Spawn(items[2]) })
			// fmt.Printf("항목1: %s, 항목2: %s, 항목3: %s\n", items[0], items[1], items[2])
		} else {
			// fmt.Println("잘못된 형식의 줄:", line)
		}
	}

}

// DelMyApp delete my app added by user
func (g *Goful) DelMyApp() {
	c := cmdline.New(&dellMyAppMode{
		Goful:                g,
		myShortCut:           "",
		myShortCutAndAppName: make(map[string]string),
		isInList:             false,
	}, g)
	c.SetText("")
	g.next = c
}

type dellMyAppMode struct {
	*Goful
	myShortCut           string
	myShortCutAndAppName map[string]string
	isInList             bool
}

func (m *dellMyAppMode) String() string { return "dellMyApp" }
func (m *dellMyAppMode) Prompt() string {
	shortcutList := []string{""}
	shortcutList, m.myShortCutAndAppName = loadMyShortcuts(myAppFile)

	if len(shortcutList) > 10 {
		shortcutList = shortcutList[:10]
		shortcutList[9] = "..."
	}
	src := strings.Join(shortcutList, ", ")
	if m.myShortCut == "" {
		return "App Shortcut to Delete (지울 단축키): " + src + " : "
		// } else if m.myShortCutAndAppName[m.myShortCut] == "" {
		// 	return "Shortcut is NOT found. 단축키를 확인해주세요."
	} else {
		return "del your app? '" + m.myShortCutAndAppName[m.myShortCut] + "' [Y, n] : "
	}
}

// func (m *dellMyAppMode) Prompt() string          { return fmt.Sprintf("dellMyApp(이름변경) %s -> ", m.src) }
func (m *dellMyAppMode) Draw(c *cmdline.Cmdline) { c.DrawLine() }
func (m *dellMyAppMode) Run(c *cmdline.Cmdline) {
	if m.myShortCutAndAppName[c.String()] != "" {
		m.isInList = true
	}
	if len(m.myShortCutAndAppName) == 0 {
		message.Info("No app to delete")
		c.Exit()
	} else if m.isInList == false && m.myShortCutAndAppName[c.String()] == "" {
		message.Errorf("Shortcut is NOT found. 단축키를 확인해주세요.")
		// m.myShortCut = c.String()
		c.SetText("")
	} else if m.myShortCut == "" {
		m.myShortCut = c.String()
		c.SetText("y")

	} else {
		if c.String() == "Y" || c.String() == "y" || c.String() == "" {

			menu.DelMyAppFromListFile(myAppFile, m.myShortCut)
			message.Info("Deleted: " + m.myShortCut + " (" + m.myShortCutAndAppName[m.myShortCut] + ") ")
			menu.Remove("myApp", m.myShortCut)
			// m.Workspace().ReloadAll()
			// m.OpenMyAppList("")
			c.Exit()
			// }
		} else {
			message.Info("canceled: " + m.myShortCut + " (" + m.myShortCutAndAppName[m.myShortCut] + ") ")
			c.Exit()
		}
	}
}

func loadMyShortcuts(path string) ([]string, map[string]string) {
	var myShortCutList []string
	mapForShortcutAndAppName := make(map[string]string)

	file, err := os.OpenFile(util.ExpandPath(path), os.O_RDONLY, os.FileMode(0644))
	if err != nil {
		return nil, mapForShortcutAndAppName
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, " <||> ")
		// println(parts[0], `:`, parts[1], `:`, parts[2])
		if len(parts) == 3 {
			myShortCutList = append(myShortCutList, parts[0])
			key, value := parts[0], parts[1]
			mapForShortcutAndAppName[key] = value
		} else {
			fmt.Println("잘못된 형식의 줄:", line)
		}
	}

	if err := scanner.Err(); err != nil {
		return myShortCutList, mapForShortcutAndAppName
	}

	return myShortCutList, mapForShortcutAndAppName
}

// //////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// addMyBookmark add my bookmart by user
func (g *Goful) AddMyBookmark() {

	src := strings.ReplaceAll(g.Dir().Path, "\\", "/")
	c := cmdline.New(&addMyBookmarkMode{
		Goful:                   g,
		myShortCut:              "",
		myBookmarkName:          "",
		myBookmarkCommand:       src,
		isDoneMyBookmarkCommand: false,
	}, g)
	c.SetText(src)
	g.next = c
}

type addMyBookmarkMode struct {
	*Goful
	myShortCut              string
	myBookmarkName          string
	myBookmarkCommand       string
	isDoneMyBookmarkCommand bool
}

func (m *addMyBookmarkMode) String() string { return "addMyBookmark" }
func (m *addMyBookmarkMode) Prompt() string {

	if !m.isDoneMyBookmarkCommand {
		return "add Bookmark      바로가기 추가:"
	} else if m.myBookmarkName == "" {
		return "nickname Bookmark 바로가기 별명: "
	} else {
		return "shortCut for '" + m.myBookmarkName + "' " + m.myShortCut + " 단축키: "
	}
}

const myBookmarkFile = "~/.goful/myBookmark"

// func (m *addMyBookmarkMode) Prompt() string          { return fmt.Sprintf("addMyBookmark(이름변경) %s -> ", m.src) }
func (m *addMyBookmarkMode) Draw(c *cmdline.Cmdline) { c.DrawLine() }
func (m *addMyBookmarkMode) Run(c *cmdline.Cmdline) {
	if !m.isDoneMyBookmarkCommand {
		m.myBookmarkCommand = c.String()
		m.isDoneMyBookmarkCommand = true
		c.SetText(myfGetLastWord(c.String()))
	} else if m.myBookmarkName == "" {
		m.myBookmarkName = c.String()
		c.SetText("")
	} else {
		m.myShortCut = c.String()
		if len(m.myShortCut) == 1 {
			writeMyAppToFile(myBookmarkFile, m.myShortCut+" <||> "+m.myBookmarkName+" <||> "+m.myBookmarkCommand+"\n")
			menu.Add("myBookmark", m.myShortCut, m.myBookmarkName, func() { m.Dir().Chdir(m.myBookmarkCommand) })
			m.Workspace().ReloadAll()
			c.Exit()
		} else {
			m.myShortCut = ": (type one character, please)"
			c.SetText("")
		}
	}
}

func (g *Goful) OpenMyBookmarkList(path string) {
	if path == "" {
		path = "~/.goful/myBookmark"
	}

	file, err := os.OpenFile(util.ExpandPath(path), os.O_RDONLY, os.FileMode(0644))
	if err != nil {
		fmt.Println("파일 열기 실패:", err)
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		items := strings.Split(line, " <||> ")
		if len(items) == 3 {
			menu.Add("myBookmark", items[0], items[1], func() { g.Dir().Chdir(items[2]) })
			// fmt.Printf("항목1: %s, 항목2: %s, 항목3: %s\n", items[0], items[1], items[2])
		} else {
			// fmt.Println("잘못된 형식의 줄:", line)
		}
	}

}

// delMyBookmark delete my app added by user
func (g *Goful) DelMyBookmark() {
	c := cmdline.New(&delMyBookmarkMode{
		Goful:                     g,
		myShortCut:                "",
		myShortCutAndBookmarkName: make(map[string]string),
		isInList:                  false,
	}, g)
	c.SetText("")
	g.next = c
}

type delMyBookmarkMode struct {
	*Goful
	myShortCut                string
	myShortCutAndBookmarkName map[string]string
	isInList                  bool
}

func (m *delMyBookmarkMode) String() string { return "delMyBookmark" }
func (m *delMyBookmarkMode) Prompt() string {
	shortcutList := []string{""}
	shortcutList, m.myShortCutAndBookmarkName = loadMyShortcuts(myBookmarkFile)

	if len(shortcutList) > 10 {
		shortcutList = shortcutList[:10]
		shortcutList[9] = "..."
	}
	src := strings.Join(shortcutList, ", ")
	if m.myShortCut == "" {
		return "Shortcut to Delete (지울 단축키): " + src + " : "
		// } else if m.myShortCutAndAppName[m.myShortCut] == "" {
		// 	return "Shortcut is NOT found. 단축키를 확인해주세요."
	} else {
		return "del your bookmark? '" + m.myShortCutAndBookmarkName[m.myShortCut] + "' [Y, n] : "
	}
}

// func (m *delMyBookmarkMode) Prompt() string          { return fmt.Sprintf("delMyBookmark(이름변경) %s -> ", m.src) }
func (m *delMyBookmarkMode) Draw(c *cmdline.Cmdline) { c.DrawLine() }
func (m *delMyBookmarkMode) Run(c *cmdline.Cmdline) {
	if m.myShortCutAndBookmarkName[c.String()] != "" {
		m.isInList = true
	}
	if len(m.myShortCutAndBookmarkName) == 0 {
		message.Info("No bookmark to delete...")
		c.Exit()
	} else if m.isInList == false && m.myShortCutAndBookmarkName[c.String()] == "" {
		message.Errorf("Shortcut is NOT found. 단축키를 확인해주세요.")
		// m.myShortCut = c.String()
		c.SetText("")
	} else if m.myShortCut == "" {
		m.myShortCut = c.String()
		c.SetText("y")

	} else {
		if c.String() == "Y" || c.String() == "y" || c.String() == "" {

			menu.DelMyAppFromListFile(myBookmarkFile, m.myShortCut)
			message.Info("Deleted: " + m.myShortCut + " (" + m.myShortCutAndBookmarkName[m.myShortCut] + ") ")
			menu.Remove("myBookmark", m.myShortCut)
			// m.Workspace().ReloadAll()
			// m.OpenMyAppList("")
			c.Exit()
			// }
		} else {
			message.Info("canceled: " + m.myShortCut + " (" + m.myShortCutAndBookmarkName[m.myShortCut] + ") ")
			c.Exit()
		}
	}
}
