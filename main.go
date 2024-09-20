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
	filer.SetTimeFormat("060102_15:04")   // ex: "Jan _2 15:04"

	// Setup open command for C-m (when the enter key is pressed)
	// The macro %f means expanded to a file name, for more see (spawn.go)
	opener := "xdg-open %m %&"
	switch runtime.GOOS {
	case "windows":
		opener = "Invoke-Item  %c %&"
		// opener = "explorer '%~f' %&" //windows
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
		"c", "(c) copy            복사        ", func() { g.Copy() },
		"m", "(m) move            이동        ", func() { g.Move() },
		"d", "(delete)            삭제      ", func() { g.Remove() },
		"k", "(K) mkdir           폴더생성       ", func() { g.Mkdir() },
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

	if runtime.GOOS == "windows" {
		menu.Add("external-command",
			"c", "(f5) copy %m to %D2   복사", ifElse(runtime.GOOS == "windows", func() { g.Shell(`fcp /cmd=force_copy %M /to='%~D2/'`, -7) }, func() { g.Shell(`cp -r -v %M %D2`, -7) }), //func() { g.Shell(`fcp /cmd=diff '%~F' /to='%~D2'`) },
			"m", "(f6) move %m to %D2   이동", ifElse(runtime.GOOS == "windows", func() { g.Shell(`fcp /cmd=move %M /to='%~D2/'`, -7) }, func() { g.Shell(`cp -r -v %M %D2`, -7) }),
			"k", "(f7) make directory   새폴더", ifElse(runtime.GOOS == "windows", func() { g.Shell(`mkdir ` + `'` + util.RemoveExt(g.File().Name()) + `'`) }, func() { g.Shell(`mkdir -vp ` + `'./` + util.RemoveExt(g.File().Name()) + `'`) }),
			"d", "(f8) del /s %M        삭제", ifElse(runtime.GOOS == "windows", func() { g.Shell(`recycle -s %M `, -7) }, ifElse(runtime.GOOS == "darwin", func() { g.Shell(`mv %M ~/.Trash`, -7) }, func() { g.Shell(`mv %M ~/.local/share/Trash`, -7) })),
			// "D", "rd /s /q %~m     폴더 삭제", func() { g.Shell("rd /s /q %~m") },
			"n", "(n)  create newfile   새파일", func() { g.Shell("copy nul ") },
			"r", "(r)  move (rename) %f 이름변경", ifElse(runtime.GOOS == "windows", func() { g.Shell("move %F './" + g.File().Name() + `'`) }, func() { g.Shell("mv -vi %f '" + g.File().Name() + `'`) }),
			"R", "(R)  bulk rename      이름 일괄 변경 ", func() { g.BulkRename() },
			"w", "     where . *        찾기", func() { g.Shell("where . *") },
			// "A", "archives menu     ", func() { g.Menu("archive") },
		)
	} else {
		menu.Add("external-command",
			"c", "(f5) copy %m to %D2      복사", ifElse(runtime.GOOS == "windows", func() { g.Shell(`fcp /cmd=force_copy %M /to='%~D2/'`, -7) }, func() { g.Shell(`cp -r -v %M %D2`, -7) }), //func() { g.Shell("cp -vai %m %D2") },
			"m", "(f6) move %m to %D2      이동", ifElse(runtime.GOOS == "windows", func() { g.Shell(`fcp /cmd=move %M /to='%~D2/'`, -7) }, func() { g.Shell(`cp -r -v %M %D2`, -7) }), //func() { g.Shell("mv -vi %m %D2") },
			"k", "(f7) make directory      새폴더", ifElse(runtime.GOOS == "windows", func() { g.Shell(`mkdir ` + `'` + util.RemoveExt(g.File().Name()) + `'`) }, func() { g.Shell(`mkdir -vp ` + `'./` + util.RemoveExt(g.File().Name()) + `'`) }),
			"D", "(f8) remove %m files     삭제", ifElse(runtime.GOOS == "windows", func() { g.Shell(`recycle -s %M `, -7) }, ifElse(runtime.GOOS == "darwin", func() { g.Shell(`mv %M ~/.Trash`, -7) }, func() { g.Shell(`mv %M ~/.local/share/Trash`, -7) })),
			"n", "(n)  create newfile      새파일", func() { g.Shell("touch './" + g.File().Name() + `'`) },
			"T", "     time copy %f to %m  시간복사", func() { g.Shell("touch -r %f %m") },
			"M", "(m)  change mode %m      권한변경", func() { g.Shell("chmod 644 %m", -3) },
			"r", "(r)  move (rename) %f    이름변경", ifElse(runtime.GOOS == "windows", func() { g.Shell("move %F './" + g.File().Name() + `'`) }, func() { g.Shell("mv -vi %f '" + g.File().Name() + `'`) }),
			"R", "(R)  bulk rename %m      이름변경(대량)", func() { g.Shell(`rename -v "s///" %m`, -6) },
			"f", "(f)  find . -name        찾기", func() { g.Shell(`find . -name "*"`, -1) },
			// "A", "archives menu     ", func() { g.Menu("archive") },
		)
	}
	g.AddKeymap("X", func() { g.Menu("external-command") })

	menu.Add("tab",
		"t", "(C-t)   New tab          새탭        ", func() { g.CreateWorkspace(); g.MoveWorkspace(1) },
		"T", "(M-c)   close tab        탭 닫기     ", func() { g.CloseWorkspace() },
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
		"d", "~/            홈", func() { g.Dir().Chdir("~") },
		"k", "~/Desktop     바탕화면 ", func() { g.Dir().Chdir("~/Desktop") },
		"c", "~/Documents   내문서", func() { g.Dir().Chdir("~/Documents") },
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
		".dir":  func() { g.Dir().EnterDir(); g.Workspace().ReloadAll() },
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

	return widget.Keymap{
		"C-[": func() { g.Workspace().ReloadAll(); g.Dir().Reset() }, // C-[ means ESC
		"C-i": func() { g.Workspace().MoveFocus(1) },                 //C-i = tab
		//C-m means Enter key
		//C means Ctrl key, M means Meta key (Alt key)

		"a": func() { g.Shell(`7z a '%~D2/%~d.zip' %M`, -7) }, //zip to neighbor folder
		"A": func() { g.Shell(`7z a '%~d.zip' %M`, -7) },      //zip to current folder

		//b: func() { g.Menu("bookmark") }
		"B":   func() { g.Workspace().Dir().GoPreviousFolder() }, //go to previous folder
		"C-b": func() { g.Workspace().MoveFocus(-1) },            //move to previous window
		"M-b": func() { g.MoveWorkspace(-1) },                    //move to previous tab

		"c": func() { g.Copy() }, //copy

		"C": ifElse(runtime.GOOS == "windows", func() { //Duplicate
			g.Shell("Copy-Item -Recurse %F '" + util.RemoveExt(g.File().Name()) + `_` + util.GetExt((g.File().Name())) + `'`)
		}, func() {
			g.Shell("cp -r %f '" + util.RemoveExt(g.File().Name()) + `_` + util.GetExt((g.File().Name())) + `'`)
		}),

		"d": ifElse(runtime.GOOS == "windows", func() { g.Shell(`recycle -s %M `, -7) }, //move file(s) to recycle bin
			ifElse(runtime.GOOS == "darwin", func() { g.Shell(`echo "Move file(s) to Trash? 휴지통으로 삭제? "; %| `, -7) },
				func() { g.Shell(`mv %M ~/.local/share/Trash`, -7) })),
		// "d":      ifElse(runtime.GOOS == "windows", func() { g.Shell(`recycle -s %M `, -7) }, ifElse(runtime.GOOS == "darwin", func() { g.Shell(`mv %M ~/.Trash`, -7) }, func() { g.Shell(`mv %M ~/.local/share/Trash`, -7) })),
		"D": func() { g.Workspace().ReloadAll(); g.Chdir() }, //change directory

		//"e"   : func() { g.Shell("xdg-open %f") },	//open with default application
		//"E"   :

		"f": func() { g.Dir().Finder() }, //search file
		"/": func() { g.Dir().Finder() }, //search file
		"F": func() { g.Workspace().Dir().GoFowardFolder() },

		"g": func() { g.Glob() },    //search file in current folder
		"G": func() { g.Globdir() }, //search file in current folder and subfolders

		"h": func() { g.Dir().Chdir("..") }, //go to parent folder //hjkl ←↓↑→,    ui ↟↡,    ^,U = Home,    $, I = End
		//"H":
		"i": func() { g.Dir().MoveCursor(-5) }, //hjkl ←↓↑→,    ui ↟↡,    ^,U = Home,    $, I = End
		"I": func() { g.Dir().MoveTop() },      //hjkl ←↓↑→,    ui ↟↡,    ^,U = Home,    $, I = End

		"j": func() { g.Dir().MoveCursor(1) }, //hjkl ←↓↑→,    ui ↟↡,    ^,U = Home,    $, I = End
		//"J":
		"k": func() { g.Dir().MoveCursor(-1) }, //hjkl ←↓↑→,    ui ↟↡,    ^,U = Home,    $, I = End
		"K": func() { g.Mkdir() },              //make directory
		// "l":  open file with default application
		//"L":
		"m": func() { g.Move() }, //move file
		//"M":
		//C-M means enter. open file with default application

		"n": func() { g.Touch() }, //new file
		"N": func() { //file Name copy 파일명 복사
			myClip := util.RemoveExt(g.File().Name())
			glippy.Set(myClip)
			message.Info("file Name copied(파일명 복사함): " + myClip)
		},
		//"o":  open file with default application
		"O": ifElse(runtime.GOOS == "windows", func() { g.Spawn(`explorer . %&`) }, ifElse(runtime.GOOS == "darwin", func() { g.Spawn(`open %D %&`) }, func() { g.Spawn(`xdg-open %D %&`) })), //open folder with file manager

		"p": //paste file 복사 파일 붙여넣기
		ifElse(runtime.GOOS == "windows", func() {
			value, _ := glippy.Get()
			value = strings.Replace(strings.Replace(strings.Replace(value, "\r\n", ` `, -1), `\`, `/`, -1), `"`, `'`, -1) //space in file name is not working without '
			g.Shell(`fcp /cmd=force_copy `+value+` /to='%~D/'`, -7)
		}, func() {
			value, _ := glippy.Get()
			if strings.Contains(value, "\n") {
				value = `'` + strings.Replace(value, "\n", `' '`, -1) + `'` //coped file name from finder has \n and doesn't have '. so add ' and replace \n to ' '
			}
			g.Shell(`cp -r -v `+value+` %D`, -7)
		}),

		"P": //move file 복사파일 이동함
		ifElse(runtime.GOOS == "windows", func() {
			value, _ := glippy.Get()
			value = strings.Replace(strings.Replace(strings.Replace(value, "\r\n", ` `, -1), `\`, `/`, -1), `"`, `'`, -1)
			g.Shell(`fcp /cmd=Move `+value+` /to='%~D/'`, -7)
		}, func() {
			value, _ := glippy.Get()
			if strings.Contains(value, "\n") {
				value = `'` + strings.Replace(value, "\n", `' '`, -1) + `'`
			}
			g.Shell(`mv -f -v `+value+` %D`, -7)

		}),

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

		"u": func() { g.Dir().MoveCursor(5) }, //hjkl ←↓↑→,    ui ↟↡,    ^,U = Home,    $, I = End
		"U": func() { g.Dir().MoveBottom() },  //hjkl ←↓↑→,    ui ↟↡,    ^,U = Home,    $, I = End

		//v: view menu
		//"V":

		"w":   func() { g.Workspace().ReloadAll(); g.Workspace().ChdirNeighbor2This() }, //change next window to this folder
		"W":   func() { g.Workspace().ReloadAll(); g.Workspace().ChdirNeighbor() },      // change this window to next folder
		"C-w": func() { g.Workspace().ReloadAll(); g.Workspace().CreateDir() },          //create new window
		"M-w": func() { g.Workspace().ReloadAll(); g.Workspace().CloseDir() },           //close window

		//"x": command menu
		//"X": external command menu

		"y": func() { //file copy 파일 복사
			myClip := strings.Join(g.Dir().MarkfileQuotedPaths(), " ")
			glippy.Set(myClip)
			message.Info("Yanked file(파일 복사함)): " + myClip)
		},
		"Y": func() { //Path copy 경로 복사
			myClip := util.RemoveExt(g.File().Path())
			glippy.Set(myClip)
			message.Info("path copied(경로 복사함): " + myClip)
		},

		"z": func() { g.Shell(`7z x '%~F' -o'%~D2/%~x'`) }, //extract zip file to neighbor folder
		"Z": func() { g.Shell(`7z x '%~F' -o'%~D/%~x'`) },  //extract zip file to current folder

		// function keys do External command
		"f2": ifElse(runtime.GOOS == "windows", func() { g.Shell("move %F './" + g.File().Name() + `'`) }, func() { g.Shell("mv -vi %f '" + g.File().Name() + `'`) }),
		"f5": ifElse(runtime.GOOS == "windows", func() { g.Shell(`fcp /cmd=force_copy %M /to='%~D2/'`, -7) }, func() { g.Shell(`cp -r -v %M %D2`, -7) }),
		"f6": ifElse(runtime.GOOS == "windows", func() { g.Shell(`fcp /cmd=move %M /to='%~D2/'`, -7) }, func() { g.Shell(`mv -f -v %M %D2`, -7) }),
		"f7": ifElse(runtime.GOOS == "windows", func() { g.Shell(`mkdir ` + `'` + util.RemoveExt(g.File().Name()) + `'`) }, func() { g.Shell(`mkdir -vp ` + `'` + util.RemoveExt(g.File().Name()) + `'`) }),
		"f8": ifElse(runtime.GOOS == "windows", func() { g.Shell(`recycle -s %M `, -7) }, ifElse(runtime.GOOS == "darwin", func() { g.Shell(`echo "Move file(s) to Trash? 휴지통으로 삭제? "; %| `, -7) }, func() { g.Shell(`mv %M ~/.local/share/Trash`, -7) })),
		"f9": ifElse(runtime.GOOS == "windows", func() { g.Shell(`mkdir ` + `'` + util.RemoveExt(g.File().Name()) + `'`) }, func() { g.Shell(`mkdir -vp ` + `'` + util.RemoveExt(g.File().Name()) + `'`) }),

		"delete": func() { g.Remove() }, //delete

		"'": func() { g.Dir().Reset(); g.Workspace().ReloadAll() }, //reset

		"~":  func() { g.Dir().Chdir("~") },
		"\\": func() { g.Dir().Chdir("/") },

		"backspace": func() { g.Dir().Chdir("..") },    //go to parent folder
		"left":      func() { g.Dir().Chdir("..") },    //hjkl ←↓↑→,    ui ↟↡,    ^,U = Home,    $, I = End
		"down":      func() { g.Dir().MoveCursor(1) },  //hjkl ←↓↑→,    ui ↟↡,    ^,U = Home,    $, I = End
		"up":        func() { g.Dir().MoveCursor(-1) }, //hjkl ←↓↑→,    ui ↟↡,    ^,U = Home,    $, I = End
		//"right": open file with default application

		"home": func() { g.Dir().MoveTop() },    //hjkl ←↓↑→,    ui ↟↡,    ^,U = Home,    $, I = End
		"end":  func() { g.Dir().MoveBottom() }, //hjkl ←↓↑→,    ui ↟↡,    ^,U = Home,    $, I = End
		"^":    func() { g.Dir().MoveTop() },    //hjkl ←↓↑→,    ui ↟↡,    ^,U = Home,    $, I = End
		"$":    func() { g.Dir().MoveBottom() }, //hjkl ←↓↑→,    ui ↟↡,    ^,U = Home,    $, I = End
		"pgdn": func() { g.Dir().PageDown() },   //hjkl ←↓↑→,    ui ↟↡,    ^,U = Home,    $, I = End
		"pgup": func() { g.Dir().PageUp() },     //hjkl ←↓↑→,    ui ↟↡,    ^,U = Home,    $, I = End

		" ": func() { g.Dir().ToggleMark() }, //space key
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
		// "C-n":       func() { w.History.CursorDown() },
		// "C-p":       func() { w.History.CursorUp() },
		"down": func() { w.History.CursorDown() },
		"up":   func() { w.History.CursorUp() },
		// "C-v":       func() { w.History.PageDown() },
		// "M-v":       func() { w.History.PageUp() },
		"pgdn": func() { w.History.PageDown() },
		"pgup": func() { w.History.PageUp() },
		// "M-<":       func() { w.History.MoveTop() },
		// "M->":       func() { w.History.MoveBottom() },
		"home": func() { w.History.MoveTop() },
		"end":  func() { w.History.MoveBottom() },
		// "M-n":       func() { w.History.Scroll(1) },
		// "M-p":       func() { w.History.Scroll(-1) },
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
		// "C-i":   func() { w.CursorToRight() }, //C-i = tab
		// "C-v":  func() { w.PageDown() },
		// "M-v":  func() { w.PageUp() },
		"pgdn": func() { w.PageDown() },
		"pgup": func() { w.PageUp() },
		// "M-<":  func() { w.MoveTop() },
		// "M->":  func() { w.MoveBottom() },
		"home": func() { w.MoveTop() },
		"end":  func() { w.MoveBottom() },
		// "M-n":  func() { w.Scroll(1) },
		// "M-p":  func() { w.Scroll(-1) },
		"C-i": func() { w.InsertCompletion() }, //C-i = tab
		"C-m": func() { w.InsertCompletion() },
		// "C-g":  func() { w.Exit() },
		"C-[": func() { w.Exit() },
	}
}

func menuKeymap(w *menu.Menu) widget.Keymap {
	return widget.Keymap{
		// "C-n":  func() { w.MoveCursor(1) },
		// "C-p":  func() { w.MoveCursor(-1) },
		"down": func() { w.MoveCursor(1) },
		"up":   func() { w.MoveCursor(-1) },
		// "C-v":  func() { w.PageDown() },
		// "M-v":  func() { w.PageUp() },
		// "M->":  func() { w.MoveBottom() },
		// "M-<":  func() { w.MoveTop() },
		"C-m": func() { w.Exec() }, //C-m = enter
		"C-g": func() { w.Exit() },
		"C-[": func() { w.Exit() }, //// C-[ means ESC //
	}
}
