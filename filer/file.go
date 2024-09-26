package filer

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/epainos/gofuli/look"
	"github.com/epainos/gofuli/message"
	"github.com/epainos/gofuli/util"
	"github.com/epainos/gofuli/widget"
	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
)

var statView = fileStatView{true, true, true}

type fileStatView struct {
	size       bool
	permission bool
	time       bool
}

// SetStatView sets the file state view.
func SetStatView(size, permission, time bool) { statView = fileStatView{size, permission, time} }

// ToggleSizeView toggles the file size view.
func ToggleSizeView() { statView.size = !statView.size }

// TogglePermView toggles the file permission view.
func TogglePermView() { statView.permission = !statView.permission }

// ToggleTimeView toggles the file time view.
func ToggleTimeView() { statView.time = !statView.time }

var timeFormat = "06-01-02 15:04"

// SetTimeFormat sets the time format of files.
func SetTimeFormat(format string) {
	timeFormat = format
}

// FileStat is file information.
type FileStat struct {
	os.FileInfo             // os.Lstat(path)
	stat        os.FileInfo // os.Stat(path)
	path        string      // full path of file
	name        string      // base name of path or ".." as upper directory
	display     string      // display name for draw
	marked      bool        // marked whether
	myColor     tcell.Style
}

// ifElse 함수 정의
func ifElse(condition bool, trueVal, falseVal tcell.Style) tcell.Style {
	if condition {
		return trueVal
	}
	return falseVal
}

// 확장자 확인
func hasExtension(text string, extensions []string) bool {
	// 정규 표현식 생성
	regexStr := "^.*\\.(" + strings.Join(extensions, "|") + ")$"
	re := regexp.MustCompile(regexStr)

	// 정규 표현식 매칭
	return re.MatchString(text)
}

// NewFileStat creates a new file stat of the file in the directory.
func NewFileStat(dir string, name string) *FileStat {
	path := filepath.Join(dir, name)

	lstat, err := os.Lstat(path)
	if err != nil {
		message.Error(err)
		return nil
	}
	stat, err := os.Stat(path)
	if err != nil {
		stat = lstat
	}

	var display string
	d := tcell.StyleDefault
	myColor := d.Foreground(tcell.ColorGray)
	if stat.IsDir() {
		display = "📂 " + name //📁
	} else {
		display = util.RemoveExt(name)
		ext := filepath.Ext(name)
		//💾📙📘⚛⛯☢🧲🐬⚒🄰⚙⛭🛠🔧🧭🛜🛡🖨🕸🌐📏🎨🎧🎬🎮🎴💳🗂🗃🪧▶🦥🚯🍥⛔🐴✉📩🕹🗒🗓📄🏠⛪♿☕☀🌞🌅🌄🎴🏡🏘️🏗️🏢🏛⛏🪛🪪🔆🪙⏹⏹️🪟🆒🌞☀️⛱🌬🌬️🧰🖥💻⚓🔍🔎🔥🔨🔩

		if stat.Mode().Perm()&0111 != 0 || hasExtension(ext, []string{"exe", "com", "bat", "sh", "app"}) { //exec file is treated one more metoth
			display = "🌞 " + display                             //⏹
			myColor = d.Foreground(tcell.ColorYellow).Bold(true) //  ifElse(runtime.GOOS == "windows", d.Foreground(tcell.ColorYellow).Bold(true), d.Foreground(tcell.ColorSkyblue).Background((tcell.ColorDarkSlateGray)).Bold(true))
		} else if hasExtension(ext, []string{"doc", "docx", "ppt", "pptx", "xls", "xlsx", "hwp", "hwpx"}) { //오피스파일
			display = "📘 " + display
			myColor = d.Foreground(tcell.ColorSkyblue) //.Background((tcell.ColorGreen))
		} else if hasExtension(ext, []string{"txt", "rtf", "me", "md", "csv"}) { //오피스파일
			display = "📜 " + display
			myColor = d.Foreground(tcell.ColorOlive) //.Background((tcell.ColorGreen))
		} else if hasExtension(ext, []string{"pdf", "json"}) { //pdf파일
			display = "📙 " + display
			myColor = d.Foreground(tcell.ColorCadetBlue) //.Background((tcell.ColorGreen))
		} else if hasExtension(ext, []string{"jpg", "png", "jpeg", "gif", "bmp", "psd"}) { //이미지 파일
			display = "🎨 " + display
			myColor = d.Foreground(tcell.ColorGreenYellow) //.Background((tcell.ColorGreen))
		} else if hasExtension(ext, []string{"mp4", "mov", "avi", "mkv"}) { //영상 파일
			display = "🎬 " + display                       //🎬🎦🎥📽🎞
			myColor = d.Foreground(tcell.ColorYellowGreen) //.Background((tcell.ColorGreen))
		} else if hasExtension(ext, []string{"html", "htm", "css", "cshtml", "xml"}) { //인터넷 파일
			display = "🌐 " + display
			myColor = d.Foreground(tcell.ColorDodgerBlue) //.Background((tcell.ColorGreen))
		} else if hasExtension(ext, []string{"zip", "gz", "tar", "tgz", "bx2", "xz", "txz", "rar"}) { //압축파일
			display = "📦 " + display //📥📦
			myColor = d.Foreground(tcell.ColorBurlyWood)
		} else if hasExtension(ext, []string{"msi", "deb", "rpm"}) { //압축파일
			display = "🧰 " + display //📥📦
			myColor = d.Foreground(tcell.ColorOrange)
		} else if hasExtension(ext, []string{"iso", "dmg"}) { //이미지 파일
			display = "💿 " + display                //💽💿
			myColor = d.Foreground(tcell.ColorPeru) //.Background((tcell.ColorGreen))
		} else if hasExtension(ext, []string{"dwg", "dxg", "dgn", "svg", "esp"}) { //캐드파일
			display = "📐 " + display
			myColor = d.Foreground(tcell.ColorSteelBlue) //.Background((tcell.ColorGreen))
		} else if hasExtension(ext, []string{"cfg", "yaml", "plist", "properties"}) { //설정파일
			display = "💻 " + display
			myColor = d.Foreground(tcell.ColorKhaki) //.Background((tcell.ColorGreen))
		} else if hasExtension(ext, []string{"py", "c", "cs", "go", "swift", "js", "jave", "dart"}) { //코드파일
			display = "📙 " + display
			myColor = d.Foreground(tcell.ColorDarkOrange) //.Background((tcell.ColorGreen))
		} else {
			display = "📄 " + display
		}

	}

	return &FileStat{
		FileInfo: lstat,
		stat:     stat,
		path:     path,
		name:     name,
		display:  display,
		marked:   false,
		myColor:  myColor,
	}
}

// Name returns the file name.
func (f *FileStat) Name() string {
	return f.name
}

// SetDisplay sets the display name for drawing.
func (f *FileStat) SetDisplay(name string) {
	f.display = name
}

// ResetDisplay resets the display name to the file name.
func (f *FileStat) ResetDisplay() {
	if f.stat.IsDir() {
		f.display = f.name
	} else {
		f.display = util.RemoveExt(f.name)
	}
}

// Mark the file.
func (f *FileStat) Mark() {
	f.marked = true
}

// Markoff the file.
func (f *FileStat) Markoff() {
	f.marked = false
}

// ToggleMark toggles the file mark.
func (f *FileStat) ToggleMark() {
	f.marked = !f.marked
}

// Path returns the file path.
func (f *FileStat) Path() string {
	f.path = strings.Replace(f.path, `\`, `/`, -1)
	return f.path
}

// Ext retruns the file extension.
func (f *FileStat) Ext() string {
	if f.stat.IsDir() {
		return ""
	}
	if ext := filepath.Ext(f.Name()); ext != f.Name() {
		return ext
	}
	return ""
}

// ext is zip?
func (f *FileStat) IsColorful() bool {
	return true
}

// IsLink reports whether the symlink.
func (f *FileStat) IsLink() bool {
	return f.Mode()&os.ModeSymlink != 0
}

func (f *FileStat) IsDir4osx() bool {
	if runtime.GOOS == "darwin" && f.stat.IsDir() && strings.HasSuffix(f.name, ".app") {
		return false
	}

	return f.IsDir()
}

// IsExec reports whether the executable file.
func (f *FileStat) IsExec() bool {
	// if runtime.GOOS == "darwin" && f.stat.IsDir() && strings.HasSuffix(f.name, ".app") {
	// 	return false
	// }
	return f.stat.Mode().Perm()&0111 != 0
}

// IsFIFO reports whether the named pipe file.
func (f *FileStat) IsFIFO() bool {
	return f.stat.Mode()&os.ModeNamedPipe != 0
}

// IsDevice reports whether the device file.
func (f *FileStat) IsDevice() bool {
	return f.stat.Mode()&os.ModeDevice != 0
}

// IsCharDevice reports whether the character device file.
func (f *FileStat) IsCharDevice() bool {
	return f.stat.Mode()&os.ModeCharDevice != 0
}

// IsSocket reports whether the socket file.
func (f *FileStat) IsSocket() bool {
	return f.stat.Mode()&os.ModeSocket != 0
}

// IsMarked reports whether the marked file.
func (f *FileStat) IsMarked() bool {
	return f.marked
}

func (f *FileStat) suffix() string {
	if f.IsLink() {
		link, _ := os.Readlink(f.Path())
		if f.stat.IsDir() {
			return "@ -> " + link + "/"
		}
		return "@ -> " + link
	} else if f.IsDir() {
		return "/"
	} else if f.IsFIFO() {
		return "|"
	} else if f.IsSocket() {
		return "="
	} else if f.IsExec() {
		return "*"
	}
	return ""
}

func (f *FileStat) states() string {
	ret := f.Ext()
	if statView.size {
		if f.stat.IsDir() {
			ret += fmt.Sprintf("%8s", "<DIR>")
		} else {
			ret += fmt.Sprintf("%8s", util.FormatSize(f.stat.Size()))
		}
	}
	if statView.permission {
		ret += " " + f.stat.Mode().String()
	}
	if statView.time {
		ret += " " + f.stat.ModTime().Format(timeFormat)
	}
	return ret
}

func (f *FileStat) look() tcell.Style {
	switch {
	case f.IsMarked():
		return look.Marked()
	case f.IsLink():
		if f.stat.IsDir() {
			return look.SymlinkDir()
		}
		return look.Symlink()
	case f.IsDir():
		return look.Directory()
	// case f.IsExec():
	// 	return look.Executable()
	case f.IsColorful():
		return look.SetMyColor(f.myColor)
	default:
		return look.Default()
	}
}

// Draw the file name and file stats.
func (f *FileStat) Draw(x, y, width int, focus bool) {
	style := f.look()
	if focus {
		style = style.Reverse(true)
	}
	states := f.states()
	width -= len(states)
	pre := " "
	if f.marked {
		pre = "*"
	}
	s := pre + f.display + f.suffix()
	s = runewidth.Truncate(s, width, "~")
	s = runewidth.FillRight(s, width)
	x = widget.SetCells(x, y, s, style)
	widget.SetCells(x, y, states, style)
}
