package app

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"unicode"

	// "github.com/epainos/gofuli/app" // Removed to fix import cycle and missing metadata issues
	"github.com/epainos/gofuli/cmdline"
	"github.com/epainos/gofuli/filer"
	"github.com/epainos/gofuli/look"
	"github.com/epainos/gofuli/menu"
	"github.com/epainos/gofuli/message"
	"github.com/epainos/gofuli/util"
	"github.com/epainos/gofuli/widget"
	"github.com/f1bonacc1/glippy"
	"github.com/mattn/go-runewidth"
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
	input := c.String()

	// 빈 입력이면 첫 번째 옵션을 선택
	if input == "" {
		if len(m.options) > 0 {
			m.result = m.options[0]
		}
		c.Exit()
		return
	}

	// 사용자 입력과 옵션들을 비교 (대소문자 구분)
	for _, opt := range m.options {
		if input == opt {
			m.result = opt
			c.Exit()
			return
		}
	}

	// 일치하는 옵션이 없으면 입력을 지움
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

// 파일명 축약 함수: 앞 n자 + ... + 확장자 (폴더는 확장자 구분 안함)
func shortName(full string, n int) string {
	// 실제 파일/폴더인지 확인
	if info, err := os.Stat(full); err == nil {
		// 폴더인 경우 확장자 처리하지 않음
		if info.IsDir() {
			r := []rune(full)
			if len(r) > n {
				return string(r[:n]) + ".."
			}
			return full
		}
	}

	// 파일인 경우 또는 확인할 수 없는 경우 기존 로직 사용
	ext := filepath.Ext(full)
	name := full

	// 확장자가 있으면 파일명에서 제거
	if ext != "" {
		name = strings.TrimSuffix(full, ext)
	}

	r := []rune(name)
	if len(r) > n {
		name = string(r[:n]) + ".."
	}
	return name + ext
}

// Copy starts the copy mode.
func (g *Goful) Copy() {
	c := cmdline.New(&copyMode{Goful: g, src: ""}, g)
	if g.Dir().IsMark() {
		// 여러 파일 복사 시에도 목적지 폴더를 바로 표시
		c.SetText(g.Workspace().NextDir().Path)
	} else {
		// 단일 파일 복사 시에는 (반대쪽 pane 경로 + 파일명)으로 기본값 세팅
		dstPath := filepath.Join(g.Workspace().NextDir().Path, g.File().Name())
		c.SetText(dstPath)
	}
	g.next = c
}

type copyMode struct {
	*Goful
	src string // 이 필드는 이제 첫 번째 엔터 입력 전까지는 비어있지 않고, src 파일(들) 경로를 저장
}

func (m *copyMode) String() string { return "copy" }

func (m *copyMode) Prompt() string {
	if m.src == "" { // 아직 src가 설정되지 않은 초기 상태
		if m.Dir().IsMark() {
			// 마크된 파일들이 있다면 마크된 파일 수와 목적지 폴더를 표시
			return fmt.Sprintf("Copy(복사) %d files -> ", m.Dir().MarkCount())
		} else {
			// 단일 파일이라면 복사될 전체 경로(파일명 포함)를 표시
			// dstPath := filepath.Join(m.Workspace().NextDir().Path, shortName(m.File().Name(), 10)shortName(m.File().Name(), 10))
			return fmt.Sprintf("Copy(복사) %s -> ", shortName(m.File().Name(), 10))
		}
	}
	return "Copy(복사) -> "
}

func (m *copyMode) Draw(c *cmdline.Cmdline) { c.DrawLine() }

func (m *copyMode) Run(c *cmdline.Cmdline) {
	if m.src == "" { // 첫 번째 엔터 입력 (현재 파일/마크된 파일 -> 목적지 폴더)
		var srcs []string
		if m.Dir().IsMark() {
			srcs = m.Dir().MarkfilePaths() // 마크된 파일 목록
		} else {
			srcs = []string{m.File().Path()} // 단일 파일 목록 (전체 경로)
		}

		dst := c.String() // 현재 cmdline에 표시된 텍스트(목적지 경로)
		// 입력값이 파일이면 그 상위 폴더, 폴더면 그대로
		dstDir := dst
		if len(srcs) == 1 || (!m.Dir().IsMark() && len(srcs) == 1) {
			dstDir = filepath.Dir(dst)
		}
		if _, err := os.Stat(dstDir); os.IsNotExist(err) {
			err := os.MkdirAll(dstDir, 0755)
			if err != nil {
				message.Error(fmt.Errorf("Create error (생성 오류): %v", err))
				c.Exit()
				return
			}
		}
		m.copy(dst, srcs...)
		c.Exit()
	} else {
		dst := c.String()
		var srcs []string
		if m.Dir().IsMark() {
			srcs = m.Dir().MarkfilePaths()
		} else {
			srcs = []string{m.src} // m.src가 단일 파일 경로를 가지고 있다고 가정
		}
		dstDir := dst
		if len(srcs) == 1 || (!m.Dir().IsMark() && len(srcs) == 1) {
			dstDir = filepath.Dir(dst)
		}
		if _, err := os.Stat(dstDir); os.IsNotExist(err) {
			err := os.MkdirAll(dstDir, 0755)
			if err != nil {
				message.Error(fmt.Errorf("Create error (생성 오류): %v", err))
				c.Exit()
				return
			}
		}
		m.copy(dst, srcs...)
		c.Exit()
	}
}

// Copy2 starts the copy mode with completion notification.
func (g *Goful) Copy2() {
	c := cmdline.New(&copy2Mode{Goful: g, src: ""}, g)
	if g.Dir().IsMark() {
		// 여러 파일 복사 시에도 목적지 폴더를 바로 표시
		c.SetText(g.Workspace().NextDir().Path)
	} else {
		// 단일 파일 복사 시에는 (반대쪽 pane 경로 + 파일명)으로 기본값 세팅
		dstPath := filepath.Join(g.Workspace().NextDir().Path, g.File().Name())
		c.SetText(dstPath)
	}
	g.next = c
}

type copy2Mode struct {
	*Goful
	src string // 이 필드는 이제 첫 번째 엔터 입력 전까지는 비어있지 않고, src 파일(들) 경로를 저장
}

func (m *copy2Mode) String() string { return "copy2" }

func (m *copy2Mode) Prompt() string {
	if m.src == "" { // 아직 src가 설정되지 않은 초기 상태
		if m.Dir().IsMark() {
			// 마크된 파일들이 있다면 마크된 파일 수와 목적지 폴더를 표시
			return fmt.Sprintf("Async Copy(overwrite) (비동기 복사(덮어쓰기)) %d files -> ", m.Dir().MarkCount())
		} else {
			// 단일 파일이라면 복사될 전체 경로(파일명 포함)를 표시
			// dstPath := filepath.Join(m.Workspace().NextDir().Path, m.File().Name())
			return fmt.Sprintf("Copy(복사) %s -> ", shortName(m.File().Name(), 10))
		}
	}
	return "Copy(복사)  -> "
}

func (m *copy2Mode) Draw(c *cmdline.Cmdline) { c.DrawLine() }

func (m *copy2Mode) Run(c *cmdline.Cmdline) {
	if m.src == "" { // 첫 번째 엔터 입력 (현재 파일/마크된 파일 -> 목적지 폴더)
		if m.Dir().IsMark() {
			m.src = strings.Join(m.Dir().MarkfilePaths(), "|") // 마크된 파일 경로들을 구분자로 조인하여 저장
		} else {
			m.src = m.File().Path() // 현재 파일의 전체 경로 저장
		}
		dst := util.ExpandPath(c.String()) // 현재 cmdline에 표시된 텍스트(목적지 경로)
		var srcs []string
		if m.Dir().IsMark() {
			srcs = m.Dir().MarkfilePaths() // 마크된 파일 목록
		} else {
			srcs = []string{m.File().Path()} // 단일 파일 목록
		}
		// 입력값이 파일이면 그 상위 폴더, 폴더면 그대로
		dstDir := dst
		if len(srcs) == 1 || (!m.Dir().IsMark() && len(srcs) == 1) {
			dstDir = filepath.Dir(dst)
		}
		if _, err := os.Stat(dstDir); os.IsNotExist(err) {
			err := os.MkdirAll(dstDir, 0755)
			if err != nil {
				message.Error(fmt.Errorf("Create error (생성 오류): %v", err))
				c.Exit()
				return
			}
		}
		m.copy2(dst, srcs...)
		c.Exit()
	} else {
		dst := util.ExpandPath(c.String())
		var srcs []string
		if m.Dir().IsMark() {
			srcs = m.Dir().MarkfilePaths()
		} else {
			srcs = []string{m.src} // m.src가 단일 파일 경로를 가지고 있다고 가정
		}
		dstDir := dst
		if len(srcs) == 1 || (!m.Dir().IsMark() && len(srcs) == 1) {
			dstDir = filepath.Dir(dst)
		}
		if _, err := os.Stat(dstDir); os.IsNotExist(err) {
			err := os.MkdirAll(dstDir, 0755)
			if err != nil {
				message.Error(fmt.Errorf("Create error (생성 오류): %v", err))
				c.Exit()
				return
			}
		}
		m.copy2(dst, srcs...)
		c.Exit()
	}
}

// copy2: 쉘 명령을 통해 파일을 복사하고 완료 여부를 Goful에 알립니다.
func (m *copy2Mode) copy2(dst string, srcs ...string) {
	// 완료/오류 메시지를 보낼 채널
	completionChan := make(chan string)

	// 고루틴에서 쉘 명령 실행
	go func() {
		defer close(completionChan) // 작업이 끝나면 채널 닫기

		var shellCmd string
		var args []string

		if runtime.GOOS == "windows" {
			shellCmd = "fcp"
			finalDst := dst // 기본적으로 원래 목적지 사용

			// 복사 대상이 단일 디렉토리인지 확인
			if len(srcs) == 1 {
				srcInfo, err := os.Stat(srcs[0])
				// 에러가 없고, 소스가 디렉토리인 경우
				if err == nil && srcInfo.IsDir() {
					// 목적지 경로에 소스 디렉토리의 이름을 합쳐 새로운 목적지 경로를 생성
					// 예: dst="D:\Backup", srcs[0]="C:\MyFolder" -> finalDst="D:\Backup\MyFolder"
					finalDst = filepath.Join(dst, filepath.Base(srcs[0]))
				}
			}

			// fcp 명령어 인자 구성
			args = []string{"/cmd=force_copy"}
			args = append(args, srcs...)
			// 수정된 최종 목적지 경로를 사용
			args = append(args, "/to="+filepath.ToSlash(finalDst))

		} else {
			shellCmd = "cp"
			args = []string{"-r"} // -v (verbose) 제거하여 진행 상황 출력 안 함
			args = append(args, srcs...)
			args = append(args, dst)
		}

		cmd := exec.Command(shellCmd, args...)

		if err := cmd.Run(); err != nil { // Run()은 Start()와 Wait()를 동시에 수행합니다.
			completionChan <- fmt.Sprintf("복사 에러: %v", err)
		} else {
			// [수정된 부분] 완료 메시지에 복사된 파일 목록 추가
			filenames := make([]string, len(srcs))
			for i, src := range srcs {
				// 전체 경로에서 파일 이름만 추출
				filenames[i] = filepath.Base(src)
			}
			// 파일 이름 목록을 하나의 문자열로 합침
			completionChan <- fmt.Sprintf("복사 완료! : (%s)", strings.Join(filenames, ", "))
		}

	}()

	// UI 업데이트를 위한 고루틴 (완료 메시지만 표시)
	go func() {
		msg := <-completionChan // 채널에서 메시지가 올 때까지 대기
		// 여기에서 Goful의 상태 바를 업데이트하는 메서드를 호출하세요.
		// 예: m.Goful.SetStatusBarText(msg)
		message.Info("[Status]: " + msg) // 콘솔에 완료/오류 메시지 출력
	}()
}

// Duplicate 함수 정의: 마크된 파일 또는 단일 파일 복제를 시작합니다.
func (g *Goful) Duplicate() {
	var srcPaths []string // 복제할 파일(들)의 목록

	if g.Dir().IsMark() {
		// 마크된 파일들이 있다면 그 목록을 가져옵니다.
		srcPaths = g.Dir().MarkfilePaths()
	} else {
		// 마크된 파일이 없다면 현재 선택된 파일만 복제합니다.
		srcPaths = []string{g.File().Name()}
	}

	if len(srcPaths) == 0 {
		message.Info("None to duplicate (복제할 파일 없음)")
		return
	}

	// cmdline 모드를 시작하며 복제할 파일 목록을 넘겨줍니다.
	c := cmdline.New(&duplicateMode{g, srcPaths}, g)
	if len(srcPaths) == 1 {
		baseName := filepath.Base(srcPaths[0])
		ext := filepath.Ext(baseName)
		fileNameWithoutExt := strings.TrimSuffix(baseName, ext)

		// 중복 파일명 처리: _, __, ___ 형태로 증가
		suggestedName := fileNameWithoutExt + "_"
		srcDir := filepath.Dir(srcPaths[0])
		counter := 1
		for {
			testPath := filepath.Join(srcDir, suggestedName+ext)
			if _, err := os.Stat(testPath); os.IsNotExist(err) {
				break
			}
			suggestedName = fileNameWithoutExt + strings.Repeat("_", counter+1)
			counter++
		}

		c.SetText(suggestedName + ext) // 입력란에 기본값 세팅 (확장자 포함)
		c.MoveCursor(-len(ext))        // 확장자 앞에 커서 위치
	}
	g.next = c
}

// ---
// duplicateMode는 cmdline.Mode 인터페이스를 구현하며 실제 복제 로직을 담습니다.
type duplicateMode struct {
	*Goful            // Goful 인스턴스에 접근하기 위해 포함
	srcPaths []string // 복제할 파일(들)의 전체 경로 목록
}

func (m *duplicateMode) String() string { return "duplicate" }

func (m *duplicateMode) Prompt() string {
	if len(m.srcPaths) == 1 {
		baseName := filepath.Base(m.srcPaths[0])
		return fmt.Sprintf("Duplicate(복제) %s -> ", shortName(baseName, 10))
	}
	if len(m.srcPaths) > 1 {
		return fmt.Sprintf("Duplicate %d files? (파일 %d개 복제?) [Y/n] ", len(m.srcPaths), len(m.srcPaths))
	}
	return "Duplicate files (파일 복제)? [Y/n] " // 복제할 파일이 없는 경우 (오류 처리)
}

func (m *duplicateMode) Draw(c *cmdline.Cmdline) { c.DrawLine() }

func (m *duplicateMode) Run(c *cmdline.Cmdline) {
	if len(m.srcPaths) == 0 {
		message.Info("None to duplicate (복제할 파일 없음)")
		c.Exit()
		return
	}

	// 단일 파일 또는 폴더인 경우 사용자 입력을 받아서 처리
	if len(m.srcPaths) == 1 {
		srcPath := m.srcPaths[0]
		srcInfo, err := os.Stat(srcPath)
		if err != nil {
			message.Error(err)
			c.Exit()
			return
		}

		userInput := c.String()
		if userInput == "" {
			baseName := filepath.Base(srcPath)
			ext := filepath.Ext(baseName)
			fileNameWithoutExt := strings.TrimSuffix(baseName, ext)
			if srcInfo.IsDir() {
				userInput = fileNameWithoutExt + "_copy"
			} else {
				userInput = fileNameWithoutExt + "_"
			}
		}

		srcDir := filepath.Dir(srcPath)
		var newFileName string
		if srcInfo.IsDir() {
			// 폴더는 확장자 없음
			newFileName = userInput
			// 중복 폴더명 처리
			counter := 1
			for {
				testPath := filepath.Join(srcDir, newFileName)
				if _, err := os.Stat(testPath); os.IsNotExist(err) {
					break
				}
				newFileName = userInput + "_" + strconv.Itoa(counter)
				counter++
			}
		} else {
			ext := filepath.Ext(filepath.Base(srcPath))
			newFileName = userInput
			if ext != "" && !strings.HasSuffix(userInput, ext) {
				newFileName = userInput + ext
			}
			// 중복 파일명 처리
			fileNameWithoutExt := strings.TrimSuffix(userInput, ext)
			counter := 1
			for {
				testPath := filepath.Join(srcDir, newFileName)
				if _, err := os.Stat(testPath); os.IsNotExist(err) {
					break
				}
				newFileName = fileNameWithoutExt + strings.Repeat("_", counter+1) + ext
				counter++
			}
		}

		newAbsPath := filepath.Join(srcDir, newFileName)

		// 자기 자신 또는 하위로 복사 방지
		absSrc, _ := filepath.Abs(srcPath)
		absDst, _ := filepath.Abs(newAbsPath)
		if srcInfo.IsDir() && (absDst == absSrc || strings.HasPrefix(absDst+string(os.PathSeparator), absSrc+string(os.PathSeparator))) {
			message.Error(fmt.Errorf("Cannot copy to self/sub (자신/하위로 복사 불가)"))
			c.Exit()
			return
		}

		// 내부 copy 함수 사용 (폴더/파일 모두 지원)
		m.Goful.copy(newAbsPath, srcPath)
		message.Info(fmt.Sprintf("Duplicated (복제완료): %s", newFileName))

		m.Goful.Dir().Reset()
		m.Goful.Workspace().ReloadAll()
		c.Exit()
		return
	}

	// 여러 파일/폴더인 경우 내부 함수 사용
	for _, srcPath := range m.srcPaths {
		srcInfo, err := os.Stat(srcPath)
		if err != nil {
			message.Error(err)
			continue
		}
		srcDir := filepath.Dir(srcPath)
		baseName := filepath.Base(srcPath)
		var newName string
		if srcInfo.IsDir() {
			newName = baseName + "_"
			counter := 1
			for {
				testPath := filepath.Join(srcDir, newName)
				if _, err := os.Stat(testPath); os.IsNotExist(err) {
					break
				}
				newName = baseName + strings.Repeat("_", counter+1)
				counter++
			}
		} else {
			ext := filepath.Ext(baseName)
			fileNameWithoutExt := strings.TrimSuffix(baseName, ext)
			newName = fileNameWithoutExt + "_" + ext
			counter := 1
			for {
				testPath := filepath.Join(srcDir, newName)
				if _, err := os.Stat(testPath); os.IsNotExist(err) {
					break
				}
				newName = fileNameWithoutExt + strings.Repeat("_", counter+1) + ext
				counter++
			}
		}
		newAbsPath := filepath.Join(srcDir, newName)
		// 자기 자신 또는 하위로 복사 방지
		absSrc, _ := filepath.Abs(srcPath)
		absDst, _ := filepath.Abs(newAbsPath)
		if srcInfo.IsDir() && (absDst == absSrc || strings.HasPrefix(absDst+string(os.PathSeparator), absSrc+string(os.PathSeparator))) {
			message.Error(fmt.Errorf("Cannot copy to self/sub (자신/하위로 복사 불가): %s -> %s", absSrc, absDst))
			continue
		}
		m.Goful.copy(newAbsPath, srcPath)
		message.Info(fmt.Sprintf("Duplicated (복제완료): %s", newName))
	}

	m.Goful.Dir().Reset()
	m.Goful.Workspace().ReloadAll()
	c.Exit()
}

// Move starts the move mode.
func (g *Goful) Move() {
	c := cmdline.New(&moveMode{Goful: g, src: ""}, g)
	if g.Dir().IsMark() {
		// 여러 파일 이동 시에도 목적지 폴더를 바로 표시
		c.SetText(g.Workspace().NextDir().Path)
	} else {
		// 단일 파일 이동 시에도 목적지 폴더를 바로 표시
		dstPath := filepath.Join(g.Workspace().NextDir().Path, g.File().Name())
		c.SetText(dstPath)
	}
	g.next = c
}

type moveMode struct {
	*Goful
	src string // 이 필드는 이제 첫 번째 엔터 입력 전까지는 비어있지 않고, src 파일(들) 경로를 저장
}

func (m *moveMode) String() string { return "move" }

func (m *moveMode) Prompt() string {
	if m.src == "" { // 아직 src가 설정되지 않은 초기 상태
		if m.Dir().IsMark() {
			// 마크된 파일들이 있다면 마크된 파일 수와 목적지 폴더를 표시
			return fmt.Sprintf("Move(이동) %d files -> ", m.Dir().MarkCount())
		} else {
			return fmt.Sprintf("Move(이동) %s -> ", shortName(m.File().Name(), 10))
		}
	}
	// m.src가 설정된 후 (즉, 첫 엔터를 누른 후)에는 이 프롬프트는 더 이상 보이지 않음
	return "Move(이동) -> "
}

func (m *moveMode) Draw(c *cmdline.Cmdline) { c.DrawLine() }

func (m *moveMode) Run(c *cmdline.Cmdline) {
	if m.src == "" { // 첫 번째 엔터 입력 (현재 파일/마크된 파일 -> 목적지 폴더)
		var srcs []string
		if m.Dir().IsMark() {
			srcs = m.Dir().MarkfilePaths()
		} else {
			srcs = []string{m.File().Path()}
		}
		dst := c.String()
		dstDir := dst
		if len(srcs) == 1 || (!m.Dir().IsMark() && len(srcs) == 1) {
			dstDir = filepath.Dir(dst)
		}
		if _, err := os.Stat(dstDir); os.IsNotExist(err) {
			err := os.MkdirAll(dstDir, 0755)
			if err != nil {
				message.Error(fmt.Errorf("Failed to create folder (폴더 생성 에러): %v", err))
				c.Exit()
				return
			}
		}
		m.move(dst, srcs...)
		c.Exit()
	} else {
		dst := c.String()
		var srcs []string
		if m.Dir().IsMark() {
			srcs = m.Dir().MarkfilePaths()
		} else {
			srcs = []string{m.src}
		}
		dstDir := dst
		if len(srcs) == 1 || (!m.Dir().IsMark() && len(srcs) == 1) {
			dstDir = filepath.Dir(dst)
		}
		if _, err := os.Stat(dstDir); os.IsNotExist(err) {
			err := os.MkdirAll(dstDir, 0755)
			if err != nil {
				message.Error(fmt.Errorf("Failed to create folder (폴더 생성 에러): %v", err))
				c.Exit()
				return
			}
		}
		m.move(dst, srcs...)
		c.Exit()
	}
}

// Move2 starts the move mode with completion notification.
func (g *Goful) Move2() {
	c := cmdline.New(&move2Mode{Goful: g, src: ""}, g)
	if g.Dir().IsMark() {
		// 여러 파일 이동 시에도 목적지 폴더를 바로 표시
		c.SetText(g.Workspace().NextDir().Path)
	} else {
		// 단일 파일 이동 시에도 목적지 폴더를 바로 표시
		dstPath := filepath.Join(g.Workspace().NextDir().Path, g.File().Name())
		c.SetText(dstPath)
	}
	g.next = c
}

type move2Mode struct {
	*Goful
	src string // 이 필드는 이제 첫 번째 엔터 입력 전까지는 비어있지 않고, src 파일(들) 경로를 저장
}

func (m *move2Mode) String() string { return "move2" }

func (m *move2Mode) Prompt() string {
	if m.src == "" { // 아직 src가 설정되지 않은 초기 상태
		if m.Dir().IsMark() {
			return fmt.Sprintf("Async Move(overwrite) (비동기 이동(덮어쓰기)) %d files -> ", m.Dir().MarkCount())
		} else {
			// 단일 파일이라면 파일명과 목적지 폴더를 표시
			return fmt.Sprintf("Async Move(overwrite) (비동기 이동(덮어쓰기)) %s -> ", shortName(m.File().Name(), 10))
		}
	}
	// m.src가 설정된 후 (즉, 첫 엔터를 누른 후)에는 이 프롬프트는 더 이상 보이지 않음
	return "Move(이동)  -> "
}

func (m *move2Mode) Draw(c *cmdline.Cmdline) { c.DrawLine() }

func (m *move2Mode) Run(c *cmdline.Cmdline) {
	if m.src == "" { // 첫 번째 엔터 입력 (현재 파일/마크된 파일 -> 목적지 폴더)
		if m.Dir().IsMark() {
			m.src = strings.Join(m.Dir().MarkfilePaths(), "|")
		} else {
			m.src = m.File().Path()
		}
		dst := util.ExpandPath(c.String())
		var srcs []string
		if m.Dir().IsMark() {
			srcs = m.Dir().MarkfilePaths()
		} else {
			srcs = []string{m.File().Path()}
		}
		dstDir := dst
		if len(srcs) == 1 || (!m.Dir().IsMark() && len(srcs) == 1) {
			dstDir = filepath.Dir(dst)
		}
		if _, err := os.Stat(dstDir); os.IsNotExist(err) {
			err := os.MkdirAll(dstDir, 0755)
			if err != nil {
				message.Error(fmt.Errorf("Failed to create folder (폴더 생성 에러): %v", err))
				c.Exit()
				return
			}
		}
		m.move2(dst, srcs...)
		c.Exit()
	} else {
		dst := util.ExpandPath(c.String())
		var srcs []string
		if m.Dir().IsMark() {
			srcs = m.Dir().MarkfilePaths()
		} else {
			srcs = []string{m.src} // m.src가 단일 파일 경로를 가지고 있다고 가정
		}
		dstDir := dst
		if len(srcs) == 1 || (!m.Dir().IsMark() && len(srcs) == 1) {
			dstDir = filepath.Dir(dst)
		}
		if _, err := os.Stat(dstDir); os.IsNotExist(err) {
			err := os.MkdirAll(dstDir, 0755)
			if err != nil {
				message.Error(fmt.Errorf("Failed to create folder (폴더 생성 에러): %v", err))
				c.Exit()
				return
			}
		}
		m.move2(dst, srcs...)
		c.Exit()
	}
}

// move2: 쉘 명령을 통해 파일을 이동하고 완료 여부를 Goful에 알립니다.
func (m *move2Mode) move2(dst string, srcs ...string) {
	completionChan := make(chan string)

	go func() {
		defer close(completionChan)

		var shellCmd string
		var args []string

		if runtime.GOOS == "windows" {
			shellCmd = "fcp"
			finalDst := dst // 기본 목적지 경로 설정

			// [수정 1] 이동 대상이 단일 디렉토리인지 확인
			if len(srcs) == 1 {
				srcInfo, err := os.Stat(srcs[0])
				if err == nil && srcInfo.IsDir() {
					// 목적지 경로에 소스 디렉토리 이름을 추가하여 최종 목적지 경로 생성
					finalDst = filepath.Join(dst, filepath.Base(srcs[0]))
				}
			}

			args = []string{"/cmd=move"}
			args = append(args, srcs...)
			// 수정된 최종 목적지 경로를 사용
			args = append(args, "/to="+filepath.ToSlash(finalDst))

		} else {
			shellCmd = "mv"
			args = []string{"-f"}
			args = append(args, srcs...)
			args = append(args, dst)
		}

		cmd := exec.Command(shellCmd, args...)

		if err := cmd.Run(); err != nil {
			completionChan <- fmt.Sprintf("Move error (이동 에러): %v", err)
		} else {
			// [수정 2] 완료 메시지에 이동된 파일 목록 추가
			filenames := make([]string, len(srcs))
			for i, src := range srcs {
				filenames[i] = filepath.Base(src)
			}
			completionChan <- fmt.Sprintf("Moved (이동 완료) : (%s)", strings.Join(filenames, ", "))
		}

	}()

	go func() {
		msg := <-completionChan
		message.Info("[Status]: " + msg)
	}()
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

// Rename2 starts a new rename mode that uses external commands.
func (g *Goful) Rename2() {
	src := g.File().Name()
	// cmdline.New를 사용하여 새로운 rename2Mode를 생성합니다.
	// 이 모드에서 사용자로부터 새 이름을 입력받을 것입니다.
	c := cmdline.New(&rename2Mode{g, src}, g)
	c.SetText(src)
	// 확장자를 제외한 부분에 커서를 위치시켜 편리하게 이름을 바꿀 수 있도록 합니다.
	c.MoveCursor(-len(filepath.Ext(src)))
	g.next = c // Goful의 다음 상태를 이 cmdline 모드로 설정합니다.
}

type rename2Mode struct {
	*Goful        // Goful 인스턴스에 접근하기 위해 포함
	src    string // 원본 파일의 전체 경로 (예: /path/to/old_file.txt)
}

func (m *rename2Mode) String() string          { return "rename2" }
func (m *rename2Mode) Prompt() string          { return "Async Rename (비동기로 이름바꿈) -> " }
func (m *rename2Mode) Draw(c *cmdline.Cmdline) { c.DrawLine() }

// Run 메서드가 실제 외부 명령어 실행 로직을 포함합니다.
func (m *rename2Mode) Run(c *cmdline.Cmdline) {
	dst := util.ExpandPath(c.String()) // 사용자로부터 입력받은 새 이름 (예: new_file.txt)
	if dst == "" {
		// 비어있는 이름이 입력되면 아무것도 하지 않고 종료합니다.
		c.Exit()
		return
	}

	// 원본 파일의 디렉토리 경로를 얻습니다.
	// 이는 mv/move 명령어가 올바른 경로에서 작동하도록 하기 위함입니다.
	srcDir := filepath.Dir(m.src)
	oldAbsPath := m.src                      // 원본 파일의 절대 경로
	newAbsPath := filepath.Join(srcDir, dst) // 새 파일의 절대 경로

	var cmd *exec.Cmd
	var cmdStr string // 사용자가 볼 명령어

	if runtime.GOOS == "windows" {
		// Windows의 move 명령어
		// 예: cmd /C move "C:\old_dir\old_file.txt" "C:\old_dir\new_file.txt"
		// cmd.Path 대신 exec.Command를 바로 사용
		cmd = exec.Command("cmd", "/C", "move", oldAbsPath, newAbsPath)
		cmdStr = fmt.Sprintf("move \"%s\" \"%s\"", oldAbsPath, newAbsPath)
	} else {
		// Linux/macOS의 mv 명령어
		// 예: mv -vi "/path/to/old_file.txt" "/path/to/new_file.txt"
		// -vi 옵션은 interactive (덮어쓸지 물어봄) 및 verbose (실행 결과 출력)
		cmd = exec.Command("mv", "-vi", oldAbsPath, newAbsPath)
		cmdStr = fmt.Sprintf("mv -vi \"%s\" \"%s\"", oldAbsPath, newAbsPath)
	}

	// 사용자에게 어떤 명령어가 실행될지 메시지를 보여줄 수 있습니다.
	// message.Info("실행될 명령어: " + cmdStr)

	// 명령어 실행 및 에러 처리
	output, err := cmd.CombinedOutput() // 표준 출력과 에러 출력을 함께 받습니다.
	if err != nil {
		// 에러 발생 시 Goful 메시지 창에 에러를 표시합니다.
		message.Error(fmt.Errorf("Cmd error (명령 에러) (%s): %v\n%s", cmdStr, err, string(output)))
	} else {
		// 성공 메시지 (선택 사항)
		message.Info("Renamed (변경 완료): " + string(output))
	}

	// 이름 변경 작업 완료 후 화면 갱신
	m.Goful.Dir().Reset()
	m.Goful.Workspace().ReloadAll()

	c.Exit() // cmdline 모드를 종료하고 Goful의 기본 UI로 돌아갑니다.
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
	var srcToRemove string
	if !g.Dir().IsMark() {
		srcToRemove = g.File().Name()
	}
	// removeMode를 초기화할 때 src 필드를 바로 채워줍니다.
	c := cmdline.New(&removeMode{g, srcToRemove}, g)

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

// Remove2 starts the remove mode.
func (g *Goful) Remove2() {
	var srcToRemove string
	if !g.Dir().IsMark() {
		srcToRemove = g.File().Name()
	}
	// removeMode를 초기화할 때 src 필드를 바로 채워줍니다.
	c := cmdline.New(&remove2Mode{g, srcToRemove}, g)

	g.next = c
}

type remove2Mode struct {
	*Goful
	src string
}

func (m *remove2Mode) String() string { return "remove2" }
func (m *remove2Mode) Prompt() string {
	if m.Dir().IsMark() {
		return fmt.Sprintf("Remove %d files to Trash? (휴지동에 이동?) [Y/n] ", m.Dir().MarkCount())
	} else {
		return fmt.Sprintf("Remove %s to Trash? (휴지통에 이동) [Y/n] ", shortName(filepath.Base(m.src), 10))
	}
}
func (m *remove2Mode) Draw(c *cmdline.Cmdline) { c.DrawLine() }

func (m *remove2Mode) Run(c *cmdline.Cmdline) {
	if marked := m.Dir().IsMark(); marked || m.src != "" {
		switch c.String() {
		case "y", "Y", "": // 'y' 또는 엔터 입력 시 삭제 진행
			var pathsToDelete []string
			if marked {
				pathsToDelete = m.Dir().MarkfilePaths()
			} else {
				pathsToDelete = []string{m.src}
			}

			for _, path := range pathsToDelete {
				// 파일을 삭제하기 전에 절대 경로로 변환합니다.
				absPath, err := filepath.Abs(path)
				if err != nil {
					fmt.Printf("Error: No abs path (오류: 절대경로)(%s) : %v\n", path, err)
					continue // 다음 파일로 넘어갑니다.
				}

				var cmd *exec.Cmd
				var cmdStr string // 사용자에게 보여줄 명령어 문자열

				if runtime.GOOS == "windows" {
					cmd = exec.Command("recycle", "-s", absPath)
					cmdStr = fmt.Sprintf(`recycle -s "%s"`, absPath)
					// fmt.Printf("Windows: '%s' 파일을 휴지통으로 이동합니다.\n", absPath)
				} else if runtime.GOOS == "darwin" {
					// macOS: osascript에 절대 경로를 전달합니다.
					cmd = exec.Command("osascript", "-e", fmt.Sprintf(`tell app "Finder" to delete POSIX file "%s"`, absPath))
					cmdStr = fmt.Sprintf(`osascript -e 'tell app "Finder" to delete POSIX file "%s"'`, absPath)
					// fmt.Printf("macOS: '%s' 파일을 휴지통으로 이동합니다.\n", absPath)
				} else {
					// Linux 등 그 외 OS: rm 명령어를 사용하여 영구 삭제
					cmd = exec.Command("trash", absPath) // 주의: 영구 삭제!
					cmdStr = fmt.Sprintf(`trash "%s"`, absPath)
					// fmt.Printf("Linux/Other: '%s' 파일을 영구 삭제합니다.\n", absPath)
				}

				if cmd != nil {
					output, err := cmd.CombinedOutput()
					if err != nil {
						message.Info("Delete error (삭제 에러): " + cmdStr + " Run error (실행 에러): " + err.Error())
						// fmt.Printf("오류: '%s' 실행 에러: %v\n%s\n", cmdStr, err, string(output))
					} else {
						output = output
						// message.Info("Deleted (삭제성공): " + cmdStr + " Cmd complete (명령 완료): " + string(output))
						// fmt.Printf("성공: '%s' 명령 실행 완료.\n%s\n", cmdStr, string(output))
					}
				} else {
					// fmt.Printf("오류: 지원되지 않는 운영체제 또는 삭제 명령을 구성할 수 없습니다.\n")
				}
			}
			// 이름 변경 작업 완료 후 화면 갱신
			m.Goful.Dir().Reset()
			m.Goful.Workspace().ReloadAll()
			c.Exit()
		case "n", "N":
			c.Exit()
		default:
			c.SetText("")
		}
	} else {
		// m.src = c.String()
		// c.SetText("")
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
		// Windows에서는 권한 설정을 하지 않으므로 바로 폴더 생성
		if runtime.GOOS == "windows" {
			return ""
		}
		return "Mode(권한) default 755: "
	}
	return "Make directory(새폴더): "
}
func (m *mkdirMode) Draw(c *cmdline.Cmdline) { c.DrawLine() }
func (m *mkdirMode) Run(c *cmdline.Cmdline) {
	if m.path != "" {
		// Windows에서는 권한 설정 없이 바로 폴더 생성
		if runtime.GOOS == "windows" {
			if err := os.MkdirAll(m.path, 0755); err != nil {
				message.Error(err)
			}
			message.Info("Made directory(폴더만듬) " + m.path)
			c.Exit()
			m.Workspace().ReloadAll()
			return
		}

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
		m.Workspace().ReloadAll()
		c.Exit()
	} else {
		m.path = c.String()
		// Windows에서는 첫 번째 입력 후 바로 폴더 생성
		if runtime.GOOS == "windows" {
			if err := os.MkdirAll(m.path, 0755); err != nil {
				message.Error(err)
			}
			message.Info("Made directory(폴더만듬) " + m.path)
			m.Workspace().ReloadAll()
			c.Exit()
			return
		}
		c.SetText("")
	}
}

// Mkdir2 starts the mode for creating a new directory.
func (g *Goful) Mkdir2() {
	// util.RemoveExt(g.File().Name())을 기본 프롬프트 값으로 설정
	defaultDirName := util.RemoveExt(g.File().Name())
	c := cmdline.New(&mkdir2Mode{Goful: g}, g)
	c.SetText(defaultDirName)         // 프롬프트에 기본 폴더명 제안
	c.MoveCursor(len(defaultDirName)) // 커서를 텍스트 끝으로 이동
	g.next = c
}

type mkdir2Mode struct {
	*Goful // Goful 인스턴스에 접근하기 위해 포함
}

func (m *mkdir2Mode) String() string          { return "mkdir2" }
func (m *mkdir2Mode) Prompt() string          { return "Make Dir (폴더 생성) -> " }
func (m *mkdir2Mode) Draw(c *cmdline.Cmdline) { c.DrawLine() }

// Run 메서드가 실제 폴더 생성 로직을 포함합니다.
func (m *mkdir2Mode) Run(c *cmdline.Cmdline) {
	newDirName := c.String() // 사용자로부터 입력받거나 수정된 새 폴더 이름
	if newDirName == "" {
		message.Info("error: Empty name (이름 없음)")
		c.Exit()
		return
	}

	// 현재 디렉토리 경로와 새 폴더 이름을 결합하여 전체 경로를 만듭니다.
	targetDirPath := filepath.Join(m.Goful.Dir().Path, newDirName)

	var shellCmd string
	var args []string
	var cmdStr string // 사용자가 볼 명령어

	if runtime.GOOS == "windows" {
		// Windows: mkdir "새폴더이름"
		// exec.Command는 인자를 자동으로 쉘 스케이프 해주므로, 직접 따옴표를 넣지 않아도 됩니다.
		shellCmd = "cmd"
		args = []string{"/C", "mkdir", targetDirPath}
		cmdStr = fmt.Sprintf("mkdir \"%s\"", targetDirPath)
	} else {
		// Linux/macOS: mkdir -vp "새폴더이름"
		// -v: 생성된 디렉토리를 출력
		// -p: 부모 디렉토리가 없으면 함께 생성
		shellCmd = "mkdir"
		args = []string{"-vp", targetDirPath}
		cmdStr = fmt.Sprintf("mkdir -vp \"%s\"", targetDirPath)
	}

	// 명령어 실행 및 에러 처리
	// CombinedOutput()은 명령의 표준 출력과 표준 에러를 함께 반환합니다.
	output, err := exec.Command(shellCmd, args...).CombinedOutput()
	if err != nil {
		message.Error(fmt.Errorf("creation error (생성 에러) (%s): %v\n%s", cmdStr, err, string(output)))
	} else {
		message.Info("Folder created (폴더 생성 완료): " + string(output))
	}

	// 폴더 생성 작업 완료 후 화면 갱신
	m.Goful.Dir().Reset()
	m.Goful.Workspace().ReloadAll()

	c.Exit() // cmdline 모드를 종료하고 Goful의 기본 UI로 돌아갑니다.
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
	if runtime.GOOS == "windows" {
		message.Info("Windows doesn't need to CHMOD (윈도우는 권한설정 불필요)")
		return
	}
	if !g.Dir().IsMark() {
		// 단일 파일인 경우 파일 정보를 미리 설정
		file := g.File().Name()
		lstat, err := os.Lstat(file)
		if err == nil {
			g.next = cmdline.New(&chmodMode{g, lstat}, g)
			return
		}
	}
	g.next = cmdline.New(&chmodMode{g, nil}, g)
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
		name := m.fi.Name()
		shortName := shortName(name, 10)
		mode := m.fi.Mode()
		return fmt.Sprintf("Chmod(권한변경) %s %o -> ", shortName, mode&os.ModePerm)
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
		m.Workspace().ReloadAll()
		c.Exit()
	} else {
		// 이 부분은 더 이상 사용되지 않지만, 혹시 모를 경우를 위해 남겨둠
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
		path = util.ExpandPath(path)

		// 파일명이 포함된 경로인지 확인하고 폴더 경로만 추출
		dirPath := path
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			// 파일인 경우 상위 디렉토리로 이동
			dirPath = filepath.Dir(path)
			message.Info("Moving to file location (파일위치로 이동합니다): " + dirPath)
		}

		m.Dir().Chdir(dirPath)
		c.Exit()
	}
}

// HeaderPathEdit starts the path editing mode directly in the header path bar with completion and history.
func (g *Goful) HeaderPathEdit() {
	// 현재 디렉토리 경로를 초기값으로 설정
	currentPath := g.Dir().Path
	c := cmdline.New(&headerPathEditMode{g, currentPath}, g)
	c.SetText(filer.TildePath(currentPath))

	g.next = c
}

type headerPathEditMode struct {
	*Goful
	originalPath string
}

func (m *headerPathEditMode) String() string { return "headerpathedit" }
func (m *headerPathEditMode) Prompt() string { return "" }

func (m *headerPathEditMode) Draw(c *cmdline.Cmdline) {
	// 상단 헤더 위치에 경로 표시
	ws := m.Workspace()

	// 헤더 위치 계산
	_, y := m.LeftTop()
	xpos := 0
	for i, ws := range m.Workspaces {
		tabStr := fmt.Sprintf(" %s ", ws.Title)
		tabLen := len([]rune(tabStr))
		if m.Current == i {
			break
		}
		xpos += tabLen
	}
	xpos += len(fmt.Sprintf(" | "))

	// 현재 디렉토리 위치 계산
	width := (m.Width() - xpos) / len(ws.Dirs)
	for i := 0; i < ws.Focus; i++ {
		xpos += width
	}

	// 경로 텍스트 준비
	pathText := c.String()
	// 빈 문자열일 때는 현재 경로로 초기화하지 않음 (사용자가 지운 상태 유지)

	// 경로 텍스트를 너비에 맞게 조정
	pathText = util.ShortenPath(pathText, width-4)
	pathText = runewidth.Truncate(pathText, width-4, "~")
	pathText = runewidth.FillRight(pathText, width-4)

	// 경로 표시
	style := look.Default().Reverse(true)
	widget.SetCells(xpos, y, pathText, style)

	// 커서 위치 표시
	cursorPos := c.Cursor()
	if cursorPos < len(pathText) {
		widget.ShowCursor(xpos+cursorPos, y)
	}
}

func (m *headerPathEditMode) Run(c *cmdline.Cmdline) {
	if path := c.String(); path != "" {
		path = util.ExpandPath(path)

		// 파일명이 포함된 경로인지 확인하고 폴더 경로만 추출
		dirPath := path
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			// 파일인 경우 상위 디렉토리로 이동
			dirPath = filepath.Dir(path)
			message.Info("Moving to file location (파일위치로 이동합니다): " + dirPath)
		}

		m.Dir().Chdir(dirPath)
		c.Exit()
	} else {
		// 빈 경로인 경우 원래 경로로 복원
		m.Dir().Chdir(m.originalPath)
		c.Exit()
	}
}

// HeaderPathEdit 모드에서 자동완성을 위한 함수
func headerPathCompletion(c *cmdline.Cmdline) {
	// ~만 입력된 경우 자동으로 ~/로 보정
	if c.String() == "~" {
		c.SetText("~/")
		c.MoveCursor(2) // 커서를 맨 끝으로 이동
		return
	}

	// 현재 입력된 경로를 가져옴
	inputPath := c.String()

	// 경로를 디렉토리와 파일명으로 분리 (수정된 로직)
	var dirname, filename string

	// 마지막 슬래시 위치 찾기
	lastSlash := strings.LastIndex(inputPath, "/")
	if lastSlash == -1 {
		// 슬래시가 없으면 현재 디렉토리에서 검색
		dirname = "."
		filename = inputPath
	} else {
		// 슬래시가 있으면 해당 위치로 분리
		dirname = inputPath[:lastSlash]
		filename = inputPath[lastSlash+1:]

		// 디렉토리가 비어있으면 현재 디렉토리로 설정
		if dirname == "" {
			dirname = "."
		}
	}

	// ~를 홈 디렉토리로 확장
	if dirname == "~" {
		dirname = "~/"
	}

	// 절대 경로로 확장 (입력된 경로를 기준으로)
	dirname = util.ExpandPath(inputPath[:lastSlash+1])

	// 디렉토리 열기
	dir, err := os.Open(dirname)
	if err != nil {
		return
	}
	defer dir.Close()

	// 파일 목록 읽기
	files, err := dir.Readdir(-1)
	if err != nil {
		return
	}

	// 후보 목록 생성
	var candidates []string
	for _, f := range files {
		name := f.Name()
		if strings.HasPrefix(name, filename) {
			if f.IsDir() {
				name += "/"
			}
			candidates = append(candidates, name)
		}
	}

	// 정렬
	sort.Strings(candidates)

	// 후보가 없으면 종료
	if len(candidates) == 0 {
		return
	}

	// 후보가 하나뿐이면 자동으로 삽입
	if len(candidates) == 1 {
		// 현재 경로에서 파일명 부분을 찾아서 교체
		if lastSlash == -1 {
			c.SetText(candidates[0])
		} else {
			c.SetText(inputPath[:lastSlash+1] + candidates[0])
		}
		c.MoveCursor(len(c.String()))
		return
	}

	// 여러 후보가 있으면 기존 completion 시스템 사용
	c.StartCompletion()
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

	src := ifElseSting((runtime.GOOS == "windows"), `start '`+g.File().Path()+`' %c`, ifElseSting((runtime.GOOS == "darwin"), `open -a '`+g.File().Path()+`' %m`, `'`+g.File().Path()+`' %m`))
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
			m.myShortCut = ": (type one character, please 한글자만 입력해주세요)"
			c.SetText("")
		}
	}
}

const myAppFile = "~/.goful/myApp"

func writeMyAppToFile(path string, content string) {

	// file, err := os.Create(util.ExpandPath(path))
	file, err := os.OpenFile(util.ExpandPath(path), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		glippy.Set("Create error (생성 에러): " + err.Error())

		return
	}
	defer file.Close()

	// 파일 끝에 내용 추가
	if _, err := file.WriteString(content); err != nil {
		glippy.Set("Write error (쓰기 오류): " + err.Error())
		return
	}

}

func (g *Goful) OpenMyAppList(path string) {
	if path == "" {
		path = "~/.goful/myApp"
	}

	file, err := os.OpenFile(util.ExpandPath(path), os.O_RDONLY, os.FileMode(0644))
	if err != nil {
		fmt.Println("파일 열기 에러:", err)
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
			m.myShortCut = ": (type one character, please 한글자만 입력해주세요)"
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
		fmt.Println("Open error (열기 오류):", err)
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
		return "del your bookmark (지울까요)? '" + m.myShortCutAndBookmarkName[m.myShortCut] + "' [Y, n] : "
	}
}

// func (m *delMyBookmarkMode) Prompt() string          { return fmt.Sprintf("delMyBookmark(이름변경) %s -> ", m.src) }
func (m *delMyBookmarkMode) Draw(c *cmdline.Cmdline) { c.DrawLine() }
func (m *delMyBookmarkMode) Run(c *cmdline.Cmdline) {
	if m.myShortCutAndBookmarkName[c.String()] != "" {
		m.isInList = true
	}
	if len(m.myShortCutAndBookmarkName) == 0 {
		message.Info("No bookmark to delete...(지울 것이 없네요)")
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

// UnZipToHere는 현재 디렉토리에 압축을 푸는 모드를 시작합니다.
func (g *Goful) UnZipToHere() {
	// 현재 파일이 압축 파일인지 확인하는 로직 추가 필요
	c := cmdline.New(&unZipToHereMode{Goful: g}, g)

	// 압축 파일명 (확장자 제외) 추출
	fileName := g.File().Name()
	fileNameWithoutExt := strings.TrimSuffix(fileName, filepath.Ext(fileName))

	// cmdline에 '현재 디렉토리/압축파일명(확장자제외)'를 기본 값으로 설정
	c.SetText(filepath.Join(g.Dir().Path, fileNameWithoutExt))
	g.next = c
}

type unZipToHereMode struct {
	*Goful
}

func (m *unZipToHereMode) String() string { return "unZipToHere" }

func (m *unZipToHereMode) Prompt() string {
	// 현재 파일이 압축 해제할 대상임을 표시
	return fmt.Sprintf("Extract '%s' to here(이 창에 풀기) -> ", shortName(m.File().Name(), 10))
}

func (m *unZipToHereMode) Draw(c *cmdline.Cmdline) { c.DrawLine() }

func (m *unZipToHereMode) Run(c *cmdline.Cmdline) {
	archivePath := m.File().Path() // 현재 선택된 파일 (압축 파일)
	// 목적지 경로 (cmdline에 표시된 텍스트, 기본값은 '현재 디렉토리/압축파일명(확장자제외)')
	destDir := c.String()

	// 7z x '%~F' -o'%~D/%~x'`
	// 여기서 '%~F'는 archivePath, '%~D/%~x'`는 destDir을 의미합니다.
	m.extractArchive(archivePath, destDir)
	c.Exit() // 압축 해제 시작 후 cmdline 종료
}

// extractArchive는 쉘 명령을 통해 압축 파일을 해제하고 완료 여부를 Goful에 알립니다.
func (m *unZipToHereMode) extractArchive(archivePath, destDir string) {
	completionChan := make(chan string)

	go func() {
		defer close(completionChan)

		var shellCmd string
		var args []string

		// Windows와 Linux/macOS 모두 7z 명령은 동일하게 작동합니다.
		shellCmd = "7z"
		// x: 압축 해제 (원본 폴더 구조 유지)
		// -o<path>: 출력 디렉토리 설정 (공백 없이 붙여야 함)
		// -y: 모든 질문에 예(Yes)로 자동 응답 (덮어쓰기 등)
		args = []string{"x", archivePath, "-o" + destDir, "-y"}

		cmd := exec.Command(shellCmd, args...)

		if err := cmd.Run(); err != nil {
			completionChan <- fmt.Sprintf("Unzip error (압축 해제 오류): %v", err)
		} else {
			fileName := shortName(filepath.Base(archivePath), 20)
			completionChan <- fmt.Sprintf("Unzipped (압축 해제 완료): %s", fileName)
		}
	}()

	go func() {
		msg := <-completionChan
		message.Info("[Status]: " + msg)
	}()
}

// UnzipToOtherPane은 반대쪽 패널의 디렉토리에 압축을 푸는 모드를 시작합니다.
func (g *Goful) UnzipToOtherPane() {
	// 현재 파일이 압축 파일인지 확인하는 로직 추가 필요
	c := cmdline.New(&unzipToOtherPaneMode{Goful: g}, g)

	// 압축 파일명 (확장자 제외) 추출
	fileName := g.File().Name()
	fileNameWithoutExt := strings.TrimSuffix(fileName, filepath.Ext(fileName))

	// cmdline에 '반대쪽 패널 디렉토리/압축파일명(확장자제외)'를 기본 값으로 설정
	c.SetText(filepath.Join(g.Workspace().NextDir().Path, fileNameWithoutExt))
	g.next = c
}

type unzipToOtherPaneMode struct {
	*Goful
}

func (m *unzipToOtherPaneMode) String() string { return "unzipToOtherPane" }

func (m *unzipToOtherPaneMode) Prompt() string {
	// 현재 파일이 압축 해제할 대상임을 표시
	return fmt.Sprintf("Extract '%s' to Other Pane (다른 창에 압축 풀기)-> ", shortName(m.File().Name(), 10))
}

func (m *unzipToOtherPaneMode) Draw(c *cmdline.Cmdline) { c.DrawLine() }

func (m *unzipToOtherPaneMode) Run(c *cmdline.Cmdline) {
	archivePath := m.File().Path() // 현재 선택된 파일 (압축 파일)
	// 목적지 경로 (cmdline에 표시된 텍스트, 기본값은 '반대쪽 패널 디렉토리/압축파일명(확장자제외)')
	destDir := c.String()

	// 7z x '%~F' -o'%~D2/%~x'`
	// 여기서 '%~F'는 archivePath, '%~D2/%~x'`는 destDir을 의미합니다.
	m.extractArchive(archivePath, destDir)
	c.Exit() // 압축 해제 시작 후 cmdline 종료
}

// extractArchive는 쉘 명령을 통해 압축 파일을 해제하고 완료 여부를 Goful에 알립니다.
// unZipToHereMode와 동일한 로직을 사용하므로, 별도 함수로 분리하거나
// unZipToHereMode의 extractArchive를 재활용할 수 있습니다.
// 여기서는 코드 재사용을 위해 unZipToHereMode의 메서드를 그대로 호출합니다.
func (m *unzipToOtherPaneMode) extractArchive(archivePath, destDir string) {
	// unZipToHereMode의 extractArchive와 동일
	// Goful의 메서드를 호출하는 방식으로 재활용합니다.
	tempMode := &unZipToHereMode{Goful: m.Goful}
	tempMode.extractArchive(archivePath, destDir)
}

// ZipToHere는 현재 디렉토리에 마크된 파일/폴더 또는 현재 파일을 압축하는 모드를 시작합니다.
func (g *Goful) ZipToHere() {
	c := cmdline.New(&ZipToHereMode{Goful: g}, g)

	// 기본 압축 파일명 설정: 현재 디렉토리명.zip
	// 마크된 파일이 있다면, 여러 파일 압축임을 표시할 수 있지만,
	// 7z a 명령어는 단일 출력 파일명을 받으므로,
	// 여기서는 기본적으로 현재 디렉토리 이름을 따르도록 합니다.

	var defaultZipFileName string
	if g.Dir().IsMark() && g.Dir().MarkCount() == 1 {
		marked := g.Dir().MarkfilePaths()
		base := filepath.Base(marked[0])
		if info, err := os.Stat(marked[0]); err == nil && info.IsDir() {
			defaultZipFileName = base + ".zip"
		} else {
			ext := filepath.Ext(base)
			nameWithoutExt := base[:len(base)-len(ext)]
			defaultZipFileName = nameWithoutExt + ".zip"
		}
	} else if g.Dir().IsMark() && g.Dir().MarkCount() > 1 {
		currentDirName := filepath.Base(g.Dir().Path)
		defaultZipFileName = currentDirName + ".zip"
	} else {
		fileName := g.File().Name()
		if g.File().IsDir() {
			defaultZipFileName = fileName + ".zip"
		} else {
			ext := filepath.Ext(fileName)
			nameWithoutExt := fileName[:len(fileName)-len(ext)]
			defaultZipFileName = nameWithoutExt + ".zip"
		}
	}

	// cmdline에 '현재 디렉토리/현재디렉토리명.zip'을 기본 값으로 설정
	c.SetText(filepath.Join(g.Dir().Path, defaultZipFileName))
	g.next = c
}

type ZipToHereMode struct {
	*Goful
}

func (m *ZipToHereMode) String() string { return "ZipToHere" }

func (m *ZipToHereMode) Prompt() string {
	var targets string
	if m.Dir().IsMark() {
		targets = fmt.Sprintf("%d items", m.Dir().MarkCount())
	} else {
		targets = shortName(m.File().Name(), 10)
	}
	return fmt.Sprintf("Zip %s to -> ", targets)
}

func (m *ZipToHereMode) Draw(c *cmdline.Cmdline) { c.DrawLine() }

func (m *ZipToHereMode) Run(c *cmdline.Cmdline) {
	destZipPath := c.String() // cmdline에 표시된 텍스트 (압축될 .zip 파일의 전체 경로)

	var sources []string
	if m.Dir().IsMark() {
		sources = m.Dir().MarkfilePaths() // 마크된 파일/폴더 목록
	} else {
		sources = []string{m.File().Path()} // 단일 파일/폴더
	}

	m.createArchive(destZipPath, sources...) // 압축 시작
	c.Exit()                                 // 압축 시작 후 cmdline 종료
}

// createArchive는 쉘 명령을 통해 파일을 압축하고 완료 여부를 Goful에 알립니다.
func (m *ZipToHereMode) createArchive(destZipPath string, sources ...string) {
	completionChan := make(chan string)

	go func() {
		defer close(completionChan)

		var shellCmd string
		var args []string

		// 7z a '압축파일.zip' '대상1' '대상2' ...
		shellCmd = "7z"
		// a: 압축 (add) 명령어
		// -tzip: ZIP 형식으로 압축 (기본적으로 7z 형식)
		// -y: 모든 질문에 예(Yes)로 자동 응답 (덮어쓰기 등)
		args = []string{"a", "-tzip", "-y", destZipPath}

		// 소스 파일들을 절대 경로로 변환
		var absoluteSources []string
		for _, source := range sources {
			if absPath, err := filepath.Abs(source); err == nil {
				absoluteSources = append(absoluteSources, absPath)
			} else {
				absoluteSources = append(absoluteSources, source)
			}
		}
		args = append(args, absoluteSources...)

		cmd := exec.Command(shellCmd, args...)

		// 현재 작업 디렉토리를 압축할 파일이 있는 디렉토리로 설정
		if len(sources) > 0 {
			if absPath, err := filepath.Abs(filepath.Dir(sources[0])); err == nil {
				cmd.Dir = absPath
			}
		}

		// 에러 출력 캡처
		var stderr bytes.Buffer
		cmd.Stderr = &stderr

		if err := cmd.Run(); err != nil {
			errorOutput := stderr.String()
			completionChan <- fmt.Sprintf("Zip error (압축 에러): %v\n에러 출력: %s", err, errorOutput)
		} else {
			fileName := shortName(filepath.Base(destZipPath), 20)
			completionChan <- fmt.Sprintf("Ziped (압축 완료): %s", fileName)
		}
	}()

	go func() {
		msg := <-completionChan
		message.Info("[Status]: " + msg)
	}()
}

// ZipToOtherPane은 마크된 파일/폴더 또는 현재 파일을 반대쪽 패널의 디렉토리에 압축하는 모드를 시작합니다.
func (g *Goful) ZipToOtherPane() {
	c := cmdline.New(&zipToOtherPaneMode{Goful: g}, g)

	// 선택된 파일/폴더 수에 따라 기본 압축 파일명 결정
	var defaultZipFileName string
	if g.Dir().IsMark() && g.Dir().MarkCount() == 1 {
		marked := g.Dir().MarkfilePaths()
		base := filepath.Base(marked[0])
		if info, err := os.Stat(marked[0]); err == nil && info.IsDir() {
			defaultZipFileName = base + ".zip"
		} else {
			ext := filepath.Ext(base)
			nameWithoutExt := base[:len(base)-len(ext)]
			defaultZipFileName = nameWithoutExt + ".zip"
		}
	} else if g.Dir().IsMark() && g.Dir().MarkCount() > 1 {
		currentDirName := filepath.Base(g.Dir().Path)
		defaultZipFileName = currentDirName + ".zip"
	} else {
		fileName := g.File().Name()
		if g.File().IsDir() {
			defaultZipFileName = fileName + ".zip"
		} else {
			ext := filepath.Ext(fileName)
			nameWithoutExt := fileName[:len(fileName)-len(ext)]
			defaultZipFileName = nameWithoutExt + ".zip"
		}
	}

	// cmdline에 '반대쪽 패널 디렉토리/현재디렉토리명.zip'을 기본 값으로 설정
	c.SetText(filepath.Join(g.Workspace().NextDir().Path, defaultZipFileName))
	g.next = c
}

type zipToOtherPaneMode struct {
	*Goful
}

func (m *zipToOtherPaneMode) String() string { return "zipToOtherPane" }

func (m *zipToOtherPaneMode) Prompt() string {
	var targets string
	if m.Dir().IsMark() {
		targets = fmt.Sprintf("%d items", m.Dir().MarkCount())
	} else {
		targets = shortName(m.File().Name(), 10)
	}
	return fmt.Sprintf("Zip %s to Other Pane (반대창에 압축) -> ", targets)
}

func (m *zipToOtherPaneMode) Draw(c *cmdline.Cmdline) { c.DrawLine() }

func (m *zipToOtherPaneMode) Run(c *cmdline.Cmdline) {
	destZipPath := c.String() // cmdline에 표시된 텍스트 (압축될 .zip 파일의 전체 경로)

	var sources []string
	if m.Dir().IsMark() {
		sources = m.Dir().MarkfilePaths() // 마크된 파일/폴더 목록
	} else {
		sources = []string{m.File().Path()} // 단일 파일/폴더
	}

	m.createArchive(destZipPath, sources...) // 압축 시작
	c.Exit()                                 // 압축 시작 후 cmdline 종료
}

// createArchive는 쉘 명령을 통해 파일을 압축하고 완료 여부를 Goful에 알립니다.
// ZipToHereMode와 동일한 로직을 사용하므로, 별도 함수로 분리하거나
// ZipToHereMode의 createArchive를 재활용할 수 있습니다.
// 여기서는 코드 재사용을 위해 ZipToHereMode의 메서드를 그대로 호출합니다.
func (m *zipToOtherPaneMode) createArchive(destZipPath string, sources ...string) {
	// ZipToHereMode의 createArchive와 동일
	// Goful의 메서드를 호출하는 방식으로 재활용합니다.
	tempMode := &ZipToHereMode{Goful: m.Goful}
	tempMode.createArchive(destZipPath, sources...)
}

// PasteCopy starts the paste copy mode.
func (g *Goful) PasteCopy() {
	// 클립보드에서 파일 경로 가져오기

	value, _ := getClipboardContent()
	// message.Info(value)

	if value == "" {
		message.Info("Clipboard is empty (클립보드가 비었습니다)")
		return
	}

	c := cmdline.New(&pasteCopyMode{Goful: g, src: ""}, g)
	var filePaths []string
	mode := &pasteCopyMode{Goful: g} // 임시 인스턴스
	switch runtime.GOOS {
	case "windows":
		filePaths = mode.arrangeFilePathForWindows(value)
	case "darwin":
		filePaths = mode.arrangeFilePathForMac(value)
	default:
		filePaths = mode.arrangeFilePathForLin(value)
	}
	if len(filePaths) == 1 {
		// 단일 파일 복사 시에는 (pane 경로 + 파일명)으로 기본값 세팅
		dstPath := filepath.Join(g.Workspace().Dir().Path, filepath.Base(filePaths[0]))
		c.SetText(dstPath)
	} else {
		// 여러 파일 복사 시에도 목적지 폴더를 바로 표시
		c.SetText(g.Workspace().Dir().Path)
	}
	g.next = c
}

type pasteCopyMode struct {
	*Goful
	src string // 이 필드는 이제 첫 번째 엔터 입력 전까지는 비어있지 않고, src 파일(들) 경로를 저장
}

func (m *pasteCopyMode) String() string { return "pastecopy" }

func (m *pasteCopyMode) Prompt() string {
	if m.src == "" { // 아직 src가 설정되지 않은 초기 상태
		// OS별로 파일 경로 정리
		value, _ := getClipboardContent()
		// message.Info((value)) // 불필요한 디버그 메시지 제거
		var filePaths []string
		switch runtime.GOOS {
		case "windows":
			filePaths = m.arrangeFilePathForWindows(value)
		case "darwin":
			filePaths = m.arrangeFilePathForMac(value)
		default:
			filePaths = m.arrangeFilePathForLin(value)
		}

		if len(filePaths) == 0 {
			// message.Info(fmt.Sprintf("No path in clipboard1 (클립보드에 경로 없음): %s)", value)) // 불필요한 메시지 제거
			return ""
		} else if len(filePaths) == 1 {
			return fmt.Sprintf("Paste Copy(클립보드에서 복사) %s -> ", shortName(filepath.Base(filePaths[0]), 20))
		} else {
			// 여러 파일일 때 개수만 표시
			return fmt.Sprintf("Paste Copy(클립보드에서 복사) %d files -> ", len(filePaths))
		}
	}
	return "pasteCopy(복사) -> "
}

func (m *pasteCopyMode) Draw(c *cmdline.Cmdline) { c.DrawLine() }

func (m *pasteCopyMode) Run(c *cmdline.Cmdline) {

	value, _ := getClipboardContent()
	if value == "" {
		message.Info("Clipboard is empty (클립보드가 비었습니다)")
		c.Exit()
		return
	}
	if m.src == "" { // 첫 번째 엔터 입력 (현재 파일/마크된 파일 -> 목적지 폴더)
		var srcs []string

		switch runtime.GOOS {
		case "windows":
			srcs = m.arrangeFilePathForWindows(value)
		case "darwin":
			srcs = m.arrangeFilePathForMac(value)
			// message.Info(fmt.Sprintf("Parsed33 paths (%d): %v", len(srcs), srcs))

		default:
			srcs = m.arrangeFilePathForLin(value)
		}
		if len(srcs) == 0 {
			message.Info(fmt.Sprintf("No path in clipboard (클립보드에 경로 없음): %s)", value))
			c.Exit()
			return
		} // 디버깅: 파싱된 경로들 출력

		// 실제로 존재하는 경로만 필터링
		var realSrcs []string
		for _, s := range srcs {
			if _, err := os.Stat(s); err == nil {
				realSrcs = append(realSrcs, s)
			} else {
				message.Info(fmt.Sprintf("Path not found: %s", s))
			}
		}
		if len(realSrcs) == 0 {
			// message.Info("Path in clipboard not found (클립보드 파일 없음)")
			c.Exit()
			return
		}
		if len(realSrcs) < len(srcs) {
			message.Info(fmt.Sprintf("Some Paths excluded: %d found, %d total", len(realSrcs), len(srcs)))
		}

		// 디버깅: 실제 존재하는 경로들 출력
		// message.Info(fmt.Sprintf("Real paths (%d): %v", len(realSrcs), realSrcs))

		dst := c.String() // 현재 cmdline에 표시된 텍스트(목적지 경로)
		// 입력값이 파일이면 그 상위 폴더, 폴더면 그대로
		dstDir := dst
		if len(realSrcs) == 1 || (!m.Dir().IsMark() && len(realSrcs) == 1) {
			dstDir = filepath.Dir(dst)
		}
		if _, err := os.Stat(dstDir); os.IsNotExist(err) {
			err := os.MkdirAll(dstDir, 0755)
			if err != nil {
				message.Error(fmt.Errorf("Create error (생성 오류): %v", err))
				c.Exit()
				return
			}
		}
		// 디버깅: copy 함수 호출 정보
		// message.Info(fmt.Sprintf("Copying to dst: %s, from srcs: %v", dst, realSrcs))
		m.copy(dst, realSrcs...)
		c.Exit()
	} else {
		dst := c.String()
		var srcs []string
		if m.Dir().IsMark() {
			srcs = m.Dir().MarkfilePaths()
		} else {
			srcs = []string{m.src} // m.src가 단일 파일 경로를 가지고 있다고 가정
		}
		dstDir := dst
		if len(srcs) == 1 || (!m.Dir().IsMark() && len(srcs) == 1) {
			dstDir = filepath.Dir(dst)
		}
		if _, err := os.Stat(dstDir); os.IsNotExist(err) {
			err := os.MkdirAll(dstDir, 0755)
			if err != nil {
				message.Error(fmt.Errorf("Create error (생성 오류): %v", err))
				c.Exit()
				return
			}
		}
		m.copy(dst, srcs...)
		c.Exit()
	}
}

// arrangeFilePathForWindows: Windows용 파일 경로 정리
func (m *pasteCopyMode) arrangeFilePathForWindows(str string) []string {
	var result []string

	// 줄바꿈으로 분리된 파일 경로들 처리 (새로운 getClipboardContent에서 제공)
	if strings.Contains(str, "\n") {
		paths := strings.Split(str, "\n")
		for _, path := range paths {
			path = strings.TrimSpace(path)
			if path != "" {
				result = append(result, path)
			}
		}
		return result
	}

	// 1. 따옴표로 감싸진 경로(큰따옴표 또는 작은따옴표) 우선 추출
	re := regexp.MustCompile(`"([^"]+)"|'([^']+)'`)
	matches := re.FindAllStringSubmatch(str, -1)
	for _, match := range matches {
		if match[1] != "" {
			result = append(result, match[1])
		} else if match[2] != "" {
			result = append(result, match[2])
		}
	}
	if len(result) > 0 {
		return result
	}

	// 2. 따옴표가 없는 경우: 드라이브 문자(C:, D: 등) 기준으로 경로 분리
	// (공백 기준 분리는 경로 내 공백이 있을 때 잘리므로 사용하지 않음)
	str = strings.ReplaceAll(str, `\\`, `/`)
	str = strings.ReplaceAll(str, `\`, `/`)
	str = strings.ReplaceAll(str, "\r\n", ` `)

	// 드라이브 문자 패턴 찾기 (예: C:/, D:/)
	driveRe := regexp.MustCompile(`([A-Za-z]:/)`)
	indices := driveRe.FindAllStringIndex(str, -1)

	if len(indices) == 0 {
		// 드라이브 문자가 없으면 전체를 하나의 경로로 취급
		path := strings.TrimSpace(str)
		if path != "" {
			result = append(result, path)
		}
		return result
	}

	for i := 0; i < len(indices); i++ {
		start := indices[i][0]
		var end int
		if i+1 < len(indices) {
			end = indices[i+1][0]
		} else {
			end = len(str)
		}
		path := strings.TrimSpace(str[start:end])
		if path != "" {
			result = append(result, path)
		}
	}
	return result
}

// arrangeFilePathForMac: macOS용 파일 경로 정리
func (m *pasteCopyMode) arrangeFilePathForMac(str string) []string {
	plainPrefixes := []string{
		"/Applications", "/Library", "/System", "/Users",
		"/Volumes", "/bin", "/cores", "/dev", "/etc",
		"/home", "/opt", "/private", "/sbin", "/tmp",
		"/usr", "/var",
	}
	// 경로가 넘어온 경우
	// 파일명에 / 또는 \ 이 있는지 검사 (경로 구분자가 아닌 실제 파일명 부분만)
	if strings.Contains(str, "/") || strings.Contains(str, "\\") {

		quotedPrefixes := []string{
			"'/Applications", "'/Library", "'/System", "'/Users",
			"'/Volumes", "'/bin", "'/cores", "'/dev", "'/etc",
			"'/home", "'/opt", "'/private", "'/sbin", "'/tmp",
			"'/usr", "'/var",
		}

		spaceQuotedPrefixes := []string{
			"' /Applications", "' /Library", "' /System", "' /Users",
			"' /Volumes", "' /bin", "' /cores", "' /dev", "' /etc",
			"' /home", "' /home", "' /private", "' /sbin", "' /tmp",
			"' /usr", "' /var",
		}

		doubleQuotedPrefixes := []string{
			"' '/Applications", "' '/Library", "' '/System", "' '/Users",
			"' '/Volumes", "' '/bin", "' '/cores", "' '/dev", "' '/etc",
			"' '/home", "' '/opt", "' '/private", "' '/sbin", "' '/tmp",
			"' '/usr", "' '/var",
		}

		var result []string
		var currentPath strings.Builder
		i := 0

		for i < len(str) {
			// 공백 건너뛰기 (경우에 따라)
			if unicode.IsSpace(rune(str[i])) && currentPath.Len() == 0 {
				i++
				continue
			}

			found := false

			// 4가지 유형의 접두사 확인 (우선순위 순)
			for _, prefixes := range [][]string{
				doubleQuotedPrefixes,
				spaceQuotedPrefixes,
				quotedPrefixes,
				plainPrefixes,
			} {
				for _, prefix := range prefixes {
					if i+len(prefix) <= len(str) && str[i:i+len(prefix)] == prefix {
						if currentPath.Len() > 0 {
							// 경로 정규화 전 후행 공백 제거
							rawPath := strings.TrimSpace(currentPath.String())
							cleanPath := util.NormalizeFileName(strings.ReplaceAll(rawPath, "\\", ""))
							result = append(result, cleanPath)
							currentPath.Reset()
						}

						// 접두사 처리
						var cleanedPrefix string
						switch {
						case strings.HasPrefix(prefix, "' '"):
							cleanedPrefix = strings.TrimSpace(prefix[2:])
						case strings.HasPrefix(prefix, "' "):
							cleanedPrefix = strings.TrimSpace(prefix[1:])
						case strings.HasPrefix(prefix, "'"):
							cleanedPrefix = strings.TrimSpace(prefix[1:])
						default:
							cleanedPrefix = strings.TrimSpace(prefix)
						}
						currentPath.WriteString(cleanedPrefix)

						i += len(prefix)
						found = true
						break
					}
				}
				if found {
					break
				}
			}

			if !found {
				// 백슬래시는 건너뛰고 일반 문자만 추가
				if str[i] != '\\' {
					currentPath.WriteByte(str[i])
				}
				i++
			}
		}

		if currentPath.Len() > 0 {
			lastPath := currentPath.String()
			// 후행 공백 및 따옴표 제거
			lastPath = strings.TrimSpace(lastPath)
			if strings.HasSuffix(lastPath, "'") {
				lastPath = lastPath[:len(lastPath)-1]
				lastPath = strings.TrimSpace(lastPath)
			}
			// 백슬래시 제거 후 정규화
			lastPath = util.NormalizeFileName(strings.ReplaceAll(lastPath, "\\", ""))
			result = append(result, lastPath)
		}

		return result
	}

	// 클립보드에 파일로 넘어온 경우

	// 1단계: AppleScript로 첫 번째 파일의 절대 경로 얻기
	script := `
	try
		set fileData to the clipboard as «class furl»
		
		if class of fileData is list then
			set firstFile to item 1 of fileData
			return POSIX path of firstFile
		else
			return POSIX path of fileData
		end if
	on error
		return "ERROR:No file found"
	end try
	`

	cmd := exec.Command("osascript", "-e", script)
	output, err := cmd.Output()
	if err != nil {
		return []string{fmt.Sprintf("AppleScript execution failed / AppleScript 실행 실패: %v", err)}
	}

	firstFilePath := strings.TrimSpace(string(output))
	if strings.HasPrefix(firstFilePath, "ERROR:") {
		return []string{fmt.Sprintf("AppleScript error / AppleScript 오류: %s", firstFilePath)}
	}

	// 디렉토리 경로 추출
	var baseDir string
	if strings.HasSuffix(firstFilePath, "/") {
		// 첫 번째가 폴더인 경우: 그 폴더의 부모 디렉토리를 찾음
		trimmedPath := strings.TrimSuffix(firstFilePath, "/")
		lastSlash := strings.LastIndex(trimmedPath, "/")
		if lastSlash == -1 {
			return []string{fmt.Sprintf("Invalid folder path / 잘못된 폴더 경로: %s", firstFilePath)}
		}
		baseDir = trimmedPath[:lastSlash+1] // "/" 포함
	} else {
		// 첫 번째가 파일인 경우: 파일이 있는 디렉토리가 부모 디렉토리
		lastSlash := strings.LastIndex(firstFilePath, "/")
		if lastSlash == -1 {
			return []string{fmt.Sprintf("Invalid file path / 잘못된 파일 경로: %s", firstFilePath)}
		}
		baseDir = firstFilePath[:lastSlash+1] // "/" 포함
	}

	// 2단계: 전달받은 문자열에서 파일명들 추출
	if str == "" {
		return []string{"No filename found / 파일명을 찾을 수 없음"}
	}

	// 여러 구분자로 분리: \r\n, \r, \n
	var lines []string
	if strings.Contains(str, "\r\n") {
		lines = strings.Split(str, "\r\n")
	} else if strings.Contains(str, "\r") {
		lines = strings.Split(str, "\r")
	} else if strings.Contains(str, "\n") {
		lines = strings.Split(str, "\n")
	} else {
		lines = []string{str}
	}

	var fileNames []string
	for _, line := range lines {
		fileName := line
		// fileName := strings.TrimSpace(line)
		if fileName != "" {
			fileNames = append(fileNames, fileName)
		}
	}

	if len(fileNames) == 0 {
		return []string{"No filename found / 파일명을 찾을 수 없음"}
	}

	// // 파일명들에 / 또는 \ 이 있는지 검사
	// for _, fileName := range fileNames {
	// 	if strings.Contains(fileName, "/") || strings.Contains(fileName, "\\") {
	// 		message.Error(fmt.Errorf("⚠️ 파일명에 경로 구분자(/,\\)가 포함되어 있습니다. 처리할 수 없습니다."))
	// 		return []string{}
	// 	}
	// }

	// 3단계: 디렉토리 경로 + 파일명으로 완전한 경로 생성
	var paths []string

	for _, fileName := range fileNames {
		// 파일명이 이미 경로로 시작하는지 확인
		isFullPath := false
		for _, prefix := range plainPrefixes {
			if strings.HasPrefix(fileName, prefix) {
				isFullPath = true
				break
			}
		}

		var fullPath string
		if isFullPath {
			fullPath = fileName // 이미 전체 경로인 경우 그대로 사용
		} else {
			fullPath = baseDir + fileName // 아니면 baseDir 추가
		}
		paths = append(paths, util.NormalizeFileName(fullPath))
	}

	return paths
}

// arrangeFilePathForLin: Linux용 파일 경로 정리
func (m *pasteCopyMode) arrangeFilePathForLin(str string) []string {
	var result []string

	if strings.Contains(str, "\n") {
		// 줄바꿈이 있는 경우 (복사된 파일 경로)
		// /Users\n/System\n/Applications -> /Users, /System, /Applications
		paths := strings.Split(str, "\n")
		for _, path := range paths {
			path = strings.TrimSpace(path)
			if path != "" {
				result = append(result, path)
			}
		}
	} else {
		// 줄바꿈이 없는 경우 (복사된 파일)
		// /Users /System /Applications -> /Users, /System, /Applications
		if str == "" {
			message.Info("===your path is EMPTY ===")
			return result
		}

		var indices []int
		indices = append(indices, 0)
		for i := 1; i < len(str); i++ {
			if str[i-1] == ' ' && str[i] == '/' {
				indices = append(indices, i-1)
			}
		}
		indices = append(indices, len(str))

		for i := 0; i < len(indices)-1; i++ {
			path := strings.TrimSpace(str[indices[i]:indices[i+1]])
			if path != "" {
				result = append(result, path)
			}
		}
	}

	return result
}

// PasteMove starts the paste move mode.
func (g *Goful) PasteMove() {
	c := cmdline.New(&pasteMoveMode{Goful: g, src: ""}, g)
	// 클립보드에서 파일 경로 가져오기
	value, _ := getClipboardContent()
	var filePaths []string
	mode := &pasteMoveMode{Goful: g} // 임시 인스턴스
	switch runtime.GOOS {
	case "windows":
		filePaths = mode.arrangeFilePathForWindows(value)
	case "darwin":
		filePaths = mode.arrangeFilePathForMac(value)
	default:
		filePaths = mode.arrangeFilePathForLin(value)
	}
	if len(filePaths) == 1 {
		// 단일 파일 이동 시에는 (pane 경로 + 파일명)으로 기본값 세팅
		dstPath := filepath.Join(g.Workspace().Dir().Path, filepath.Base(filePaths[0]))
		c.SetText(dstPath)
	} else {
		// 여러 파일 이동 시에도 목적지 폴더를 바로 표시
		c.SetText(g.Workspace().Dir().Path)
	}
	g.next = c
}

type pasteMoveMode struct {
	*Goful
	src string // 이 필드는 이제 첫 번째 엔터 입력 전까지는 비어있지 않고, src 파일(들) 경로를 저장
}

func (m *pasteMoveMode) String() string { return "pastemove" }

func (m *pasteMoveMode) Prompt() string {
	if m.src == "" { // 아직 src가 설정되지 않은 초기 상태
		// OS별로 파일 경로 정리
		value, _ := getClipboardContent()

		var filePaths []string
		switch runtime.GOOS {
		case "windows":
			filePaths = m.arrangeFilePathForWindows(value)
		case "darwin":
			filePaths = m.arrangeFilePathForMac(value)
		default:
			filePaths = m.arrangeFilePathForLin(value)
		}

		if len(filePaths) == 0 {
			// message.Info(fmt.Sprintf("No path in clipboar4d (클립보드에 경로 없음): %s)", value)) // 불필요한 메시지 제거
			return ""
		} else if len(filePaths) == 1 {
			return fmt.Sprintf("Paste Move(클립보드에서 이동) %s -> ", shortName(filepath.Base(filePaths[0]), 20))
		} else {
			// 여러 파일일 때 개수만 표시
			return fmt.Sprintf("Paste Move(클립보드에서 이동) %d files -> ", len(filePaths))
		}
	}
	return "pasteMove(이동) -> "
}

func (m *pasteMoveMode) Draw(c *cmdline.Cmdline) { c.DrawLine() }

func (m *pasteMoveMode) Run(c *cmdline.Cmdline) {
	value, _ := getClipboardContent()
	if value == "" {
		message.Info("Clipboard is empty (클립보드가 비었습니다)")
		c.Exit()
		return
	}
	if m.src == "" { // 첫 번째 엔터 입력 (현재 파일/마크된 파일 -> 목적지 폴더)
		var srcs []string

		switch runtime.GOOS {
		case "windows":
			srcs = m.arrangeFilePathForWindows(value)
		case "darwin":
			srcs = m.arrangeFilePathForMac(value)
		default:
			srcs = m.arrangeFilePathForLin(value)
		}
		if len(srcs) == 0 {
			message.Info(fmt.Sprintf("No path in clipboard (클립보드에 경로 없음): %s", value))
			c.Exit()
			return
		}

		// 실제로 존재하는 경로만 필터링
		var realSrcs []string
		for _, s := range srcs {
			if _, err := os.Stat(s); err == nil {
				realSrcs = append(realSrcs, s)
			} else {
				message.Info(fmt.Sprintf("Path not found: %s", s))
			}
		}
		if len(realSrcs) == 0 {
			c.Exit()
			return
		}
		if len(realSrcs) < len(srcs) {
			message.Info(fmt.Sprintf("Some Paths excluded: %d found, %d total", len(realSrcs), len(srcs)))
		}

		dst := c.String() // 현재 cmdline에 표시된 텍스트(목적지 경로)
		// 입력값이 파일이면 그 상위 폴더, 폴더면 그대로
		dstDir := dst
		if len(realSrcs) == 1 || (!m.Dir().IsMark() && len(realSrcs) == 1) {
			dstDir = filepath.Dir(dst)
		}
		if _, err := os.Stat(dstDir); os.IsNotExist(err) {
			err := os.MkdirAll(dstDir, 0755)
			if err != nil {
				message.Error(fmt.Errorf("Create error (생성 오류): %v", err))
				c.Exit()
				return
			}
		}
		m.move(dst, realSrcs...)
		c.Exit()
	} else {
		dst := c.String()
		var srcs []string
		if m.Dir().IsMark() {
			srcs = m.Dir().MarkfilePaths()
		} else {
			srcs = []string{m.src} // m.src가 단일 파일 경로를 가지고 있다고 가정
		}
		dstDir := dst
		if len(srcs) == 1 || (!m.Dir().IsMark() && len(srcs) == 1) {
			dstDir = filepath.Dir(dst)
		}
		if _, err := os.Stat(dstDir); os.IsNotExist(err) {
			err := os.MkdirAll(dstDir, 0755)
			if err != nil {
				message.Error(fmt.Errorf("Create error (생성 오류): %v", err))
				c.Exit()
				return
			}
		}
		m.move(dst, srcs...)
		c.Exit()
	}
}

// arrangeFilePathForWindows: Windows용 파일 경로 정리 (PasteMove용)
func (m *pasteMoveMode) arrangeFilePathForWindows(str string) []string {
	return (*pasteCopyMode)(m).arrangeFilePathForWindows(str)
}

// arrangeFilePathForMac: macOS용 파일 경로 정리 (PasteMove용)
func (m *pasteMoveMode) arrangeFilePathForMac(str string) []string {
	return (*pasteCopyMode)(m).arrangeFilePathForMac(str)
}

// arrangeFilePathForLin: Linux용 파일 경로 정리 (PasteMove용)
func (m *pasteMoveMode) arrangeFilePathForLin(str string) []string {
	return (*pasteCopyMode)(m).arrangeFilePathForLin(str)
}

// getWindowsClipboardFiles는 Windows에서 클립보드의 파일 경로를 가져오는 함수입니다.
func getWindowsClipboardFiles() ([]string, error) {
	// PowerShell을 사용하여 Windows 클립보드에서 파일 경로를 가져옵니다 (최적화된 버전)
	command := `
		try {
			# 인코딩 설정 (한글 처리를 위해 필수)
			[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
			[Console]::InputEncoding = [System.Text.Encoding]::UTF8
			$OutputEncoding = [System.Text.Encoding]::UTF8
			
			Add-Type -AssemblyName System.Windows.Forms
			$clipboard = [System.Windows.Forms.Clipboard]::GetDataObject()
			
			if ($clipboard.GetDataPresent([System.Windows.Forms.DataFormats]::FileDrop)) {
				$files = $clipboard.GetData([System.Windows.Forms.DataFormats]::FileDrop)
				$files -join "` + "`n" + `"
			} else {
				$text = [System.Windows.Forms.Clipboard]::GetText()
				if ($text -and (Test-Path $text)) { $text } else { "" }
			}
		} catch { "" }
	`

	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", command)
	cmd.Env = append(os.Environ(), "PYTHONIOENCODING=utf-8")
	output, err := cmd.Output()
	if err != nil {
		// PowerShell 실행 실패 시 빈 결과 반환
		return []string{}, nil
	}

	result := strings.TrimSpace(string(output))
	if result == "" {
		return []string{}, nil
	}

	// 줄바꿈으로 분리된 파일 경로들을 배열로 변환
	filePaths := strings.Split(result, "\n")
	var validPaths []string

	for _, path := range filePaths {
		path = strings.TrimSpace(path)
		if path != "" {
			validPaths = append(validPaths, path)
		}
	}

	return validPaths, nil
}

// DragToCopy starts the drag to copy mode.
func (g *Goful) DragToCopy() {
	// 드래그 정보를 모드에 전달
	mode := &dragToCopyMode{
		Goful:     g,
		src:       "",
		dragStart: g.dragStart,
		dragEnd: struct {
			x, y int
			dir  *filer.Directory
		}{
			x:   0,
			y:   0,
			dir: nil,
		},
	}

	c := cmdline.New(mode, g)
	if g.Dir().IsMark() {
		// 여러 파일 복사 시에도 목적지 폴더를 바로 표시
		c.SetText(g.Workspace().NextDir().Path)
	} else {
		// 단일 파일 복사 시에는 (반대쪽 pane 경로 + 파일명)으로 기본값 세팅
		dstPath := filepath.Join(g.Workspace().NextDir().Path, g.File().Name())
		c.SetText(dstPath)
	}
	g.next = c
}

type dragToCopyMode struct {
	*Goful
	src       string // 이 필드는 이제 첫 번째 엔터 입력 전까지는 비어있지 않고, src 파일(들) 경로를 저장
	dragStart *struct {
		x, y     int
		dir      *filer.Directory
		file     *filer.FileStat
		dragging bool
	}
	dragEnd struct {
		x, y int
		dir  *filer.Directory
	}
}

func (m *dragToCopyMode) String() string { return "dragtocopy" }

func (m *dragToCopyMode) Prompt() string {
	if m.src == "" { // 아직 src가 설정되지 않은 초기 상태
		if m.Dir().IsMark() {
			// 마크된 파일들이 있다면 마크된 파일 수와 목적지 폴더를 표시
			return fmt.Sprintf("Drag Copy(드래그복사) %d files -> ", m.Dir().MarkCount())
		} else {
			// 단일 파일이라면 복사될 전체 경로(파일명 포함)를 표시
			return fmt.Sprintf("Drag Copy(드래그복사) %s -> ", shortName(m.File().Name(), 10))
		}
	}
	return "Drag Copy(드래그복사) -> "
}

func (m *dragToCopyMode) Draw(c *cmdline.Cmdline) { c.DrawLine() }

func (m *dragToCopyMode) Run(c *cmdline.Cmdline) {
	if m.src == "" { // 첫 번째 엔터 입력 (현재 파일/마크된 파일 -> 목적지 폴더)
		var srcs []string
		if m.dragStart != nil && m.dragStart.dir.IsMark() {
			srcs = m.dragStart.dir.MarkfilePaths() // 마크된 파일 목록
		} else if m.dragStart != nil {
			srcs = []string{m.dragStart.file.Path()} // 단일 파일 목록 (전체 경로)
		} else {
			// 드래그 정보가 없으면 현재 디렉토리에서 가져오기
			if m.Dir().IsMark() {
				srcs = m.Dir().MarkfilePaths()
			} else {
				srcs = []string{m.File().Path()}
			}
		}

		dst := c.String() // 현재 cmdline에 표시된 텍스트(목적지 경로)
		// 입력값이 파일이면 그 상위 폴더, 폴더면 그대로
		dstDir := dst
		if len(srcs) == 1 || (!m.Dir().IsMark() && len(srcs) == 1) {
			dstDir = filepath.Dir(dst)
		}
		if _, err := os.Stat(dstDir); os.IsNotExist(err) {
			err := os.MkdirAll(dstDir, 0755)
			if err != nil {
				message.Error(fmt.Errorf("Create error (생성 오류): %v", err))
				c.Exit()
				return
			}
		}
		m.copy(dst, srcs...)
		m.Workspace().ReloadAll()
		c.Exit()
	} else {
		dst := c.String()
		var srcs []string
		if m.Dir().IsMark() {
			srcs = m.Dir().MarkfilePaths()
		} else {
			srcs = []string{m.src} // m.src가 단일 파일 경로를 가지고 있다고 가정
		}
		dstDir := dst
		if len(srcs) == 1 || (!m.Dir().IsMark() && len(srcs) == 1) {
			dstDir = filepath.Dir(dst)
		}
		if _, err := os.Stat(dstDir); os.IsNotExist(err) {
			err := os.MkdirAll(dstDir, 0755)
			if err != nil {
				message.Error(fmt.Errorf("Create error (생성 오류): %v", err))
				c.Exit()
				return
			}
		}
		m.copy(dst, srcs...)
		m.Workspace().ReloadAll()
		c.Exit()
	}
}

// getClipboardContent는 OS별로 클립보드 내용을 가져오는 통합 함수입니다.
func getClipboardContent() (string, error) {
	if runtime.GOOS == "windows" {
		// Windows에서는 PowerShell 방식만 사용 (안전성 우선)
		filePaths, err := getWindowsClipboardFiles()
		if err != nil {
			// 에러가 발생하면 기존 glippy.Get()을 사용
			value, _ := glippy.Get()
			return value, nil
		}

		if len(filePaths) > 0 {
			// 파일 경로들을 하나의 문자열로 결합
			return strings.Join(filePaths, "\n"), nil
		}

		// 파일 경로가 없으면 일반 텍스트를 가져옵니다
		value, _ := glippy.Get()
		return value, nil
	} else {
		// Windows가 아닌 경우 기존 방식 사용
		value, _ := glippy.Get()
		return value, nil
	}
}

// Windows API 상수들 
