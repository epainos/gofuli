package main

import (
	"os"
	"runtime"
	"strings"

	"github.com/anmitsu/goful/app"
	"github.com/anmitsu/goful/cmdline"
	"github.com/anmitsu/goful/filer"
	"github.com/anmitsu/goful/look"
	"github.com/anmitsu/goful/menu"
	"github.com/anmitsu/goful/message"
	"github.com/anmitsu/goful/util"
	"github.com/anmitsu/goful/widget"
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
	const history4Dir = "~/.goful/history/dir"

	goful := app.NewGoful(state)
	config(goful, is_tmux)
	_ = cmdline.LoadHistory(history)

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
	message.Sec(5)                                // display second for a message

	// Setup widget keymaps.
	g.ConfigFiler(filerKeymap)
	filer.ConfigFinder(finderKeymap)
	cmdline.Config(cmdlineKeymap)
	cmdline.ConfigCompletion(completionKeymap)
	menu.Config(menuKeymap)

	filer.SetStatView(true, false, false) // size, permission and time
	filer.SetTimeFormat("06-01-02 15:04") // ex: "Jan _2 15:04"

	// Setup open command for C-m (when the enter key is pressed)
	// The macro %f means expanded to a file name, for more see (spawn.go)
	opener := "xdg-open %f %&"
	switch runtime.GOOS {
	case "windows":
		opener = "explorer '%~f' %&"
	case "darwin":
		opener = "open %f %&"
	}
	g.MergeKeymap(widget.Keymap{
		"C-m": func() { g.Spawn(opener) }, // C-m means Enter key
		// "o":     func() { g.Spawn(opener) },
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

	} else {
		g.ConfigShell(func(cmd string) []string {
			return []string{"bash", "-c", cmd}
		})
		g.ConfigTerminal(func(cmd string) []string {
			// for not close the terminal when the shell finishes running
			const tail = `;read -p "HIT ENTER KEY"`

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
		"c", "copy            복사        ", func() { g.Copy() },
		"m", "move            이동        ", func() { g.Move() },
		"D", "delete          삭제      ", func() { g.Remove() },
		"k", "mkdir           폴더생성       ", func() { g.Mkdir() },
		"n", "newfile         파일생성     ", func() { g.Touch() },
		"M", "chmod           권한수정       ", func() { g.Chmod() },
		"r", "rename          이름변경      ", func() { g.Rename() },
		"R", "bulk rename     이름 일괄 변경 ", func() { g.BulkRename() },
		"d", "chdir           경로 이동       ", func() { g.Chdir() },
		"g", "glob            찾기 ", func() { g.Glob() },
		"G", "globdir         찾기(하부폴더)", func() { g.Globdir() },
		"b", "go pre dir      폴더 뒤로 가기", func() { g.Dir().GoPreviousFolder() },
		"f", "go forward dir  폴더 앞으로 가기", func() { g.Dir().GoFowardFolder() },
	)
	g.AddKeymap("x", func() { g.Menu("command") })

	if runtime.GOOS == "windows" {
		menu.Add("external-command",
			"c", "copy %m to %D2   복사", ifElse(runtime.GOOS == "windows", func() { g.Shell(`fcp /cmd=force_copy %M /to=%D2`, -7) }, func() { g.Shell(`cp -r -v %M %D2`, -7) }), //func() { g.Shell(`fcp /cmd=diff '%~F' /to='%~D2'`) },
			"m", "move %m to %D2   이동", ifElse(runtime.GOOS == "windows", func() { g.Shell(`fcp /cmd=move %M /to=%D2`, -7) }, func() { g.Shell(`cp -r -v %M %D2`, -7) }),
			"d", "del /s %M        삭제", ifElse(runtime.GOOS == "windows", func() { g.Shell(`fcp /cmd=delete %M `, -7) }, func() { g.Remove() }),
			// "D", "rd /s /q %~m     폴더 삭제", func() { g.Shell("rd /s /q %~m") },
			"k", "make directory   새폴더", ifElse(runtime.GOOS == "windows", func() { g.Shell(`mkdir ` + `'` + util.RemoveExt(g.File().Name()) + `'`) }, func() { g.Shell(`mkdir -vp ` + `'./` + util.RemoveExt(g.File().Name()) + `'`) }),
			"n", "create newfile   새파일", func() { g.Shell("copy nul ") },
			"r", "move (rename) %f 이름변경", ifElse(runtime.GOOS == "windows", func() { g.Shell("move %F './" + g.File().Name() + `'`) }, func() { g.Shell("mv -vi %f '" + g.File().Name() + `'`) }),
			"R", "bulk rename      이름 일괄 변경 ", func() { g.BulkRename() },
			"w", "where . *        찾기", func() { g.Shell("where . *") },
			// "A", "archives menu     ", func() { g.Menu("archive") },
		)
	} else {
		menu.Add("external-command",
			"c", "copy %m to %D2      복사", ifElse(runtime.GOOS == "windows", func() { g.Shell(`fcp /cmd=force_copy %M /to=%D2`, -7) }, func() { g.Shell(`cp -r -v %M %D2`, -7) }), //func() { g.Shell("cp -vai %m %D2") },
			"m", "move %m to %D2      이동", ifElse(runtime.GOOS == "windows", func() { g.Shell(`fcp /cmd=move %M /to=%D2`, -7) }, func() { g.Shell(`cp -r -v %M %D2`, -7) }), //func() { g.Shell("mv -vi %m %D2") },
			"D", "remove %m files     삭제", ifElse(runtime.GOOS == "windows", func() { g.Shell(`fcp /cmd=delete %M `, -7) }, func() { g.Remove() }),
			"k", "make directory      새폴더", ifElse(runtime.GOOS == "windows", func() { g.Shell(`mkdir ` + `'` + util.RemoveExt(g.File().Name()) + `'`) }, func() { g.Shell(`mkdir -vp ` + `'./` + util.RemoveExt(g.File().Name()) + `'`) }),
			"n", "create newfile      새파일", func() { g.Shell("touch './" + g.File().Name() + `'`) },
			"T", "time copy %f to %m  시간복사", func() { g.Shell("touch -r %f %m") },
			"M", "change mode %m      권한변경", func() { g.Shell("chmod 644 %m", -3) },
			"r", "move (rename) %f    이름변경", ifElse(runtime.GOOS == "windows", func() { g.Shell("move %F './" + g.File().Name() + `'`) }, func() { g.Shell("mv -vi %f '" + g.File().Name() + `'`) }),
			"R", "bulk rename %m      이름변경(대량)", func() { g.Shell(`rename -v "s///" %m`, -6) },
			"f", "find . -name        찾기", func() { g.Shell(`find . -name "*"`, -1) },
			// "A", "archives menu     ", func() { g.Menu("archive") },
		)
	}
	g.AddKeymap("X", func() { g.Menu("external-command") })

	menu.Add("tab",
		"n", "New tab       	새탭        ", func() { g.CreateWorkspace(); g.MoveWorkspace(1) },
		"c", "close tab     	탭 닫기     ", func() { g.CloseWorkspace() },
		"t", "changeTitle   	탭이름 변경    ", func() { g.ChangeWorkspaceTitle() },
		"T", "changeTitle   	탭이름 변경    ", func() { g.ChangeWorkspaceTitle() },
		"f", "Forward tab   	앞탭으로      ", func() { g.MoveWorkspace(1) },
		"b", "Backward tab  	뒷탭으로       ", func() { g.MoveWorkspace(-1) },
		"s", "Swap next dir 	앞창으로 바꿈       ", func() { g.Workspace().SwapNextDir() },
		"S", "Swap prev dir 	뒷창으로 바꿈       ", func() { g.Workspace().SwapPrevDir() },
		"r", "Reload all    	모두 다시 읽음       ", func() { g.Workspace().ReloadAll() },
		"o", "Open new dir  	창 추가       ", func() { g.Workspace().CreateDir() },
		"O", "clOse dir     	창 닫기       ", func() { g.Workspace().CloseDir() },
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
		"d", "~/            홈", func() { g.Dir().Chdir("~") },
		"k", "~/Desktop     바탕화면 ", func() { g.Dir().Chdir("~/Desktop") },
		"c", "~/Documents   내문서", func() { g.Dir().Chdir("~/Documents") },
		"l", "~/Downloads   다운로드", func() { g.Dir().Chdir("~/Downloads") },
	)
	if runtime.GOOS == "windows" {
		menu.Add("bookmark",
			"A", "C:/", func() { g.Dir().Chdir("A:/") },
			"C", "C:/", func() { g.Dir().Chdir("C:/") },
			"D", "D:/", func() { g.Dir().Chdir("D:/") },
			"E", "E:/", func() { g.Dir().Chdir("E:/") },
			"X", "D:/", func() { g.Dir().Chdir("X:/") },
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

	menu.Add("editor",
		"e", "vscodE       코드 ", func() { g.Spawn("code %f %&") },
		"E", "Emacs client 이맥스 ", func() { g.Spawn("emacsclient -n %f %&") },
		"v", "Vim          빔 ", ifElse(runtime.GOOS == "windows", func() { g.Spawn(`gvim '"%~F"'`) }, func() { g.Spawn("vim %f") }),
		"x", "eXcel        엑셀", ifElse(runtime.GOOS == "windows", func() { g.Spawn(`start 'C:/Program Files/Microsoft Office/root/Office16/excel.exe' '"%~F"'`) }, func() { g.Spawn(`open -a "Microsoft Excel"  %f %&`) }),
		"c", "Chrome       크롬", ifElse(runtime.GOOS == "windows", func() { g.Spawn(`start 'C:/Program Files/Google/Chrome/Application/chrome' '"%~F"'`) }, func() { g.Spawn(`open -a "Google Chrome" %f %&`) }),
	)
	g.AddKeymap("e", func() { g.Menu("editor") })

	menu.Add("image",
		"x", "default    기본열기", func() { g.Spawn(opener) },
		"e", "eog        ", func() { g.Spawn("eog '%~f' %&") },
		"g", "gimp       ", func() { g.Spawn("gimp %m %&") },
	)

	menu.Add("media",
		"x", "default   기본열기", func() { g.Spawn(opener) },
		"m", "mpv               ", func() { g.Spawn("mpv %f") },
		"v", "vlc               ", func() { g.Spawn("vlc %f %&") },
	)

	var associate widget.Keymap

	associate = widget.Keymap{
		".dir":  func() { g.Dir().EnterDir() },
		".exec": func() { g.Shell(" ./" + g.File().Name()) },

		".zip": func() { g.Shell(`7z x '%~F' -o'%~D/%~x'`) },
		".tar": func() { g.Shell(`7z x '%~F' -o'%~D/%~x'`) }, //func() { g.Shell(`tar xvf %f -C %D`) },
		".gz":  func() { g.Shell(`7z x '%~F' -o'%~D/%~x'`) }, //func() { g.Shell(`tar xvfz %f -C %D`) },
		".tgz": func() { g.Shell(`7z x '%~F' -o'%~D/%~x'`) }, //func() { g.Shell(`tar xvfz %f -C %D`) },
		".bz2": func() { g.Shell(`7z x '%~F' -o'%~D/%~x'`) }, //func() { g.Shell(`tar xvfj %f -C %D`) },
		".xz":  func() { g.Shell(`7z x '%~F' -o'%~D/%~x'`) }, //func() { g.Shell(`tar xvfJ %f -C %D`) },
		".txz": func() { g.Shell(`7z x '%~F' -o'%~D/%~x'`) }, //func() { g.Shell(`tar xvfJ %f -C %D`) },
		".rar": func() { g.Shell(`7z x '%~F' -o'%~D/%~x'`) }, //func() { g.Shell(`unrar x %f -C %D`) },

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

// Widget keymap functions.

func filerKeymap(g *app.Goful) widget.Keymap {
	// Setup open command for C-m (when the enter key is pressed)
	// The macro %f means expanded to a file name, for more see (spawn.go)
	opener := "xdg-open %f %&"
	switch runtime.GOOS {
	case "windows":
		opener = "explorer '%~f' %&"
	case "darwin":
		opener = "open %f %&"
	}
	openerCurrentDir := "xdg-open %D %&"
	switch runtime.GOOS {
	case "windows":
		openerCurrentDir = "explorer '%~D' %&"
	case "darwin":
		openerCurrentDir = "open %D %&"
	}
	return widget.Keymap{
		"M-C-o": func() { g.CreateWorkspace() },
		"M-C-w": func() { g.CloseWorkspace() },
		"M-f":   func() { g.MoveWorkspace(1) },
		"M-b":   func() { g.MoveWorkspace(-1) },
		"C-o":   func() { g.Workspace().CreateDir() },
		"C-w":   func() { g.Workspace().CloseDir() },
		"C-l":   func() { g.Workspace().ReloadAll() },
		"'":     func() { g.Dir().Reset(); g.Workspace().ReloadAll() },
		"A":     func() { g.Shell(`7z a '%~d.zip' %M`, -7) },                                 //같은 창에 압축파일 생성
		"a":     func() { g.Shell(`7z a '%~D2/%~d.zip' %M`, -7); g.Workspace().ReloadAll() }, //반대쪽 창에 압축파일 생성

		"Z": func() { g.Shell(`7z x '%~F' -o'%~D/%~x'`) },
		"z": func() { g.Shell(`7z x '%~F' -o'%~D2/%~x'`) },
		// "z":   ifElse(runtime.GOOS == "windows", func() { g.Shell("unzip %~f -d %~D2\\\\temp") }, func() { g.Shell("unzip %f -d %D2/temp") }),
		// "Z":   ifElse(runtime.GOOS == "windows", func() { g.Shell("unzip %~f -d %~D\\\\temp") }, func() { g.Shell("unzip %f -d %D/temp") }),
		"C-f": func() { g.Workspace().MoveFocus(1) },
		"C-b": func() { g.Workspace().MoveFocus(-1) },

		// "right": func() { g.Workspace().MoveFocus(1) },
		// "left":  func() { g.Workspace().MoveFocus(-1) },
		"left": func() { g.Dir().Chdir("..") },
		"C-i":  func() { g.Workspace().MoveFocus(1) }, //C-i = tab
		// "l":         func() { g.Workspace().MoveFocus(1) },

		"h": func() { g.Dir().Chdir("..") },
		"Q": func() { g.Workspace().SwapNextDir() },
		// "F": func() { g.Workspace().SwapNextDir() },
		// "B": func() { g.Workspace().SwapPrevDir() },
		"w": func() { g.Workspace().ChdirNeighbor2This() },
		"W": func() { g.Workspace().ChdirNeighbor() },
		// "t": func() { message.Info("t  입력 "); g.ChangeWorkspaceTitle() }

		// "C-h":       func() { g.Dir().Chdir("..") },
		"backspace": func() { g.Dir().Chdir("..") },
		// "u":         func() { g.Dir().Chdir("..") },
		"~":  func() { g.Dir().Chdir("~") },
		"\\": func() { g.Dir().Chdir("/") },
		"B":  func() { g.Workspace().Dir().GoPreviousFolder() },
		"F":  func() { g.Workspace().Dir().GoFowardFolder() },

		"C-n":  func() { g.Dir().MoveCursor(1) },
		"C-p":  func() { g.Dir().MoveCursor(-1) },
		"down": func() { g.Dir().MoveCursor(1) },
		"up":   func() { g.Dir().MoveCursor(-1) },
		"j":    func() { g.Dir().MoveCursor(1) },
		"k":    func() { g.Dir().MoveCursor(-1) },
		"C-d":  func() { g.Dir().MoveCursor(5) },
		"C-u":  func() { g.Dir().MoveCursor(-5) },
		"u":    func() { g.Dir().MoveCursor(5) },
		"i":    func() { g.Dir().MoveCursor(-5) },
		"U":    func() { g.Dir().MoveBottom() },
		"I":    func() { g.Dir().MoveTop() },
		// "C-a":  func() { g.Dir().MoveTop() },
		// "C-e":  func() { g.Dir().MoveBottom() },
		"home":  func() { g.Dir().MoveTop() },
		"end":   func() { g.Dir().MoveBottom() },
		"^":     func() { g.Dir().MoveTop() },
		"$":     func() { g.Dir().MoveBottom() },
		"M-n":   func() { g.Dir().Scroll(1) },
		"M-p":   func() { g.Dir().Scroll(-1) },
		"C-v":   func() { g.Dir().PageDown() },
		"M-v":   func() { g.Dir().PageUp() },
		"pgdn":  func() { g.Dir().PageDown() },
		"pgup":  func() { g.Dir().PageUp() },
		" ":     func() { g.Dir().ToggleMark() },
		"C- ":   func() { g.Dir().InvertMark() },
		"`":     func() { g.Dir().InvertMark() },
		"C-g":   func() { g.Dir().Reset() },
		"C-[":   func() { g.Dir().Reset() }, // C-[ means ESC
		"f":     func() { g.Dir().Finder() },
		"/":     func() { g.Dir().Finder() },
		"q":     func() { g.Quit() },
		";":     func() { g.Shell("") },
		":":     func() { g.ShellSuspend("") },
		"M-C-t": func() { g.CloseWorkspace() },
		// "T":     func() { g.ChangeWorkspaceTitle() },
		"C-t": func() { g.CreateWorkspace() },
		"t":   func() { g.MoveWorkspace(1) },
		"n":   func() { g.Touch() },
		"K":   func() { g.Mkdir() },
		"f5":  ifElse(runtime.GOOS == "windows", func() { g.Shell(`fcp /cmd=force_copy %M /to=%D2`, -7) }, func() { g.Shell(`cp -r -v %M %D2`, -7) }),
		"f6":  ifElse(runtime.GOOS == "windows", func() { g.Shell(`fcp /cmd=move %M /to=%D2`, -7) }, func() { g.Shell(`mv -f -v %M %D2`, -7) }),
		"f2":  ifElse(runtime.GOOS == "windows", func() { g.Shell("move %F './" + g.File().Name() + `'`) }, func() { g.Shell("mv -vi %f '" + g.File().Name() + `'`) }),
		"f7":  ifElse(runtime.GOOS == "windows", func() { g.Shell(`mkdir ` + `'` + util.RemoveExt(g.File().Name()) + `'`) }, func() { g.Shell(`mkdir -vp ` + `'` + util.RemoveExt(g.File().Name()) + `'`) }),
		"f8":  ifElse(runtime.GOOS == "windows", func() { g.Shell(`fcp /cmd=delete %M `, -7) }, func() { g.Remove() }),
		"c":   func() { g.Copy() },
		"C": ifElse(runtime.GOOS == "windows", func() {
			g.Shell("Copy-Item -Recurse %F '" + util.RemoveExt(g.File().Name()) + `_` + util.GetExt((g.File().Name())) + `'`)
		}, func() {
			g.Shell("cp -r %f '" + util.RemoveExt(g.File().Name()) + `_` + util.GetExt((g.File().Name())) + `'`)
		}),
		"m":      func() { g.Move() },
		"r":      func() { g.Rename() },
		"R":      func() { g.BulkRename() },
		"d":      func() { g.Remove() },
		"delete": func() { g.Remove() },
		"D":      func() { g.Chdir() },
		"g":      func() { g.Glob() },
		"G":      func() { g.Globdir() },
		"o":      func() { g.Spawn(opener) },
		"O":      func() { g.Spawn(openerCurrentDir) },
		"N": func() { //file Name copy 파일명 복사
			myClip := util.RemoveExt(g.File().Name())
			glippy.Set(myClip)
			message.Info("file Name copied(파일명 복사함): " + myClip)
		},
		"Y": func() { //Path copy 경로 복사
			myClip := util.RemoveExt(g.File().Path())
			glippy.Set(myClip)
			message.Info("path copied(경로 복사함): " + myClip)
		},
		"y": func() { //file copy 파일 복사
			myClip := strings.Join(g.Dir().MarkfileQuotedPaths(), " ")
			glippy.Set(myClip)
			message.Info("Yanked file(파일 복사함)): " + myClip)
			g.Dir().Reset()
			g.Workspace().ReloadAll()
		},
		"p": //paste file 복사 파일 붙여넣기
		ifElse(runtime.GOOS == "windows", func() {
			value, _ := glippy.Get()
			g.Shell(`fcp /cmd=force_copy `+value+` /to='%~D'`, -7)
			g.Workspace().ReloadAll()
		}, func() {
			value, _ := glippy.Get()
			g.Shell(`cp -r -v `+value+` %D`, -7)
			g.Dir().Reset()
			g.Workspace().ReloadAll()
			// message.Info("Pasted (복사 완료) ")

		}),
		"P": //move file 복사파일 이동함
		ifElse(runtime.GOOS == "windows", func() {
			value, _ := glippy.Get()
			g.Shell(`fcp /cmd=Move `+value+` /to='%~D'`, -7)
			g.Workspace().ReloadAll()
		}, func() {
			value, _ := glippy.Get()
			g.Shell(`mv -f -v `+value+` %D`, -7)
			g.Dir().Reset()
			g.Workspace().ReloadAll()
			// message.Info("Moved (이동 완료) ")

		}),
	}
}

func finderKeymap(w *filer.Finder) widget.Keymap {
	return widget.Keymap{
		"C-h":       func() { w.DeleteBackwardChar() },
		"backspace": func() { w.DeleteBackwardChar() },
		"C-7":       func() { w.MoveHistory(1); message.Info("C-=") },
		"C-8":       func() { w.MoveHistory(-1); message.Info("C--") },
		"C-g":       func() { w.Exit() },
		"C-[":       func() { w.Exit() },
	}
}

func cmdlineKeymap(w *cmdline.Cmdline) widget.Keymap {
	return widget.Keymap{
		"C-a": func() { w.MoveTop() },
		"C-e": func() { w.MoveBottom() },
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
		"home":      func() { w.History.MoveTop() },
		"end":       func() { w.History.MoveBottom() },
		"M-n":       func() { w.History.Scroll(1) },
		"M-p":       func() { w.History.Scroll(-1) },
		"C-x":       func() { w.History.Delete() },
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
		// "C-i":   func() { w.CursorToRight() }, //C-i = tab
		"C-v":  func() { w.PageDown() },
		"M-v":  func() { w.PageUp() },
		"pgdn": func() { w.PageDown() },
		"pgup": func() { w.PageUp() },
		"M-<":  func() { w.MoveTop() },
		"M->":  func() { w.MoveBottom() },
		"home": func() { w.MoveTop() },
		"end":  func() { w.MoveBottom() },
		"M-n":  func() { w.Scroll(1) },
		"M-p":  func() { w.Scroll(-1) },
		"C-i":  func() { w.InsertCompletion() }, //C-i = tab
		"C-m":  func() { w.InsertCompletion() },
		"C-g":  func() { w.Exit() },
		"C-[":  func() { w.Exit() },
		// "C-]":   func() { w.lo },
	}
}

func menuKeymap(w *menu.Menu) widget.Keymap {
	return widget.Keymap{
		"C-n":  func() { w.MoveCursor(1) },
		"C-p":  func() { w.MoveCursor(-1) },
		"down": func() { w.MoveCursor(1) },
		"up":   func() { w.MoveCursor(-1) },
		"C-v":  func() { w.PageDown() },
		"M-v":  func() { w.PageUp() },
		"M->":  func() { w.MoveBottom() },
		"M-<":  func() { w.MoveTop() },
		"C-m":  func() { w.Exec() },
		"C-g":  func() { w.Exit() },
		"C-[":  func() { w.Exit() },
	}
}
