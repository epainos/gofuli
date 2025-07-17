// Package app is goful application components.
package app

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"time"

	"github.com/epainos/gofuli/cmdline"
	"github.com/epainos/gofuli/filer"
	"github.com/epainos/gofuli/info"
	"github.com/epainos/gofuli/menu"
	"github.com/epainos/gofuli/message"
	"github.com/epainos/gofuli/progress"
	"github.com/epainos/gofuli/widget"
	"github.com/gdamore/tcell/v2"
)

// Goful represents a main application.
type Goful struct {
	*filer.Filer
	shell     func(cmd string) []string
	terminal  func(cmd string) []string
	next      widget.Widget
	event     chan tcell.Event
	interrupt chan int
	callback  chan func()
	task      chan int
	exit      bool
	lastClick int64 // for double click detection
	dragStart *struct {
		x, y     int
		dir      *filer.Directory
		file     *filer.FileStat
		dragging bool
	}
	// shiftPressed bool // track shift key state (removed - no longer needed)
}

// NewGoful creates a new goful client based recording a previous state.
func NewGoful(path string) *Goful {
	message.Init()
	info.Init()
	progress.Init()
	width, height := widget.Size()
	goful := &Goful{
		Filer:     filer.NewFromState(path, 0, 0, width, height-2),
		shell:     nil,
		terminal:  nil,
		next:      widget.Nil(),
		event:     make(chan tcell.Event, 1),
		interrupt: make(chan int, 2),
		callback:  make(chan func()),
		task:      make(chan int, 1),
		exit:      false,
	}
	return goful
}

// ConfigShell sets a function that returns a shell name and options.
func (g *Goful) ConfigShell(config func(cmd string) []string) {
	g.shell = config
}

// ConfigTerminal sets a function that returns a terminal name and options.
func (g *Goful) ConfigTerminal(config func(cmd string) []string) {
	g.terminal = config
}

// ConfigFiler sets a keymap function for the filer.
func (g *Goful) ConfigFiler(f func(*Goful) widget.Keymap) {
	g.MergeKeymap(f(g))
}

// Next returns a next widget for drawing and input.
func (g *Goful) Next() widget.Widget { return g.next }

// Disconnect references to a next widget for exiting.
func (g *Goful) Disconnect() { g.next = widget.Nil() }

// Resize all widgets.
func (g *Goful) Resize(x, y, width, height int) {
	offset := 0
	if !progress.IsFinished() {
		offset = 2
	}
	g.Filer.Resize(x, y, width, height-2-offset)
	g.Next().Resize(x, y, width, height-2-offset)
	progress.Resize(0, height-4, width, 1)
	message.Resize(0, height-2, width, 1)
	info.Resize(0, height-1, width, 1)
}

// Draw all widgets.
func (g *Goful) Draw() {
	g.Filer.Draw()
	g.Next().Draw()
	progress.Draw()
	message.Draw()
	info.Draw(g.File())
}

// Input to a current widget.
func (g *Goful) Input(key string) {
	if !widget.IsNil(g.Next()) {
		g.Next().Input(key)
	} else {
		g.Filer.Input(key)
	}
}

// Menu runs a menu mode.
func (g *Goful) Menu(name string) {
	m, err := menu.New(name, g)
	if err != nil {
		message.Error(err)
		return
	}
	g.next = m
}

// Run the goful client.
func (g *Goful) Run() {
	message.Info("Welcome to goful")
	g.Workspace().ReloadAll()

	go func() {
		for {
			g.event <- widget.PollEvent()
		}
	}()

	for !g.exit {
		g.Draw()
		widget.Show()
		select {
		case ev := <-g.event:
			g.eventHandler(ev)
		case <-g.interrupt:
			<-g.interrupt
		case callback := <-g.callback:
			callback()
		}
	}
}

func (g *Goful) syncCallback(callback func()) {
	g.callback <- callback
}

func (g *Goful) eventHandler(ev tcell.Event) {
	switch ev := ev.(type) {
	case *tcell.EventKey:
		key := widget.EventToString(ev)
		g.Input(key)
	case *tcell.EventMouse:
		g.handleMouseEvent(ev)
	case *tcell.EventResize:
		width, height := ev.Size()
		g.Resize(0, 0, width, height)
	}
}

// SetBorderStyle sets the filer border style.
func (g *Goful) SetBorderStyle(style widget.BorderStyle) {
	filer.SetBorderStyle(style)
	for _, ws := range g.Workspaces {
		for _, d := range ws.Dirs {
			d.SetBorderStyle(style)
		}
	}
}

// handleMouseEvent processes mouse events
func (g *Goful) handleMouseEvent(ev *tcell.EventMouse) {
	x, y := ev.Position()
	action := ev.Buttons()

	// Check for shift key state (removed - no longer needed)
	// modifiers := ev.Modifiers()
	// g.shiftPressed = (modifiers & tcell.ModShift) != 0

	// Check for mouse button press (not just button state)
	if action&tcell.Button1 != 0 {
		now := time.Now().UnixNano()

		// Check for double click (within 500ms) and no drag in progress
		if now-g.lastClick < 300000000 && g.dragStart == nil { // 300ms in nanoseconds
			// Double click - open file
			g.handleDoubleClick(x, y)
		} else if g.dragStart == nil {
			// Single click - start drag or move cursor
			g.handleMouseClick(x, y)
		} else {
			// We're already dragging, update drag position
			if g.dragStart != nil {
				// Check if we've moved enough to consider this a drag
				if abs(x-g.dragStart.x) > 5 || abs(y-g.dragStart.y) > 5 {
					// Mark as dragging (no message display)
					if !g.dragStart.dragging {
						g.dragStart.dragging = true
						// message.Info(fmt.Sprintf("드래그 시작: %s", g.dragStart.file.Name()))
					}
				}
			}
		}

		g.lastClick = now
	} else if g.dragStart != nil {
		// Mouse button released - end drag
		g.handleDragEnd(x, y)
	}

	// Mouse wheel scroll
	if action&tcell.WheelUp != 0 {
		g.handleMouseScroll(x, y, -1) // Scroll up
	}
	if action&tcell.WheelDown != 0 {
		g.handleMouseScroll(x, y, 1) // Scroll down
	}
}

// handleDoubleClick handles double click to open file
func (g *Goful) handleDoubleClick(x, y int) {
	ws := g.Workspace()
	if ws == nil {
		return
	}

	for i, dir := range ws.Dirs {
		dirX, dirY := dir.LeftTop()
		dirWidth := dir.Width()
		dirHeight := dir.Height()

		if x >= dirX && x < dirX+dirWidth &&
			y >= dirY && y < dirY+dirHeight {
			// Set focus to the clicked directory
			ws.SetFocus(i)

			// Move cursor to clicked position first
			relX := x - dirX
			relY := y - dirY
			fileIndex := g.convertClickToFileIndex(dir, relX, relY)

			if fileIndex >= 0 && fileIndex < dir.Len() {
				dir.SetCursor(fileIndex)

				// Get the clicked file
				clickedFile := dir.File()
				if clickedFile != nil {
					// Check if it's a directory
					if clickedFile.IsDir() {
						// Special handling for ".." directory
						if clickedFile.Name() == ".." {
							// Navigate to parent directory
							dir.Chdir("..")
						} else {
							// Navigate into directory
							dir.EnterDir()
						}
					} else {
						// Open the file (same as pressing Enter)
						// Use the configured opener command based on OS
						if runtime.GOOS == "windows" {
							g.Spawn("Invoke-Item -LiteralPath  %F %&") // Windows: use Invoke-Item with full path
						} else {
							g.Spawn("open %f") // Other OS: use the default opener
						}
					}
				}

				g.Workspace().ReloadAll()
			}
			break
		}
	}
}

// handleMouseClick handles mouse click at specific coordinates
func (g *Goful) handleMouseClick(x, y int) {
	// 입력모드라면 패인 영역 클릭 시 취소, 위젯 영역 클릭 시 처리
	if !widget.IsNil(g.Next()) {
		nextWidget := g.Next()

		// 입력 모드에서 제목줄(y==0) 클릭 시 메시지만 출력
		if y == 0 {
			// message.Info("입력 모드에서 제목줄 클릭됨")
			return
		}

		// cmdline인 경우 하위 위젯들(히스토리, 자동완성)의 영역도 체크
		if cmdline, ok := nextWidget.(*cmdline.Cmdline); ok {
			//cmdline 자체 영역 체크
			cmdlineX, cmdlineY := cmdline.LeftTop()
			cmdlineWidth := cmdline.Width()
			cmdlineHeight := cmdline.Height()

			// cmdline 영역 안에서 클릭
			if x >= cmdlineX && x < cmdlineX+cmdlineWidth &&
				y >= cmdlineY && y < cmdlineY+cmdlineHeight {
				// message.Info("cmdline 영역에서 클릭됨2")
				return

			}

			// 자동완성 위젯 체크 (cmdline의 하위 위젯)
			if !widget.IsNil(cmdline.Next()) {
				// 자동완성 위젯이 있는 경우 클릭 처리
				completion := cmdline.Next()
				if completion != nil {
					// message.Info("자동완성 선택됨2")
					// HandleMouseClick 메서드가 있는지 확인하고 호출
					if handler, ok := completion.(interface{ HandleMouseClick(int, int) }); ok {
						handler.HandleMouseClick(x, y)
						return
					}
				}
			}

			// 히스토리 영역 체크
			if cmdline.History != nil {
				historyX, historyY := cmdline.History.LeftTop()
				historyWidth := cmdline.History.Width()
				historyHeight := cmdline.History.Height()

				if x >= historyX && x < historyX+historyWidth &&
					y >= historyY && y < historyY+historyHeight {
					// 자동완성과 동일한 방식으로 히스토리 클릭 처리
					clickedIndex := g.convertClickToListBoxIndex(historyX, historyY, x, y, cmdline.History) + 1

					if clickedIndex >= 0 && clickedIndex < cmdline.History.Upper() {
						// 클릭한 행으로 커서 이동
						cmdline.History.SetCursor(clickedIndex)
						// 해당 히스토리 항목을 cmdline에 설정
						if cmdline.History.Cursor() != cmdline.History.Lower() {
							cmdline.SetText(cmdline.History.CurrentContent().Name())
						}
					}
					return
				}
			}

			// 모든 영역 밖에서 클릭 - cmdline 취소
			// message.Info("cmdline 영역 밖에서 클릭됨 - 취소")
			cmdline.Exit()
			g.next = widget.Nil()
			return

		}

		// 메뉴 위젯인 경우 handleMenuClick으로 처리
		if _, ok := nextWidget.(*menu.Menu); ok {
			g.handleMenuClick(x, y)
			return
		}

		// 자동완성 위젯인 경우 HandleMouseClick으로 처리
		if completion, ok := nextWidget.(*cmdline.Completion); ok {
			completion.HandleMouseClick(x, y)
			return
		}

	}

	//입력모드 끝남
	// 1. 탭 헤더 클릭 감지 (y==0)
	if y == 0 {
		xpos := 0
		for i, ws := range g.Workspaces {
			tabStr := fmt.Sprintf(" %s ", ws.Title)
			tabLen := len([]rune(tabStr))
			if x >= xpos && x < xpos+tabLen {
				// 탭 클릭됨!
				if g.Current != i {
					g.Workspace().Visible(false)
					g.Current = i
					g.Workspace().Visible(true)
					g.Workspace().ReloadAll()
					message.Info(fmt.Sprintf("탭 변경: %s", ws.Title))
				}
				return // 탭 클릭시 아래 코드 실행 안함
			}
			xpos += tabLen
		}

		// 탭이 아닌 곳 클릭: 경로 바 영역 클릭인지 확인
		ws := g.Workspace()
		width := (g.Width() - xpos) / len(ws.Dirs)
		for i := range ws.Dirs {
			label := fmt.Sprintf("[%d] ", i+1)
			labelLen := len([]rune(label))
			if x >= xpos && x < xpos+width-labelLen {
				// 경로 바 클릭됨: HeaderPathEdit 모드 진입
				ws.SetFocus(i)     // 해당 디렉토리에 포커스 설정
				g.HeaderPathEdit() // 자동완성/히스토리 기능이 있는 편집 모드
				return
			}
			xpos += width
		}
	}

	// Get the current workspace and directory
	ws := g.Workspace()
	if ws == nil {
		return
	}

	// Find which directory is clicked
	for i, dir := range ws.Dirs {
		dirX, dirY := dir.LeftTop()
		dirWidth := dir.Width()
		dirHeight := dir.Height()

		if x >= dirX && x < dirX+dirWidth &&
			y >= dirY && y < dirY+dirHeight {
			// Set focus to the clicked directory
			ws.SetFocus(i)

			// Calculate relative position within the directory
			relX := x - dirX
			relY := y - dirY

			// Convert screen coordinates to file list index
			fileIndex := g.convertClickToFileIndex(dir, relX, relY)

			if fileIndex >= 0 && fileIndex < dir.Len() {
				// Set cursor to the clicked file
				dir.SetCursor(fileIndex)

				// Start drag operation (but don't show message yet)
				g.dragStart = &struct {
					x, y     int
					dir      *filer.Directory
					file     *filer.FileStat
					dragging bool
				}{
					x:        x,
					y:        y,
					dir:      dir,
					file:     dir.File(),
					dragging: false,
				}

				g.Workspace().ReloadAll()
			}
			break
		}

	}
}

// handleMenuClick handles mouse clicks in menu mode
func (g *Goful) handleMenuClick(x, y int) {
	menu := g.Next()
	if widget.IsNil(menu) {
		return
	}

	// Get menu position and size
	menuX, menuY := menu.LeftTop()
	menuWidth := menu.Width()
	menuHeight := menu.Height()

	// Check if click is within menu bounds
	if x >= menuX && x < menuX+menuWidth &&
		y >= menuY && y < menuY+menuHeight {

		// Calculate which menu item was clicked with offset consideration
		// 메뉴를 ListBox로 캐스팅하여 offset을 고려한 인덱스 계산
		if menuListBox, ok := menu.(interface {
			SetCursor(int)
			Offset() int
		}); ok {
			// 메뉴는 offset을 고려하지 않고 직접 클릭한 행을 사용
			clickedIndex := g.convertClickToMenuIndex(menu, x, y) + 1
			if clickedIndex >= 0 {
				menuListBox.SetCursor(clickedIndex)
				// Execute the menu item
				g.Input("C-m") // Enter key to execute current item
			}
		}
	}
}

// convertClickToMenuIndex converts mouse click coordinates to menu index
func (g *Goful) convertClickToMenuIndex(menu interface{ LeftTop() (int, int) }, x, y int) int {
	_, menuY := menu.LeftTop()

	// 상대 좌표 계산
	relY := y - menuY

	// 헤더와 보더 고려
	borderOffset := 1
	clickedRow := relY - borderOffset - 1 // -1 for header

	// 유효한 범위 체크
	if clickedRow < 0 {
		return -1
	}

	// 메뉴도 ListBox를 상속받으므로 offset을 고려
	if menuWithOffset, ok := menu.(interface{ Offset() int }); ok {
		actualIndex := menuWithOffset.Offset() + clickedRow
		if actualIndex >= 0 {
			return actualIndex
		}
	}

	// offset이 없는 경우 단순히 clickedRow 반환
	return clickedRow
}

// handleListBoxClick handles mouse clicks in list box widgets (history, completion, etc.)
func (g *Goful) handleListBoxClick(x, y int, listBox interface{ SetCursor(int) }, contentGetter func() interface{ Name() string }, textSetter func(string)) {
	if listBox == nil {
		return
	}

	// Get list box position and size
	listBoxX, listBoxY := listBox.(interface{ LeftTop() (int, int) }).LeftTop()
	listBoxWidth := listBox.(interface{ Width() int }).Width()
	listBoxHeight := listBox.(interface{ Height() int }).Height()

	// Check if click is within list box bounds
	if x >= listBoxX && x < listBoxX+listBoxWidth &&
		y >= listBoxY && y < listBoxY+listBoxHeight {

		// Calculate which item was clicked with offset consideration
		clickedIndex := g.convertClickToListBoxIndex(listBoxX, listBoxY, x, y, listBox)
		if clickedIndex >= 0 {
			listBox.SetCursor(clickedIndex)
			if contentGetter != nil && textSetter != nil {
				content := contentGetter()
				if content != nil {
					textSetter(content.Name())
				}
			}
		}
	}
}

// convertClickToListBoxIndex converts mouse click coordinates to list box index
func (g *Goful) convertClickToListBoxIndex(listBoxX, listBoxY, x, y int, listBox interface{ SetCursor(int) }) int {
	// 상대 좌표 계산
	relY := y - listBoxY

	// 헤더와 보더 고려
	borderOffset := 1
	clickedRow := relY - borderOffset - 1 // -1 for header

	// 유효한 범위 체크
	if clickedRow < 0 {
		return -1
	}

	// ListBox는 단일 열이므로 offset + row로 계산
	if listBoxWithOffset, ok := listBox.(interface{ Offset() int }); ok {
		actualIndex := listBoxWithOffset.Offset() + clickedRow
		if actualIndex >= 0 {
			return actualIndex
		}
	}

	// offset이 없는 경우 단순히 clickedRow 반환
	return clickedRow
}

// handleDragEnd handles the end of a drag operation
func (g *Goful) handleDragEnd(x, y int) {
	if g.dragStart == nil {
		return
	}

	// Check if we dragged to a different location (lower threshold)
	if abs(x-g.dragStart.x) > 5 || abs(y-g.dragStart.y) > 5 {
		// This was a drag operation, not just a click
		// message.Info(fmt.Sprintf("드래그 감지됨: 거리=%d", dragDistance))
		g.handleFileDrag(g.dragStart.x, g.dragStart.y, x, y)
	} else {
		// This was just a click, not a drag
		// message.Info(fmt.Sprintf("클릭 감지됨: 거리=%d", dragDistance))
		// Clear drag state without doing anything
	}

	// Clear drag state
	g.dragStart = nil
}

// handleFileDrag handles dragging files from one location to another
func (g *Goful) handleFileDrag(startX, startY, endX, endY int) {
	ws := g.Workspace()
	if ws == nil || g.dragStart == nil {
		return
	}

	// Find target directory and check if we're dropping on a folder
	targetDir := g.findTargetDirectory(endX, endY)
	if targetDir == nil {
		// message.Info("드래그 대상이 없음")
		return
	}

	// 드래그 종료 시 DragToCopy 모드 호출
	g.DragToCopy()

	// 드래그 종료 위치 정보는 DragToCopy 함수에서 직접 전달됨
}

// abs returns the absolute value of an integer
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// copyFile copies a file from source to destination
func (g *Goful) copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}

// findTargetDirectory finds which directory pane the coordinates are in
func (g *Goful) findTargetDirectory(x, y int) *filer.Directory {
	ws := g.Workspace()
	if ws == nil {
		return nil
	}

	for _, dir := range ws.Dirs {
		dirX, dirY := dir.LeftTop()
		dirWidth := dir.Width()
		dirHeight := dir.Height()

		if x >= dirX && x < dirX+dirWidth &&
			y >= dirY && y < dirY+dirHeight {
			return dir
		}
	}
	return nil

}

// findTargetFolder finds if the coordinates are on a folder within a directory
func (g *Goful) findTargetFolder(x, y int) *filer.FileStat {
	ws := g.Workspace()
	if ws == nil {
		return nil
	}

	for _, dir := range ws.Dirs {
		dirX, dirY := dir.LeftTop()
		dirWidth := dir.Width()
		dirHeight := dir.Height()

		if x >= dirX && x < dirX+dirWidth &&
			y >= dirY && y < dirY+dirHeight {
			// Calculate relative position within the directory
			relX := x - dirX
			relY := y - dirY

			// Convert screen coordinates to file list index
			fileIndex := g.convertClickToFileIndex(dir, relX, relY)

			if fileIndex >= 0 && fileIndex < dir.Len() {
				// Set cursor to this position temporarily to get the file
				originalCursor := dir.Cursor()
				dir.SetCursor(fileIndex)
				file := dir.File()
				dir.SetCursor(originalCursor) // Restore original cursor

				if file != nil && file.IsDir() {
					return file
				}
			}
			// 파일이 없거나 클릭한 위치에 파일이 없으면 현재 폴더 위치를 반환 (빈 공간)
			// 현재 디렉토리를 나타내는 FileStat 객체 생성
			currentDir := filer.NewFileStat(dir.Path, ".")
			if currentDir != nil {
				return currentDir
			}
			break
		}
	}
	return nil
}

// convertClickToFileIndex converts mouse click coordinates to file list index
func (g *Goful) convertClickToFileIndex(dir *filer.Directory, x, y int) int {
	// Account for border and header
	borderOffset := 1
	if dir.BorderStyle() != widget.NoBorder {
		borderOffset = 1
	}

	// Add small offset to improve click accuracy for cell areas
	y += 1

	// Calculate which file row was clicked (subtract header)
	clickedRow := y - borderOffset - 1 // -1 for header

	// Get the current scroll position (offset)
	scrollOffset := dir.Offset()

	// Calculate the actual file index
	fileIndex := scrollOffset + clickedRow

	// Ensure the index is within bounds
	if fileIndex < 0 {
		fileIndex = 0
	}
	if fileIndex >= dir.Len() {
		fileIndex = dir.Len() - 1
	}

	return fileIndex
}

// handleMouseScroll handles mouse wheel scrolling
func (g *Goful) handleMouseScroll(x, y, direction int) {
	// 입력모드에서 마우스 휠 처리
	if !widget.IsNil(g.Next()) {
		nextWidget := g.Next()

		// cmdline의 히스토리 영역에서 마우스 휠 처리
		if cmdline, ok := nextWidget.(*cmdline.Cmdline); ok {
			// 자동완성 위젯이 열려있는지 먼저 확인
			if !widget.IsNil(cmdline.Next()) {
				completion := cmdline.Next()
				if completion != nil {
					compX, compY := completion.LeftTop()
					compWidth := completion.Width()
					compHeight := completion.Height()

					// 자동완성 창 위에서 마우스 휠 사용
					if x >= compX && x < compX+compWidth &&
						y >= compY && y < compY+compHeight {
						message.Info(fmt.Sprintf("자동완성 창에서 휠 처리: direction=%d", direction))
						if listBox, ok := completion.(interface {
							CursorDown()
							CursorUp()
							CursorToRight()
							CursorToLeft()
							Column() int
						}); ok {
							// 열 개수에 따라 휠 동작 결정
							if listBox.Column() == 1 {
								// 1열일 때는 위아래로 움직임
								if direction > 0 {
									listBox.CursorDown()
								} else {
									listBox.CursorUp()
								}
							} else {
								// 2열 이상일 때는 좌우로 움직임
								if direction > 0 {
									listBox.CursorToRight()
								} else {
									listBox.CursorToLeft()
								}
							}
						}
						return // 자동완성 영역에서 휠 처리 후 종료
					}
				}
			}

			// 히스토리 영역에서 마우스 휠 처리
			if cmdline.History != nil {
				historyX, historyY := cmdline.History.LeftTop()
				historyWidth := cmdline.History.Width()
				historyHeight := cmdline.History.Height()

				if x >= historyX && x < historyX+historyWidth &&
					y >= historyY && y < historyY+historyHeight {
					// 히스토리에서 마우스 휠로 커서 이동
					if direction > 0 {
						cmdline.History.CursorDown() // 휠 다운 = 커서 다운
					} else {
						cmdline.History.CursorUp() // 휠 업 = 커서 업
					}
					return
				}
			}
		}

		// 메뉴 위젯에서 마우스 휠 처리
		if menu, ok := nextWidget.(*menu.Menu); ok {
			menuX, menuY := menu.LeftTop()
			menuWidth := menu.Width()
			menuHeight := menu.Height()

			if x >= menuX && x < menuX+menuWidth &&
				y >= menuY && y < menuY+menuHeight {
				// 메뉴에서 마우스 휠로 커서 이동
				if direction > 0 {
					menu.MoveCursor(1)
				} else {
					menu.MoveCursor(-1)
				}
				return
			}
		}

		// 자동완성 위젯에서 마우스 휠 처리 (직접 위젯인 경우)
		if completion, ok := nextWidget.(*cmdline.Completion); ok {
			compX, compY := completion.LeftTop()
			compWidth := completion.Width()
			compHeight := completion.Height()

			if x >= compX && x < compX+compWidth &&
				y >= compY && y < compY+compHeight {
				// 자동완성에서 마우스 휠로 커서 이동
				if direction > 0 {
					completion.CursorDown()
				} else {
					completion.CursorUp()
				}
				return
			}
		}

	}

	// 일반 모드에서 디렉토리 위젯 마우스 휠 처리
	ws := g.Workspace()
	if ws == nil {
		return
	}

	for _, dir := range ws.Dirs {
		dirX, dirY := dir.LeftTop()
		dirWidth := dir.Width()
		dirHeight := dir.Height()

		if x >= dirX && x < dirX+dirWidth &&
			y >= dirY && y < dirY+dirHeight {
			// Move cursor up or down instead of scrolling
			if direction > 0 {
				dir.MoveCursor(1) // Move cursor down
			} else {
				dir.MoveCursor(-1) // Move cursor up
			}
			g.Workspace().ReloadAll()
			break
		}
	}
}
