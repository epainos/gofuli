package cmdline

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/epainos/gofuli/util"
	"github.com/epainos/gofuli/widget"
	"github.com/google/shlex"
)

// Completion is a list box displays completions of the cmdline text.
type Completion struct {
	*widget.ListBox
	cmdline widget.Widget
}

var completionKeymap func(*Completion) widget.Keymap

// ConfigCompletion sets a completion keymap function.
func ConfigCompletion(config func(*Completion) widget.Keymap) {
	completionKeymap = config
}

// NewCompletion creates a new completion list box.
func NewCompletion(x, y, width, height int, cmdline *Cmdline) *Completion {
	comp := &Completion{
		ListBox: widget.NewListBox(x, y, width, height, "Completion"),
		cmdline: cmdline,
	}

	parser := parseCmdline(cmdline)
	var candidates []string
	if cmdline.mode.String() == "shell" && parser.cmdname == "" {
		candidates = append(parser.compCommands(), parser.compFiles()...)
	} else {
		candidates = parser.compFiles()
	}

	for _, v := range candidates {
		comp.AppendHighlightString(v, parser.current)
	}
	comp.ColumnAdjustContentsWidth()
	return comp
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Next returns nil.
func (c *Completion) Next() widget.Widget { return widget.Nil() }

// Disconnect do noting.
func (c *Completion) Disconnect() {}

// InsertCompletion inserts a selected completion to the cmdline and exits the completion.
func (c *Completion) InsertCompletion() {
	start := c.cmdline.(*Cmdline).TextBeforeCursor()
	start = util.ExpandPath(start)
	compname := c.CurrentContent().Name()

	// 마지막 경로 요소만 대소문자 무관하게 치환
	lastSlash := strings.LastIndexAny(start, "/\\")
	var base, prefix string
	if lastSlash == -1 {
		base = start
		prefix = ""
	} else {
		base = start[lastSlash+1:]
		prefix = start[:lastSlash+1]
	}

	// base와 compname의 공통 접두사 길이(대소문자 무관)를 구함
	baseLower := strings.ToLower(base)
	compnameLower := strings.ToLower(compname)
	common := 0
	maxlen := len(baseLower)
	if len(compnameLower) < maxlen {
		maxlen = len(compnameLower)
	}
	for i := 0; i < maxlen; i++ {
		if baseLower[i] == compnameLower[i] {
			common++
		} else {
			break
		}
	}
	// base의 공통 접두사 이후 부분을 compname으로 치환
	c.cmdline.(*Cmdline).SetText(prefix + compname)
	c.cmdline.(*Cmdline).MoveCursor(len(prefix + compname))
	c.cmdline.Disconnect()

	// 기존 대소문자 구분 로직 (주석처리)
	// for i := len(compname); i >= 0; i-- {
	// 	if strings.HasSuffix(start, compname[:i]) {
	// 		insertStr := compname[i:]
	// 		c.cmdline.(*Cmdline).InsertString(insertStr)
	// 		break
	// 	}
	// }
}

// Input to the completion or to the cmdline and exits.
func (c *Completion) Input(key string) {
	if cb, ok := completionKeymap(c)[key]; ok {
		cb()
	} else {
		c.cmdline.Disconnect()
		c.cmdline.Input(key)
	}
}

// Exit the completion.
func (c *Completion) Exit() { c.cmdline.Disconnect() }

// HandleMouseClick handles mouse click events in completion mode
func (c *Completion) HandleMouseClick(x, y int) {
	// 마우스 클릭이 completion 영역 밖에서 발생하면 ESC 효과
	compX, compY := c.LeftTop()
	compWidth := c.Width()
	compHeight := c.Height()

	if x < compX || x >= compX+compWidth ||
		y < compY || y >= compY+compHeight {
		// completion 영역 밖 클릭 - ESC 효과
		c.Exit()
		return
	}

	// completion 영역 내부 클릭 처리
	clickedIndex := c.convertClickToIndex(x, y)
	if clickedIndex >= 0 && clickedIndex < c.Upper() {
		// 클릭한 위치의 인덱스로 커서 이동
		c.SetCursor(clickedIndex)
		// 해당 자동완성 항목을 cmdline에 삽입
		c.InsertCompletion()
	}
}

// convertClickToIndex converts mouse click coordinates to list index
func (c *Completion) convertClickToIndex(x, y int) int {
	compX, compY := c.LeftTop()

	// 상대 좌표 계산
	relX := x - compX
	relY := y - compY

	// 헤더와 보더 고려
	borderOffset := 1
	if c.BorderStyle() != widget.NoBorder {
		borderOffset = 1
	}

	// 클릭한 행과 열 계산
	// ListBox Draw에서 row는 1부터 시작하므로 실제 행은 relY - borderOffset - 1
	clickedRow := relY - borderOffset - 0    // -1 for header
	colWidth := (c.Width()-2)/c.Column() - 1 // 열 너비 (보더 고려)
	clickedCol := relX / (colWidth + 1)

	// 유효한 범위 체크
	if clickedRow < 0 || clickedRow >= c.Height()-2 || clickedCol < 0 || clickedCol >= c.Column() {
		return -1
	}

	// 실제 인덱스 계산 (offset + row * column + col)
	// ListBox Draw에서 row는 1부터 시작하지만, 실제 인덱스는 0부터 시작
	// 따라서 clickedRow를 그대로 사용하면 됨 (이미 0부터 시작하도록 계산됨)
	actualIndex := c.Offset() + clickedRow*c.Column() + clickedCol

	// 범위 체크
	if actualIndex >= 0 && actualIndex < c.Upper() {
		return actualIndex
	}

	return -1
}

type parser struct {
	cmdname string
	current string
	preword string
}

func parseCmdline(c *Cmdline) *parser {
	text := c.TextBeforeCursor()
	words, _ := shlex.Split(text)

	switch i := len(words); i {
	case 0:
		return &parser{"", "", ""}
	case 1:
		if isSep(text[len(text)-1]) {
			return &parser{words[0], "", ""}
		}
		return &parser{"", words[0], ""}
	default:
		if isSep(text[len(text)-1]) {
			return &parser{words[0], "", words[i-1]}
		}
		return &parser{words[0], words[i-1], words[i-2]}
	}
}

func isSep(b byte) bool {
	return b == ' ' || b == ';' || b == '|' || b == '>' || b == '&'
}

func (p *parser) compFiles() (candidates []string) {
	candidates = make([]string, 0, 100)
	dirname, file := filepath.Split(p.current)
	if dirname == "" && file == "~" {
		dirname = "~/"
		file = ""
	}
	if dirname == "" {
		dirname = "."
	}
	dirname = util.ExpandPath(dirname)
	dir, err := os.Open(dirname)
	if err != nil {
		return candidates
	}
	defer dir.Close()
	files, err := dir.Readdir(-1)
	if err != nil {
		return candidates
	}

	for _, f := range files {
		name := f.Name()
		if strings.HasPrefix(strings.ToLower(name), strings.ToLower(file)) {
			if f.IsDir() {
				name += "/"
			}
			candidates = append(candidates, name)
		}
	}
	sort.Strings(candidates)
	return candidates
}

func (p *parser) compCommands() (candidates []string) {
	commands, _ := util.SearchCommands()
	candidates = make([]string, 0, len(commands))
	for name := range commands {
		if strings.HasPrefix(strings.ToLower(name), strings.ToLower(p.current)) {
			candidates = append(candidates, name)
		}
	}
	sort.Strings(candidates)
	return candidates
}
