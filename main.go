package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/epainos/gofuli/app"
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

func main() {

	is_tmux := false
	widget.Init()
	defer widget.Fini()

	if runtime.GOOS == "darwin" {
		is_tmux = strings.Contains(os.Getenv("TERM_PROGRAM"), "tmux")
	} else {
		is_tmux = strings.Contains(os.Getenv("TERM"), "screen")
	}
	// Change a terminal title.
	if is_tmux {
		os.Stdout.WriteString("\033kgoful\033") // for tmux
	} else {
		os.Stdout.WriteString("\033]0;goful\007") // for otherwise
	}

	const state = "~/.goful/state.json"
	const history = "~/.goful/history/shell"

	goful := app.NewGoful(state)
	config(goful, is_tmux)
	_ = cmdline.LoadHistory(history)
	goful.OpenMyAppList("~/.goful/myApp")           // myfOpenMyAppList 메서드 호출
	goful.OpenMyBookmarkList("~/.goful/myBookmark") // myfOpenMyBookmarkList 메서드 호출

	goful.Run()

	_ = goful.SaveState(state)
	_ = cmdline.SaveHistory(history)
}

func config(g *app.Goful, is_tmux bool) {

	look.Set("default") // default, midnight, black, white

	if runewidth.EastAsianWidth {
		// Because layout collapsing for ambiguous runes if LANG=ja_JP.
		widget.SetBorder('│', '_', '┌', '┐', '└', '┘') // 0x2502, 0x2500, 0x250c, 0x2510, 0x2514, 0x2518
		// widget.SetBorder('|', '-', '+', '+', '+', '+')
	} else {
		// Look good if environment variable RUNEWIDTH_EASTASIAN=0 and
		// ambiguous char setting is half-width for gnome-terminal.
		widget.SetBorder('│', '─', '┌', '┐', '└', '┘') // 0x2502, 0x2500, 0x250c, 0x2510, 0x2514, 0x2518
	}
	g.SetBorderStyle(widget.ULBorder) // AllBorder, ULBorder, NoBorder

	message.SetInfoLog("~/.goful/log/info.log")   // "" is not logging
	message.SetErrorLog("~/.goful/log/error.log") // "" is not logging
	message.Sec(3)                                // display second for a message

	// Setup widget keymaps.
	g.ConfigFiler(filerKeymap)
	filer.ConfigFinder(finderKeymap)
	cmdline.Config(cmdlineKeymap)
	cmdline.ConfigCompletion(completionKeymap)
	menu.Config(menuKeymap)

	filer.SetStatView(true, false, false) // size, permission and time
	filer.SetTimeFormat("060102_15:04")   // ex: "Jan _2 15:04"

	// Setup open command for C-m (when the enter key is pressed)
	// The macro %f means expanded to a file name, for more see (spawn.go)
	opener := "xdg-open %m %&"
	switch runtime.GOOS {
	case "windows":
		opener = "Invoke-Item  %~c %&" // c meaans comma separated file name
		// opener = "explorer '%~f' %&" //windows //open olny one file
	case "darwin":
		opener = "open %m %&"
	}
	g.MergeKeymap(widget.Keymap{
		"C-m":   func() { g.Spawn(opener) }, // C-m means Enter key
		"o":     func() { g.Spawn(opener) },
		"l":     func() { g.Spawn(opener) },
		"right": func() { g.Spawn(opener) },
	})

	// // Setup pager by $PAGER
	// pager := os.Getenv("PAGER")
	// if pager == "" {
	// 	if runtime.GOOS == "windows" {
	// 		pager = "more"
	// 	} else {
	// 		pager = "less"
	// 	}
	// }
	// if runtime.GOOS == "windows" {
	// 	pager += " %~f"
	// } else {
	// 	pager += " %f"
	// }
	// g.AddKeymap("0", func() { g.Spawn(pager) })

	// Setup a shell and a terminal to execute external commands.
	// The shell is called when execute on background by the macro %&.
	// The terminal is called when the other.
	if runtime.GOOS == "windows" {
		g.ConfigShell(func(cmd string) []string {
			return []string{"powershell", ``, cmd, ``}
			// return []string{"cmd", "/c /u", strings.Replace(cmd, `\"`, `"`, -1)}
		})
		g.ConfigTerminal(func(cmd string) []string {
			return []string{"powershell", ``, cmd, ``}
			// return []string{"cmd", "/c /u", "start", "cmd", "/c /u ", strings.Replace(cmd, `\"`, `"`, -1) + "& pause"}
			// return []string{"cmd", "/c /u ", strings.Replace(cmd, `\"`, `"`, -1) + "& pause"}
		})
	} else if runtime.GOOS == "darwin" {
		g.ConfigShell(func(cmd string) []string {
			return []string{"zsh", "-c", cmd}
		})
		g.ConfigTerminal(func(cmd string) []string {
			// for not close the terminal when the shell finishes running
			const tail = "" //`;read -p "HIT ENTER KEY"`

			// To execute bash in gnome-terminal of a new window or tab.
			title := "" //"echo -n '\033]0;" + cmd + "\007';" // for change title
			return []string{"zsh", "-c", title + cmd + tail}
		})

	} else { //linux
		g.ConfigShell(func(cmd string) []string {
			return []string{"bash", "-c", cmd}
		})
		g.ConfigTerminal(func(cmd string) []string {
			// for not close the terminal when the shell finishes running
			const tail = "" //`;read -p "HIT ENTER KEY"`

			if is_tmux { // such as screen and tmux
				return []string{"tmux", "new-window", "-n", cmd, cmd + tail}
			}
			// To execute bash in gnome-terminal of a new window or tab.
			title := "echo -n '\033]0;" + cmd + "\007';" // for change title
			return []string{"gnome-terminal", "--", "bash", "-c", title + cmd + tail}
		})
	}

	// Setup menus and add to keymap.
	menu.Add("sort",
		"n", "sort name           이름순         ", func() { g.Dir().SortName() },
		"N", "sort name decendin  이름 역순", func() { g.Dir().SortNameDec() },
		"s", "sort size           용량별         ", func() { g.Dir().SortSize() },
		"S", "sort size decending 용량 역순", func() { g.Dir().SortSizeDec() },
		"t", "sort time           시간별         ", func() { g.Dir().SortMtime() },
		"T", "sort time decending 시간 역순", func() { g.Dir().SortMtimeDec() },
		"e", "sort ext            확장자별          ", func() { g.Dir().SortExt() },
		"E", "sort ext decending  확장자역수", func() { g.Dir().SortExtDec() },
		".", "toggle priority     폴더를 따로 정렬   ", func() { filer.TogglePriority(); g.Workspace().ReloadAll() },
	)
	g.AddKeymap("s", func() { g.Menu("sort") })

	menu.Add("view",
		"s", "stat menu                 상태메뉴   ", func() { g.Menu("stat") },
		"l", "layout menu               레이아웃 ", func() { g.Menu("layout") },
		"L", "look menu                 보기메뉴   ", func() { g.Menu("look") },
		"t", "tab menu                  탭메뉴     ", func() { g.Menu("tab") },
		".", "toggle show hidden files  숨김파일 켬/끔", func() { filer.ToggleShowHiddens(); g.Workspace().ReloadAll() },
	)
	g.AddKeymap("v", func() { g.Menu("view") })

	menu.Add("layout",
		"t", "tile         왼쪽에 하나", func() { g.Workspace().LayoutTile() },
		"T", "tile-top     위쪽에 하나", func() { g.Workspace().LayoutTileTop() },
		"b", "tile-bottom  아래쪽에 하나", func() { g.Workspace().LayoutTileBottom() },
		"r", "one-row      행 정렬", func() { g.Workspace().LayoutOnerow() },
		"c", "one-column   열 정렬", func() { g.Workspace().LayoutOnecolumn() },
		"f", "fullscreen   전체화면", func() { g.Workspace().LayoutFullscreen() },
	)

	menu.Add("stat",
		"s", "Size           용량 켬/끔  ", func() { filer.ToggleSizeView() },
		"p", "Permision      권한 켬/끔  ", func() { filer.TogglePermView() },
		"t", "Time           날짜 켬/끔  ", func() { filer.ToggleTimeView() },
		"e", "Essential      용량만 보임     ", func() { filer.SetStatView(true, false, false) },
		"a", "size+per+time  용량+권한+날짜     ", func() { filer.SetStatView(true, true, true) },
		"n", "noting         없음      ", func() { filer.SetStatView(false, false, false) },
	)

	menu.Add("look",
		"d", "default      ", func() { look.Set("default") },
		"n", "midnight     ", func() { look.Set("midnight") },
		"b", "black        ", func() { look.Set("black") },
		"o", "original        ", func() { look.Set("original") },
		"w", "white        ", func() { look.Set("white") },
		"a", "all border   ", func() { g.SetBorderStyle(widget.AllBorder) },
		"u", "ul border    ", func() { g.SetBorderStyle(widget.ULBorder) },
		"0", "no border    ", func() { g.SetBorderStyle(widget.NoBorder) },
	)

	menu.Add("command",
		"c", "(c) copy            복사        ", func() { g.Copy() },
		"m", "(m) move            이동        ", func() { g.Move() },
		"d", "(delete)            삭제      ", func() { g.Remove() },
		"k", "(K) make dir        폴더생성       ", func() { g.Mkdir() },
		"n", "(n) newfile         파일생성     ", func() { g.Touch() },
		"M", "(M) chmod           권한수정       ", func() { g.Chmod() },
		"r", "(r) rename          이름변경      ", func() { g.Rename() },
		"R", "(R) bulk rename     이름 일괄 변경 ", func() { g.BulkRename() },
		"D", "(D) chdir           경로 이동       ", func() { g.Chdir() },
		"g", "(g) glob            찾기 ", func() { g.Glob() },
		"G", "(G) globdir         찾기(하부폴더)", func() { g.Globdir() },
		"b", "(B) go pre dir      폴더 뒤로 가기", func() { g.Dir().GoPreviousFolder() },
		"f", "(F) go forward dir  폴더 앞으로 가기", func() { g.Dir().GoFowardFolder() },
	)
	g.AddKeymap("x", func() { g.Menu("command") })
	menu.Add("external-command",
		"r", "(f2) (rename) %f      이름변경", func() { g.Rename2() },
		"R", "(R)  bulk rename      이름 일괄 변경 ", func() { g.BulkRename() },
		"c", "(f5) copy %m to %D2   복사", func() { g.Copy2() },
		"m", "(f6) move %m to %D2   이동", func() { g.Move2() },
		"k", "(f7) make directory   새폴더", func() { g.Mkdir2() },
		"d", "(f8) del /s %M        삭제", func() { g.Remove2() },
		// "D", "rd /s /q %~m     폴더 삭제", func() { g.Shell("rd /s /q %~m") },
		"n", "(n)  create newfile   새파일", func() { g.Shell("copy nul ") },
		"M", "(m)  change mode %m   권한변경", ifElse(runtime.GOOS == "windows", func() { message.Info("Windows doesn't need to CHMOD (윈도우는 권한설정 불필요)") }, func() { g.Chmod() }), //change file permission

		"Z", "(Z) zip to here       압축파일관련 메뉴", func() { g.ZipToHere() }, //add file to archive
		"z", "(z) zip to other      압축파일관련 메뉴", func() { g.ZipToOtherPane() }, //add file to archive
		"A", "(A) unzip to here     압축파일관련 메뉴", func() { g.UnZipToHere() }, //unzip file to here
		"a", "(a) unzip to other    압축파일관련 메뉴", func() { g.UnzipToOtherPane() }, //unzip file to other

		"y", "(y) copy to clipboard 클립보드에 파일 복사", func() {
			if runtime.GOOS == "windows" {
				myClip := strings.Join(g.Dir().MarkfileDoubleQuotedPaths(), " ")
				glippy.Set(myClip)
				message.Info("Files copied to clipboard (클립보드 복사): " + myClip)
				command := `Add-Type -AssemblyName System.Windows.Forms; $filePaths = [regex]::Matches((Get-Clipboard), '\"(.*?)\"') | ForEach-Object { $_.Groups[1].Value }; if ($filePaths.Count -gt 0) { $stringCollection = New-Object System.Collections.Specialized.StringCollection; $stringCollection.AddRange($filePaths); [System.Windows.Forms.Clipboard]::SetFileDropList($stringCollection) }`
				exec.Command("powershell", "-Command", command).Run()
			} else if runtime.GOOS == "darwin" {
				filePaths := g.Dir().MarkfilePaths()
				if err := copyFilesToMacClipboard(filePaths); err != nil {
					message.Info("클립보드 복사 실패: " + err.Error())
				} else {
					message.Info(fmt.Sprintf("✅ %d개 파일이 클립보드에 복사되었습니다! (Cmd+V로 붙여넣기)", len(filePaths)))
				}
			} else {
				myClip := strings.Join(g.Dir().MarkfileDoubleQuotedPaths(), " ")
				glippy.Set(myClip)
				message.Info("Files copied to clipboard (클립보드 복사): " + myClip)
			}
		},

		"p", "(p) paste from clipboard 클립보드에서 파일 붙여넣기", func() { g.PasteCopy() },

		"P", "(P) move from clipboard 클립보드에서 파일 이동", func() { g.PasteMove() },
	)

	g.AddKeymap("X", func() { g.Menu("external-command") })

	menu.Add("tab",
		"t", "(C-t)   New tab          새탭        ", func() { g.CreateWorkspace(); g.MoveWorkspace(1) },
		"T", "(M-t)   close tab        탭 닫기     ", func() { g.CloseWorkspace() },
		"n", "        changeTitle      탭이름 변경    ", func() { g.ChangeWorkspaceTitle() },
		"f", "(t)     Forward tab      앞탭으로      ", func() { g.MoveWorkspace(1) },
		"b", "(M-b)   Backward tab     뒷탭으로       ", func() { g.MoveWorkspace(-1) },
		"F", "(tab)   Forward window   앞창으로 이동     ", func() { g.Workspace().ReloadAll(); g.Workspace().MoveFocus(1) },
		"B", "(C-b)   Backward window  뒷창으로 이동      ", func() { g.Workspace().ReloadAll(); g.Workspace().MoveFocus(-1) },
		"s", "(Q)     Swap next dir    앞창으로 바꿈       ", func() { g.Workspace().SwapNextDir() },
		"S", "        Swap prev dir    뒷창으로 바꿈       ", func() { g.Workspace().SwapPrevDir() },
		"r", "(')     Reload all       모두 다시 읽음       ", func() { g.Workspace().ReloadAll() },
		"w", "(C-w)   Open new window  창 추가       ", func() { g.Workspace().CreateDir() },
		"W", "(M-w)   clOse window     창 닫기       ", func() { g.Workspace().CloseDir() },
	)
	g.AddKeymap("T", func() { g.Menu("tab") })

	// menu.Add("archive",
	// 	"z", "zip     ", func() { g.Shell(`zip -roD %x.zip %m`, -7) },
	// 	"t", "tar     ", func() { g.Shell(`tar cvf %x.tar %m`, -7) },
	// 	"g", "tar.gz  ", func() { g.Shell(`tar cvfz %x.tgz %m`, -7) },
	// 	"b", "tar.bz2 ", func() { g.Shell(`tar cvfj %x.bz2 %m`, -7) },
	// 	"x", "tar.xz  ", func() { g.Shell(`tar cvfJ %x.txz %m`, -7) },
	// 	"r", "rar     ", func() { g.Shell(`rar u %x.rar %m`, -7) },

	// 	"Z", "extract zip for %m", func() { g.Shell(`for i in %m; do unzip "$i" -d ./; done`, -6) },
	// 	"T", "extract tar for %m", func() { g.Shell(`for i in %m; do tar xvf "$i" -C ./; done`, -6) },
	// 	"G", "extract tgz for %m", func() { g.Shell(`for i in %m; do tar xvfz "$i" -C ./; done`, -6) },
	// 	"B", "extract bz2 for %m", func() { g.Shell(`for i in %m; do tar xvfj "$i" -C ./; done`, -6) },
	// 	"X", "extract txz for %m", func() { g.Shell(`for i in %m; do tar xvfJ "$i" -C ./; done`, -6) },
	// 	"R", "extract rar for %m", func() { g.Shell(`for i in %m; do unrar x "$i" -C ./; done`, -6) },

	// 	"1", "find . *.zip extract", func() { g.Shell(`find . -name "*.zip" -type f -prune -print0 | xargs -n1 -0 unzip -d ./`) },
	// 	"2", "find . *.tar extract", func() { g.Shell(`find . -name "*.tar" -type f -prune -print0 | xargs -n1 -0 tar xvf -C ./`) },
	// 	"3", "find . *.tgz extract", func() { g.Shell(`find . -name "*.tgz" -type f -prune -print0 | xargs -n1 -0 tar xvfz -C ./`) },
	// 	"4", "find . *.bz2 extract", func() { g.Shell(`find . -name "*.bz2" -type f -prune -print0 | xargs -n1 -0 tar xvfj -C ./`) },
	// 	"5", "find . *.txz extract", func() { g.Shell(`find . -name "*.txz" -type f -prune -print0 | xargs -n1 -0 tar xvfJ -C ./`) },
	// 	"6", "find . *.rar extract", func() { g.Shell(`find . -name "*.rar" -type f -prune -print0 | xargs -n1 -0 unrar x -C ./`) },
	// )

	menu.Add("bookmark",
		"b", "~/            홈", func() { g.Dir().Chdir("~") },
		"k", "~/Desktop     바탕화면 ", func() { g.Dir().Chdir("~/Desktop") },
		"d", "~/Documents   내문서", func() { g.Dir().Chdir("~/Documents") },
		"l", "~/Downloads   다운로드", func() { g.Dir().Chdir("~/Downloads") },
	)
	if runtime.GOOS == "windows" {
		menu.Add("bookmark",
			"A", "A:/", func() { g.Dir().Chdir("A:/") },
			"C", "C:/", func() { g.Dir().Chdir("C:/") },
			"D", "D:/", func() { g.Dir().Chdir("D:/") },
			"E", "E:/", func() { g.Dir().Chdir("E:/") },
			"X", "X:/", func() { g.Dir().Chdir("X:/") },
		)
	} else if runtime.GOOS == "darwin" {
		menu.Add("bookmark",
			"a", "/Applications 응용프로그램", func() { g.Dir().Chdir("/Applications") },
		)
	} else {
		menu.Add("bookmark",
			"e", "/etc   ", func() { g.Dir().Chdir("/etc") },
			"u", "/usr   ", func() { g.Dir().Chdir("/usr") },
			"x", "/media ", func() { g.Dir().Chdir("/media") },
		)
	}
	g.AddKeymap("b", func() { g.Menu("bookmark") })

	menu.Add("myBookmark",
		"+", "add myBookmark   바로가기 추가 ", func() { g.AddMyBookmark() },
		"-", "del myBookmark   바로가기 제거 (DELETE key delete bookmark on cursor)", func() { g.DelMyBookmark() },
	)
	g.AddKeymap("B", func() { g.Menu("myBookmark") })

	menu.Add("editor",
		"e", "vscodE       코드 ", func() { g.Spawn("code %f %&") },
		"E", "Emacs client 이맥스 ", func() { g.Spawn("emacsclient -n %f %&") },
		"c", "cursor       커서 ", func() { g.Spawn("cursor %f %&") },
		"x", "eXcel        엑셀", ifElse(runtime.GOOS == "windows", func() { g.Spawn(`start 'C:/Program Files/Microsoft Office/root/Office16/excel.exe' '"%~F"'`) }, func() { g.Spawn(`open -a "Microsoft Excel"  %f %&`) }),
		"C", "Chrome       크롬", ifElse(runtime.GOOS == "windows", func() { g.Spawn(`start 'C:/Program Files/Google/Chrome/Application/chrome' '"%~F"'`) }, func() { g.Spawn(`open -a "Google Chrome" %f %&`) }),
	)
	g.AddKeymap("e", func() { g.Menu("editor") })

	menu.Add("myApp",
		"+", "add MyApp   사용자앱 추가 ", func() { g.AddMyApp() },
		"-", "del MyApp   사용자앱 제거 ( DELETE key also can delete app on cursor  )", func() { g.DelMyApp() },
	)
	g.AddKeymap("E", func() { g.Menu("myApp") })

	menu.Add("image",
		"o", "default    기본열기", func() { g.Spawn(opener) },
		"e", "eog        ", func() { g.Spawn("eog '%~f' %&") },
		"g", "gimp       ", func() { g.Spawn("gimp %m %&") },
	)

	menu.Add("media",
		"o", "default   기본열기", func() { g.Spawn(opener) },
		"m", "mpv               ", func() { g.Spawn("mpv %f") },
		"v", "vlc               ", func() { g.Spawn("vlc %f %&") },
	)

	var associate widget.Keymap

	associate = widget.Keymap{
		".dir":  func() { g.Dir().EnterDir(); g.Workspace().ReloadAll() },
		".exec": func() { g.Shell(" ./" + g.File().Name()) },

		".zip": func() { g.UnZipToHere() },
		".tar": func() { g.UnZipToHere() },
		".gz":  func() { g.UnZipToHere() },
		".tgz": func() { g.UnZipToHere() },
		".bz2": func() { g.UnZipToHere() },
		".xz":  func() { g.UnZipToHere() },
		".txz": func() { g.UnZipToHere() },
		".rar": func() { g.UnZipToHere() },

		".go": func() { g.Shell(`go run %f`) },
		".py": func() { g.Shell(`python %f`) },
		".rb": func() { g.Shell(`ruby %f`) },
		".js": func() { g.Shell(`node %f`) },

		// ".jpg":  func() { g.Menu("image") },
		// ".jpeg": func() { g.Menu("image") },
		// ".gif":  func() { g.Menu("image") },
		// ".png":  func() { g.Menu("image") },
		// ".bmp":  func() { g.Menu("image") },

		// ".avi":  func() { g.Menu("media") },
		// ".mp4":  func() { g.Menu("media") },
		// ".mkv":  func() { g.Menu("media") },
		// ".wmv":  func() { g.Menu("media") },
		// ".flv":  func() { g.Menu("media") },
		// ".mp3":  func() { g.Menu("media") },
		// ".flac": func() { g.Menu("media") },
		// ".tta":  func() { g.Menu("media") },
	}

	g.MergeExtmap(widget.Extmap{
		"C-m": associate, //c-m  = enter
		// "o":     associate,
		"l":     associate,
		"right": associate,
	})
}

// ifElse 함수 정의
func ifElse(condition bool, trueVal func(), falseVal func()) func() {
	if condition {
		return trueVal
	}
	return falseVal
}

// Finder 클립보드에 파일 객체로 복사하는 함수 (macOS 전용)
func copyFilesToFinderClipboard(paths []string) error {
	var sb strings.Builder
	sb.WriteString("set theFiles to {")
	for i, p := range paths {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString("\"" + p + "\"")
	}
	sb.WriteString("}\n")
	sb.WriteString("set theAliasList to {}\n")
	sb.WriteString("repeat with aFile in theFiles\n")
	sb.WriteString("    set end of theAliasList to (POSIX file aFile) as alias\n")
	sb.WriteString("end repeat\n")
	sb.WriteString("tell application \"Finder\"\n")
	sb.WriteString("    set the clipboard to theAliasList\n")
	sb.WriteString("end tell\n")

	script := sb.String()
	cmd := exec.Command("osascript", "-e", script)
	return cmd.Run()
}

func filerKeymap(g *app.Goful) widget.Keymap {
	// Setup open command for C-m (when the enter key is pressed)
	// The macro %f means expanded to a file name, for more see (spawn.go)
	opener := "xdg-open %m %&"
	switch runtime.GOOS {
	case "windows":
		opener = "Invoke-Item  %~c %&" // c meaans comma separated file name
	case "darwin":
		opener = "open %m %&"
	}

	return widget.Keymap{

		"C-[": func() { g.Workspace().ReloadAll(); g.Dir().Reset() }, // C-[ means ESC
		"C-i": func() { g.Workspace().MoveFocus(1) },                 //C-i = tab
		//C-m means Enter key
		//Cmeans Ctrl key, M means Meta key (Alt key)

		"a": func() { g.UnzipToOtherPane() },
		"A": func() { g.UnZipToHere() },
		// "a": func() { g.Shell(`ZIP to neighbor folder [반대쪽에 압축];   7z a '%~D2/%~d.zip' %M`, -7) }, //zip to neighbor folder
		// "A": func() { g.Shell(`ZIP to here [여기에 압축];   7z a '%~d.zip' %M`, -7) },                  //zip to current folder

		// "b": func() { g.Menu("bookmark") }
		// "B": func() { g.Menu("myBookmark") }
		"C-b": func() { g.Workspace().MoveFocus(-1) }, //move to previous window
		"M-b": func() { g.MoveWorkspace(-1) },         //move to previous tab

		"c": func() { g.Copy() }, //copy
		//C-c는 윈도우 기본 단축키가 우선이라 문제가 있을 수 있어서 뺌
		"M-c": ifElse(runtime.GOOS == "windows", func() { //file copy 파일 복사
			myClip := strings.Join(g.Dir().MarkfileDoubleQuotedPaths(), " ")
			glippy.Set(myClip)
			message.Info("Files copied to clipboard (클립보드 복사): " + myClip)
			command := `Add-Type -AssemblyName System.Windows.Forms; $filePaths = [regex]::Matches((Get-Clipboard), '\"(.*?)\"') | ForEach-Object { $_.Groups[1].Value }; if ($filePaths.Count -gt 0) { $stringCollection = New-Object System.Collections.Specialized.StringCollection; $stringCollection.AddRange($filePaths); [System.Windows.Forms.Clipboard]::SetFileDropList($stringCollection) }`

			exec.Command("powershell", "-Command", command).Run()

		}, ifElse(runtime.GOOS == "darwin", func() { // macOS 전용 파일 복사
			filePaths := g.Dir().MarkfilePaths()
			if err := copyFilesToMacClipboard(filePaths); err != nil {
				message.Info("클립보드 복사 실패: " + err.Error())
			} else {
				message.Info(fmt.Sprintf("✅ %d개 파일이 클립보드에 복사되었습니다! (Cmd+V로 붙여넣기)", len(filePaths)))
			}
		}, func() { // Linux 및 기타 OS
			myClip := strings.Join(g.Dir().MarkfileDoubleQuotedPaths(), " ")
			glippy.Set(myClip)
			message.Info("Files copied to clipboard (클립보드 복사): " + myClip)
		})),

		"C": func() { g.Duplicate() }, //duplicate file 복제파일
		// "C": ifElse(runtime.GOOS == "windows", func() { //Duplicate
		// 	g.Shell("duplicate file [파일복제];   Copy-Item -Recurse  '" + strings.ReplaceAll(strings.ReplaceAll(g.File().Name(), "[", "`["), "]", "`]") + "' '" + util.RemoveExt(g.File().Name()) + `_` + util.GetExt((g.File().Name())) + `'`) // WTF?? // fileName having '[ ]' does not work with Invoke-Item.
		// }, func() {
		// 	g.Shell("duplicate file [파일복제];   cp -r %f '" + util.RemoveExt(g.File().Name()) + `_` + util.GetExt((g.File().Name())) + `'`)
		// }),

		"d": func() { g.Remove2() }, //copy file
		// "d": ifElse(runtime.GOOS == "windows", func() { g.Shell(`DELETE files? (파일삭제?);   recycle -s %M `, -7) }, //move file(s) to recycle bin
		// ifElse(runtime.GOOS == "darwin", func() { g.Shell(`echo "Move file(s) to Trash? 휴지통으로 삭제? "; %| `, -7) },
		// func() { g.Shell(`DELETE files? [파일삭제?];   trash %m`, -7) })),

		"D": func() { g.Workspace().ReloadAll(); g.Chdir() }, //change directory

		//"e"   : //open with editor application
		// "E"  : //oenp whtih custom apllication

		"f": func() { g.Dir().Finder() }, //search file
		"/": func() { g.Dir().Finder() }, //search file
		"F": func() { //file Name copy 파일명 복사
			myClip := util.RemoveExt(g.File().Name())
			glippy.Set(myClip)
			message.Info("File name copied (파일명 복사): " + myClip)
		},

		"g": func() { g.Glob() },    //search file in current folder
		"G": func() { g.Globdir() }, //search file in current folder and subfolders

		"h": func() { g.Dir().Chdir("..") },                    //go to parent folder //hjkl ←↓↑→,    ui ↟↡,    ^,U = Home,    $, I = End
		"H": func() { g.Workspace().Dir().GoPreviousFolder() }, //go to previous folder
		// "i": func() { g.Dir().MoveCursor(-5) }, //hjkl ←↓↑→,    ui ↟↡,    ^,U = Home,    $, I = End
		// "I": func() { g.Dir().MoveTop() },      //hjkl ←↓↑→,    ui ↟↡,    ^,U = Home,    $, I = End

		"j": func() { g.Dir().MoveCursor(1) }, //hjkl ←↓↑→,    ui ↟↡,    ^,U = Home,    $, I = End
		"J": func() { g.Dir().MoveCursor(5) },
		"k": func() { g.Dir().MoveCursor(-1) }, //hjkl ←↓↑→,    ui ↟↡,    ^,U = Home,    $, I = End
		"K": func() { g.Dir().MoveCursor(-5) },
		// "l":  open file with default application
		"L": func() {
			ws := g.Workspace()
			if ws == nil {
				return
			}
			dir := ws.Dir()
			if dir == nil {
				return
			}
			dir.GoFowardFolder()
		},
		"m": func() { g.Move() },
		"M": ifElse(runtime.GOOS == "windows", func() { message.Info("Windows doesn't need to CHMOD (윈도우는 권한설정 불필요)") }, func() { g.Chmod() }), //change file permission
		//move file
		"M-m": func() { g.PasteMove() }, //move file 복사파일 이동함
		"c-m": func() { g.PasteMove() }, //move file 복사파일 이동함
		//C-M means enter. open file with default applicationmm

		"n": func() { g.Touch() }, //new file
		"N": func() { g.Mkdir() }, //new folder
		//"o":  open file with default application
		"O": ifElse(runtime.GOOS == "windows", func() { g.Spawn(`explorer . %&`) }, ifElse(runtime.GOOS == "darwin", func() { g.Spawn(`open %D %&`) }, func() { g.Spawn(`xdg-open %D %&`) })), //open folder with file manager

		"p": func() { g.PasteCopy() }, //paste file 복사 파일 붙여넣기

		"P": func() { g.PasteMove() }, //move file 복사파일 이동함

		"q": func() { g.Quit() },
		"Q": func() { g.Workspace().SwapNextDir() },

		"r":   func() { g.Rename() },
		"R":   func() { g.BulkRename() },
		"C-r": func() { g.Workspace().ReloadAll() },

		//"s": sort
		//"S":

		"t": func() { g.Workspace().ReloadAll(); g.MoveWorkspace(1) }, //move to next tab
		//"T": open Tab menu
		"C-t": func() { g.Workspace().ReloadAll(); g.CreateWorkspace() }, //create new tab
		"M-t": func() { g.Workspace().ReloadAll(); g.CloseWorkspace() },  //close tab

		// "u": func() { g.Dir().MoveCursor(5) }, //hjkl ←↓↑→,    ui ↟↡,    ^,U = Home,    $, I = End
		// "U": func() { g.Dir().MoveBottom() },  //hjkl ←↓↑→,    ui ↟↡,    ^,U = Home,    $, I = End

		//"v": view menu
		"M-v": func() { g.PasteCopy() }, //paste file 복사 파일 붙여넣기
		// "C-v": func() { g.PasteCopy() }, //paste file 복사 파일 붙여넣기. 윈도우 기본 단축키가 우선이라 문제 생김

		"w":   func() { g.Workspace().ReloadAll(); g.Workspace().ChdirNeighbor2This() }, //change next window to this folder
		"W":   func() { g.Workspace().ReloadAll(); g.Workspace().ChdirNeighbor() },      // change this window to next folder
		"C-w": func() { g.Workspace().ReloadAll(); g.Workspace().CreateDir() },          //create new window
		"M-w": func() { g.Workspace().ReloadAll(); g.Workspace().CloseDir() },           //close window

		"x": func() { g.Menu("command") },
		"X": func() { g.Menu("external") },

		// "V": ifElse(runtime.GOOS == "windows", func() { // file cut 파일 잘라내기 ... 라고 하지만 복사만 됨. 지우려다가 놔둠
		// 	myClip := strings.Join(g.Dir().MarkfileDoubleQuotedPaths(), " ")
		// 	glippy.Set(myClip)
		// 	message.Info("Files cut to clipboard (클립보드 잘라내기): " + myClip)
		// 	// PowerShell 스크립트: 파일 목록과 함께 '잘라내기' 효과(Move = 2)를 클립보드에 설정
		// 	command := `
		//     Add-Type -AssemblyName System.Windows.Forms;
		//     $filePaths = [regex]::Matches((Get-Clipboard), '\"(.*?)\"') | ForEach-Object { $_.Groups[1].Value };
		//     if ($filePaths.Count -gt 0) {
		//         $stringCollection = New-Object System.Collections.Specialized.StringCollection;
		//         $stringCollection.AddRange($filePaths);

		//         $dataObject = New-Object System.Windows.Forms.DataObject;
		//         $dataObject.SetFileDropList($stringCollection);

		//         # '잘라내기' 효과(Move = 2)를 위한 MemoryStream 생성
		//         $moveEffect = New-Object System.IO.MemoryStream;
		//         $moveEffect.WriteByte(2); # 1은 Copy, 2는 Move

		//         # 클립보드에 데이터와 함께 'Preferred DropEffect'를 설정
		//         $dataObject.SetData("Preferred DropEffect", $moveEffect);
		//         [System.Windows.Forms.Clipboard]::SetDataObject($dataObject, $true);
		//     }
		// `
		// 	exec.Command("powershell", "-Command", command).Run()

		// }, func() { // Non-windows
		// 	myClip := strings.Join(g.Dir().MarkfileDoubleQuotedPaths(), " ")
		// 	glippy.Set(myClip)
		// 	message.Info("Files cut to clipboard (클립보드 잘라내기): " + myClip)
		// }),

		"y": ifElse(runtime.GOOS == "windows", func() { //file copy 파일 복사
			myClip := strings.Join(g.Dir().MarkfileDoubleQuotedPaths(), " ")
			glippy.Set(myClip)
			message.Info("Files copied to clipboard (클립보드 복사): " + myClip)
			command := `Add-Type -AssemblyName System.Windows.Forms; $filePaths = [regex]::Matches((Get-Clipboard), '\"(.*?)\"') | ForEach-Object { $_.Groups[1].Value }; if ($filePaths.Count -gt 0) { $stringCollection = New-Object System.Collections.Specialized.StringCollection; $stringCollection.AddRange($filePaths); [System.Windows.Forms.Clipboard]::SetFileDropList($stringCollection) }`

			exec.Command("powershell", "-Command", command).Run()

		}, ifElse(runtime.GOOS == "darwin", func() { // macOS 전용 파일 복사
			myClip := g.Dir().MarkfilePaths()
			if err := copyFilesToMacClipboard(myClip); err != nil {
				message.Info("클립보드 복사 실패: " + err.Error())
			} else {
				message.Info("Files copied to clipboard (클립보드 복사): " + strings.Join(myClip, ", "))
			}
		}, func() { // Linux 및 기타 OS
			myClip := strings.Join(g.Dir().MarkfileDoubleQuotedPaths(), " ")
			glippy.Set(myClip)
			message.Info("Files copied to clipboard (클립보드 복사): " + myClip)
		})),

		"Y": func() { //Path copy 경로 복사
			myClip := strings.Join(g.Dir().MarkfilePaths(), " ")
			glippy.Set(myClip)
			message.Info("Path copied (경로 복사): " + myClip)
		},

		"z": func() { g.ZipToOtherPane() }, //add file to archive
		"Z": func() { g.ZipToHere() },      //add file to archive

		// 한글 키보드 단축키 (QWERTY -> 한글 정확한 매핑)
		// q w e r t y u i o p
		"ㅂ": func() { g.Quit() },                                                      // q -> ㅂ (quit)
		"ㅈ": func() { g.Workspace().ReloadAll(); g.Workspace().ChdirNeighbor2This() }, // w -> ㅈ (window)
		"ㄷ": func() { g.Menu("editor") },                                              // e -> ㄷ (editor)
		"ㄱ": func() { g.Rename() },                                                    // r -> ㄱ (rename)
		"ㅅ": func() { g.Workspace().ReloadAll(); g.MoveWorkspace(1) },                 // t -> ㅅ (tab)
		"ㅛ": ifElse(runtime.GOOS == "windows", func() { //y -> ㅛ (file copy to clipboard)
			myClip := strings.Join(g.Dir().MarkfileDoubleQuotedPaths(), " ")
			glippy.Set(myClip)
			message.Info("Files copied to clipboard (클립보드 복사): " + myClip)
			command := `Add-Type -AssemblyName System.Windows.Forms; $filePaths = [regex]::Matches((Get-Clipboard), '\"(.*?)\"') | ForEach-Object { $_.Groups[1].Value }; if ($filePaths.Count -gt 0) { $stringCollection = New-Object System.Collections.Specialized.StringCollection; $stringCollection.AddRange($filePaths); [System.Windows.Forms.Clipboard]::SetFileDropList($stringCollection) }`
			exec.Command("powershell", "-Command", command).Run()
		}, ifElse(runtime.GOOS == "darwin", func() { // macOS 전용 파일 복사
			filePaths := g.Dir().MarkfilePaths()
			if err := copyFilesToMacClipboard(filePaths); err != nil {
				message.Info("클립보드 복사 실패: " + err.Error())
			} else {
				message.Info(fmt.Sprintf("✅ %d개 파일이 클립보드에 복사되었습니다! (Cmd+V로 붙여넣기)", len(filePaths)))
			}
		}, func() { // Linux 및 기타 OS
			myClip := strings.Join(g.Dir().MarkfileDoubleQuotedPaths(), " ")
			glippy.Set(myClip)
			message.Info("Files copied to clipboard (클립보드 복사): " + myClip)
		})),
		//"ㅕ": func() { g.Dir().MoveCursor(5) }, // u -> ㅕ (cursor down 5)
		//"ㅑ": func() { g.Dir().MoveCursor(-5) }, // i -> ㅑ (cursor up 5)
		"ㅐ": func() { g.Spawn(opener) }, // o -> ㅐ (open with default application)
		"ㅔ": func() { g.PasteCopy() },   // p -> ㅔ (paste)

		// A S D F G H J K L 대문자
		"ㅁ": func() { g.UnZipToHere() },        // A -> ㅁ (unzip here) - A키와 a키 동일 문자라 구분 안됨, 실제로는 별도 구현 필요
		"ㄴ": func() { g.Mkdir() },              // N -> ㄴ 없음, 하지만 새 폴더는 중요하니 별도 추가 가능
		"ㅇ": func() { g.Remove2() },            // d -> ㅇ (delete)
		"ㄹ": func() { g.Dir().Finder() },       // f -> ㄹ (find)
		"ㅎ": func() { g.Glob() },               // g -> ㅎ (glob)
		"ㅗ": func() { g.Dir().Chdir("..") },    // h -> ㅗ (parent folder)
		"ㅓ": func() { g.Dir().MoveCursor(1) },  // j -> ㅓ (down)
		"ㅏ": func() { g.Dir().MoveCursor(-1) }, // k -> ㅏ (up)
		"ㅣ": func() { g.Spawn(opener) },        // l -> ㅣ (open with default, same as l key)

		// z x c v b n m
		"ㅋ": func() { g.ZipToOtherPane() }, // z -> ㅋ (zip)
		"ㅌ": func() { g.Menu("command") },  // x -> ㅌ (command menu)
		"ㅊ": func() { g.Copy() },           // c -> ㅊ (copy)
		"ㅍ": func() { g.Menu("view") },     // v -> ㅍ (view)
		"ㅠ": func() { g.Menu("bookmark") }, // b -> ㅠ (bookmark)
		"ㅜ": func() { g.Touch() },          // n -> ㅜ (new file)
		"ㅡ": func() { g.Move() },           // m -> ㅡ (move)

		// 대문자/쌍자음 (Shift + 키)
		"ㅃ": func() { g.Workspace().SwapNextDir() },                                                                                                                                           // Q -> ㅃ (swap)
		"ㅉ": func() { g.Workspace().ReloadAll(); g.Workspace().CloseDir() },                                                                                                                   // W -> ㅉ (close window)
		"ㄸ": func() { g.Workspace().ReloadAll(); g.Chdir() },                                                                                                                                  // E -> ㄸ (change directory)
		"ㄲ": func() { g.Globdir() },                                                                                                                                                           // R -> ㄲ (glob in subdirs)
		"ㅆ": func() { g.Menu("tab") },                                                                                                                                                         // T -> 쌰 (tab menu) - 실제로는 ㅆ이 맞지만 쌰로 되어있음
		"ㅒ": ifElse(runtime.GOOS == "windows", func() { g.Spawn(`explorer . %&`) }, ifElse(runtime.GOOS == "darwin", func() { g.Spawn(`open %D %&`) }, func() { g.Spawn(`xdg-open %D %&`) })), //open folder with file manager                                // O -> ㅒ (move to top)
		"ㅖ": func() { g.PasteMove() },                                                                                                                                                         //move file 복사파일 이동함

		// "z": func() { g.Shell(`UnZip to neighbor folder [반대쪽에 압축풀기];   7z x '%~F' -o'%~D2/%~x'`) }, //extract zip file to neighbor folder
		// "Z": func() { g.Shell(`UnZip to here [여기에 압축풀기];   7z x '%~F' -o'%~D/%~x'`) },              //extract zip file to current folder

		// function keys do External command

		// function keys do External command
		"f2": func() { g.Rename2() },
		"f3": ifElse(runtime.GOOS == "windows", func() { g.Spawn(`~/AppData/Local/Programs/QuickLook/quickLook.exe '` + g.File().Path() + `'`) }, ifElse(runtime.GOOS == "darwin", func() { g.Spawn("qlmanage -p " + g.File().Name()) }, func() { g.Spawn(" sushi " + g.File().Path()) })),

		"f5": func() { g.Copy2() },
		"f6": func() { g.Move2() },
		"f7": func() { g.Mkdir2() },  //make new folder
		"f8": func() { g.Remove2() }, //move file(s) to recycle bin

		// "f5": ifElse(runtime.GOOS == "windows", func() {
		// 	g.Shell(`COPY to neighbor folder [반대쪽에 파일 복사];   fcp /cmd=force_copy %M /to='%~D2/'`, -7)
		// }, func() { g.Shell(`COPY to neighbor folder [반대쪽에 파일 복사];   cp -r -v %M %D2`, -7) }),
		// "f6": ifElse(runtime.GOOS == "windows", func() {
		// 	g.Shell(`MOVE to neighbor folder [반대쪽에 파일 이동];   fcp /cmd=move %M /to='%~D2/'`, -7)
		// }, func() { g.Shell(`MOVE to neighbor folder [반대쪽에 파일 이동];   mv -f -v %M %D2`, -7) }),
		// "f7": ifElse(runtime.GOOS == "windows", func() {
		// 	g.Shell(`create FOLDER [폴더만들기];   mkdir ` + `'` + util.RemoveExt(g.File().Name()) + `'`)
		// }, func() {
		// 	g.Shell(`create FOLDER [폴더만들기];  mkdir -vp ` + `'` + util.RemoveExt(g.File().Name()) + `'`)
		// }),
		// "f8": ifElse(runtime.GOOS == "windows", func() { g.Shell(`DELETE file [파일 삭제];   (recycle -s %M `, -7) }, //move file(s) to recycle bin
		// 	ifElse(runtime.GOOS == "darwin", func() { g.Shell(`echo "Move file(s) to Trash? 휴지통으로 삭제? "; %| `, -7) },
		// 		func() { g.Shell(`DELETE file [파일 삭제];   trash %m`, -7) })),
		// //"f9": ifElse(runtime.GOOS == "windows", func() { g.Shell(`mkdir ` + `'` + util.RemoveExt(g.File().Name()) + `'`) }, func() { g.Shell(`mkdir -vp ` + `'` + util.RemoveExt(g.File().Name()) + `'`) }),

		"delete": func() { g.Remove() }, //delete

		"'": func() { g.Dir().Reset(); g.Workspace().ReloadAll() }, //reset

		"~":  func() { g.Dir().Chdir("~") },
		"\\": func() { g.Dir().Chdir("/") },

		// "backspace": func() { g.Dir().Chdir("..") },    //go to parent folder
		"backspace": func() { g.Dir().Chdir("..") },    //go to parent folder
		"left":      func() { g.Dir().Chdir("..") },    //hjkl ←↓↑→,    ui ↟↡,    ^,U = Home,    $, I = End
		"down":      func() { g.Dir().MoveCursor(1) },  //hjkl ←↓↑→,    ui ↟↡,    ^,U = Home,    $, I = End
		"up":        func() { g.Dir().MoveCursor(-1) }, //hjkl ←↓↑→,    ui ↟↡,    ^,U = Home,    $, I = End
		"C-l":       func() { g.HeaderPathEdit() },     //헤더 경로 편집 모드 (자동완성/히스토리)
		"M-l":       func() { g.HeaderPathEdit() },     //헤더 경로 편집 모드 (자동완성/히스토리)
		//"right": open file with default application

		"home": func() { g.Dir().MoveTop() },    //hjkl ←↓↑→,    ui ↟↡,    ^,U = Home,    $, I = End
		"end":  func() { g.Dir().MoveBottom() }, //hjkl ←↓↑→,    ui ↟↡,    ^,U = Home,    $, I = End
		"^":    func() { g.Dir().MoveTop() },    //hjkl ←↓↑→,    ui ↟↡,    ^,U = Home,    $, I = End
		"$":    func() { g.Dir().MoveBottom() }, //hjkl ←↓↑→,    ui ↟↡,    ^,U = Home,    $, I = End
		"pgdn": func() { g.Dir().PageDown() },   //hjkl ←↓↑→,    ui ↟↡,    ^,U = Home,    $, I = End
		"pgup": func() { g.Dir().PageUp() },     //hjkl ←↓↑→,    ui ↟↡,    ^,U = Home,    $, I = End

		" ":       func() { g.Dir().ToggleMark() }, //space key
		"C-space": ifElse(runtime.GOOS == "windows", func() { g.Spawn(`~/AppData/Local/Programs/QuickLook/quickLook.exe '` + g.File().Path() + `'`) }, ifElse(runtime.GOOS == "darwin", func() { g.Spawn("qlmanage -p " + g.File().Name()) }, func() { g.Spawn(" sushi " + g.File().Path()) })),

		"`": func() { g.Dir().InvertMark() },

		";": func() { g.Workspace().ReloadAll(); g.Shell("") },
		":": func() { g.Workspace().ReloadAll(); g.ShellSuspend("") },
	}
}

func finderKeymap(w *filer.Finder) widget.Keymap {
	return widget.Keymap{
		"C-h":       func() { w.DeleteBackwardChar() },
		"backspace": func() { w.DeleteBackwardChar() },
		"C-g":       func() { w.Exit() }, //
		"C-[":       func() { w.Exit() },
	}
}

func cmdlineKeymap(w *cmdline.Cmdline) widget.Keymap {
	return widget.Keymap{
		"C-a":  func() { w.MoveTop() },
		"C-e":  func() { w.MoveBottom() },
		"home": func() { w.MoveTop() },
		"end":  func() { w.MoveBottom() },
		// "C-f":       func() { w.ForwardChar() },
		// "C-b":       func() { w.BackwardChar() },
		"right":     func() { w.ForwardChar() },
		"left":      func() { w.BackwardChar() },
		"C-f":       func() { w.ForwardWord() },
		"C-b":       func() { w.BackwardWord() },
		"C-Left":    func() { w.ForwardWord() },
		"C-Right":   func() { w.BackwardWord() },
		"C-d":       func() { w.DeleteChar() },
		"delete":    func() { w.DeleteChar() },
		"C-h":       func() { w.DeleteBackwardChar() },
		"backspace": func() { w.DeleteBackwardChar() },
		"M-d":       func() { w.DeleteForwardWord() },
		"M-h":       func() { w.DeleteBackwardWord() },
		"C-k":       func() { w.KillLine() },
		"C-i":       func() { w.StartCompletion() }, //C-i = tab
		"C-m":       func() { w.Run() },
		"C-g":       func() { w.Exit() },
		"C-[":       func() { w.Exit() }, // C-[ means ESC
		"C-n":       func() { w.History.CursorDown() },
		"C-p":       func() { w.History.CursorUp() },
		"down":      func() { w.History.CursorDown() },
		"up":        func() { w.History.CursorUp() },
		"C-v":       func() { w.History.PageDown() },
		"M-v":       func() { w.History.PageUp() },
		"pgdn":      func() { w.History.PageDown() },
		"pgup":      func() { w.History.PageUp() },
		"M-<":       func() { w.History.MoveTop() },
		"M->":       func() { w.History.MoveBottom() },
		// "home": func() { w.History.MoveTop() },
		// "end":  func() { w.History.MoveBottom() },
		"M-n": func() { w.History.Scroll(1) },
		"M-p": func() { w.History.Scroll(-1) },
		"C-x": func() { w.History.Delete() },
		// "C-r":       func() { w.History. },
	}
}

func completionKeymap(w *cmdline.Completion) widget.Keymap {
	return widget.Keymap{
		"C-n":   func() { w.CursorDown() },
		"C-p":   func() { w.CursorUp() },
		"down":  func() { w.CursorDown() },
		"up":    func() { w.CursorUp() },
		"C-f":   func() { w.CursorToRight() },
		"C-b":   func() { w.CursorToLeft() },
		"right": func() { w.CursorToRight() },
		"left":  func() { w.CursorToLeft() },
		// "home":  func() { w.MoveCursor(-30) },
		// "end":   func() { w.MoveCursor(+30) },

		"C-v":  func() { w.PageDown() },
		"M-v":  func() { w.PageUp() },
		"pgdn": func() { w.PageDown() },
		"pgup": func() { w.PageUp() },
		"M-<":  func() { w.MoveTop() },
		"M->":  func() { w.MoveBottom() },
		"M-n":  func() { w.Scroll(1) },
		"M-p":  func() { w.Scroll(-1) },
		"C-i":  func() { w.InsertCompletion() }, //C-i = tab
		"C-m":  func() { w.InsertCompletion() },
		"C-g":  func() { w.Exit() },
		"C-[":  func() { w.Exit() },
	}
}

func menuKeymap(w *menu.Menu) widget.Keymap {
	return widget.Keymap{
		// "C-n":  func() { w.MoveCursor(1) },
		// "C-p":  func() { w.MoveCursor(-1) },
		"down":      func() { w.MoveCursor(1) },
		"up":        func() { w.MoveCursor(-1) },
		"C-v":       func() { w.PageDown() },
		"M-v":       func() { w.PageUp() },
		"M->":       func() { w.MoveBottom() },
		"M-<":       func() { w.MoveTop() },
		"delete":    func() { w.RemoveMenuInWindow() },
		"backspace": func() { w.RemoveMenuInWindow() },
		"C-m":       func() { w.Exec() }, //C-m = enter
		"C-g":       func() { w.Exit() },
		"C-[":       func() { w.Exit() }, //// C-[ means ESC //
	}
}

func ExpandPath(name string) string {
	if name == "" {
		return ""
	}
	if name == "~" || strings.HasPrefix(name, "~/") || strings.HasPrefix(name, "~\\") {
		home, _ := os.UserHomeDir()
		if name == "~" {
			name = home
		} else if strings.HasPrefix(name, "~/") || strings.HasPrefix(name, "~\\") {
			name = home + name[1:]
		}
	}
	if runtime.GOOS == "windows" {
		name = strings.Replace(strings.Replace(name, `\\`, `/`, -1), `"`, `'`, -1)
	}
	// 반드시 절대경로로 변환
	abs, err := filepath.Abs(name)
	if err == nil {
		return abs
	}
	return name
}
