package filer

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	// "github.com/epainos/gofuli/cmdline"
	"github.com/epainos/gofuli/look"
	"github.com/epainos/gofuli/message"
	"github.com/epainos/gofuli/util"
	"github.com/epainos/gofuli/widget"
)

// Directory is a list box to store file stats.
type Directory struct {
	*widget.ListBox
	reader    reader
	history   map[string]string // key: path, value: file name on cursor
	finder    *Finder
	Path      string   `json:"path"`
	Sort      sortType `json:"sort_kind"`
	myHistory []string // 첫번째는 현재위치 인덱스(previous, forward로 왔다갔다 하는 이정표).  두번째부터는 이동했었던 주소

	// 경로 편집 모드 관련 필드
	EditingPath    bool
	PathEditText   string
	PathEditCursor int
}

// NewDirectory creates a new directory based on specified size and coordinates.
func NewDirectory(x, y, width, height int) *Directory {
	path, _ := filepath.Abs(".")
	listbox := widget.NewListBox(x, y, width, height, path)
	listbox.SetBorderStyle(borderStyle)
	return &Directory{
		ListBox:        listbox,
		reader:         defaultReader("."),
		history:        map[string]string{},
		Path:           path,
		Sort:           sortName,
		EditingPath:    false,
		PathEditText:   "",
		PathEditCursor: 0,
	}
}

// default border style
var borderStyle widget.BorderStyle = widget.ULBorder

// SetBorderStyle sets a directory default border style.
func SetBorderStyle(style widget.BorderStyle) {
	borderStyle = style
}

type sortType string

const (
	sortName     sortType = "Name[^]"
	sortNameRev  sortType = "Name[$]"
	sortSize     sortType = "Size[^]"
	sortSizeRev  sortType = "Size[$]"
	sortMtime    sortType = "Time[^]"
	sortMtimeRev sortType = "Time[$]"
	sortExt      sortType = "Ext[^]"
	sortExtRev   sortType = "Ext[$]"
)

var priorityDir = true

// TogglePriority toggles the priority for sorting files.
// The directory is prioritized in sorting if this is true.
func TogglePriority() {
	priorityDir = !priorityDir
}

var showHiddens = false

// ToggleShowHiddens toggles the showing of hidden files.
func ToggleShowHiddens() {
	showHiddens = !showHiddens
}

type reader interface {
	Read(callback func(name string))
	String() string
}

type defaultReader string

func (s defaultReader) String() string { return "" }
func (s defaultReader) Read(callback func(string)) {
	fd, err := os.Open(string(s))
	if err != nil {
		message.Error(err)
		return
	}
	defer fd.Close()

	// Readdirnames 대신 Readdir 사용
	fileInfos, err := fd.Readdir(-1)
	if err != nil {
		message.Error(err)
		return
	}

	for _, fi := range fileInfos {
		name := fi.Name()
		if !showHiddens && strings.HasPrefix(name, ".") {
			continue
		}
		// Normalize filename for proper display on macOS
		name = util.NormalizeFileName(name)
		callback(name)
	}
}

type globPattern string

func (s globPattern) String() string {
	return fmt.Sprintf("Glob(검색):(%s)", string(s))
}

func (s globPattern) Read(callback func(name string)) {
	matches, err := filepath.Glob(string(s))
	if err != nil {
		message.Error(err)
		return
	}
	for _, name := range matches {
		if !showHiddens && strings.HasPrefix(name, ".") {
			continue
		}
		// Normalize filename for proper display on macOS
		name = util.NormalizeFileName(name)
		callback(name)
	}
}

type globDirPattern string

func (s globDirPattern) String() string {
	return fmt.Sprintf("Globdir(하부검색):(%s)", string(s))
}

func (s globDirPattern) Read(callback func(string)) {
	_ = filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if path == "." {
			return nil
		}
		if ok, _ := filepath.Match(string(s), info.Name()); ok {
			if !showHiddens {
				if strings.HasPrefix(path, ".") || strings.HasPrefix(info.Name(), ".") {
					return nil
				}
			}
			// Normalize filename for proper display on macOS
			path = util.NormalizeFileName(path)
			callback(path)
		}
		return nil
	})
}

func (d *Directory) init4json() {
	d.ListBox = widget.NewListBox(0, 0, 0, 0, "")
	d.history = map[string]string{}
	d.SetTitle(util.AbbrPath(d.Path))
	d.SetColumn(1)
	d.reader = defaultReader(".")
}

// Resize the window and the finder.
func (d *Directory) Resize(x, y, width, height int) {
	d.ListBox.Resize(x, y, width, height)
	if d.finder != nil {
		d.finder.Resize(x, y+d.Height()-1, d.Width(), 1)
		d.ResizeRelative(0, 0, 0, -1)
	}
}

// Finder starts a finder in the directory for filtering files.
func (d *Directory) Finder() {
	x, y := d.LeftTop()
	d.finder = NewFinder(d, x, y+d.Height()-1, d.Width(), 1)
	d.ResizeRelative(0, 0, 0, -1)
}

// EnterDir changes the directory to a path on the cursor.
func (d *Directory) EnterDir() {
	if 0 >= len(d.myHistory) {
	} else {
		myIndex, _ := strconv.Atoi(d.myHistory[0])
		if len(d.myHistory)-1 > myIndex {
			d.myHistory = d.myHistory[:myIndex+1]
		}
	}
	d.Chdir(d.File().Name())
}

// Reset marking or reader.
func (d *Directory) Reset() {
	if d.IsMark() {
		d.MarkClear()
	} else if _, ok := d.reader.(defaultReader); !ok {
		name := d.File().Name()
		d.reader = defaultReader(".")
		d.read()
		d.SetCursorByName(name)
		d.SetOffsetCenteredCursor()
	}
}

// Chdir changes the current directory and reads a new path by the default reader.
// Sets the cursor to the history name or to the previous directory name if parentestirs.
func (d *Directory) Chdir(path string) {
	path = util.ExpandPath(path)
	path = filepath.Clean(path)

	if !filepath.IsAbs(path) {
		path, _ = filepath.Abs(filepath.Join(d.Path, path))
	}
	olddir := filepath.Base(d.Path)
	parent := filepath.Dir(d.Path)

	if d.finder != nil {
		d.finder.exitNotRead()
	}

	if err := os.Chdir(path); err != nil {
		message.Error(err)
		return
	}
	if !d.IsEmpty() {
		d.history[d.Path] = d.File().Name()
	}
	d.SetTitle(util.AbbrPath(path))
	d.Path = path
	d.reader = defaultReader(".")
	d.read()

	d.myHistory = AddHistory(d.myHistory, path)

	if name, ok := d.history[d.Path]; ok {
		d.SetCursorByName(name)
		d.SetOffsetCenteredCursor()
	} else if path == parent {
		d.SetCursorByName(olddir)
		d.SetOffsetCenteredCursor()
	} else {
		d.SetCursor(0)
	}

}
func AddHistory(myHistory []string, path string) []string {
	if len(myHistory) == 0 {
		myHistory = append(myHistory, "0")
	}
	if len(myHistory) >= 20 {
		myHistory = append(myHistory[:1], myHistory[2:]...)
	}
	myHistory = append(myHistory, path)

	myHistory[0] = strconv.Itoa(len(myHistory) - 1)
	return myHistory
}

// goPreviousFoler
func (d *Directory) GoPreviousFolder() {
	if len(d.myHistory) == 0 || d.myHistory[1] == "1" {
		return
	}
	myIndex, _ := strconv.Atoi(d.myHistory[0])
	if 1 >= myIndex {
		return
	} else {
		myIndex--
	}

	path := d.myHistory[myIndex]
	d.Chdir(path)
	d.myHistory = d.myHistory[:len(d.myHistory)-1]
	d.myHistory[0] = strconv.Itoa(myIndex)

}

// goForwardFoler
func (d *Directory) GoFowardFolder() {
	if len(d.myHistory) < 2 {
		return
	}
	myIndex, err := strconv.Atoi(d.myHistory[0])
	if err != nil {
		return
	}
	if myIndex >= len(d.myHistory)-1 {
		return
	}
	myIndex++

	path := d.myHistory[myIndex]
	d.Chdir(path)
	d.myHistory = d.myHistory[:len(d.myHistory)-1]
	d.myHistory[0] = strconv.Itoa(myIndex)
}

// Glob sets a reader to matching pattern in the current directory.
func (d *Directory) Glob(pattern string) {
	d.reader = globPattern(pattern)
	d.read()
}

// Globdir sets a reader to matching pattern in the directory includeing sub directories.
func (d *Directory) Globdir(pattern string) {
	d.reader = globDirPattern(pattern)
	d.read()
}

// func (d *Directory) read() {
// 	marked := make(map[string]bool, d.MarkCount())
// 	for _, e := range d.List() {
// 		if e.(*FileStat).IsMarked() {
// 			marked[e.(*FileStat).Path()] = true
// 		}
// 	}

// 	callback := func(name string) {
// 		if fs := NewFileStat(d.Path, name); fs != nil {
// 			d.AppendList(fs)
// 		}
// 	}
// 	if d.finder != nil {
// 		d.finder.find(callback)
// 	} else {
// 		d.ClearList()
// 		d.reader.Read(callback)
// 	}
// 	if d.IsEmpty() {
// 		d.AppendList(NewFileStat(d.Path, ".."))
// 	}
// 	sort.Sort(d)

// 	for _, e := range d.List() {
// 		if _, ok := marked[e.(*FileStat).Path()]; ok {
// 			e.(*FileStat).Mark()
// 		}
// 	}
// }

func (d *Directory) read() {
	marked := make(map[string]bool, d.MarkCount())
	for _, e := range d.List() {
		if e.(*FileStat).IsMarked() {
			marked[e.(*FileStat).Path()] = true
		}
	}

	// 파일 이름만 먼저 수집
	var names []string
	callback := func(name string) {
		names = append(names, name)
	}

	if d.finder != nil {
		// finder 로직은 이미 비동기적으로 처리될 수 있으므로, 여기서는 기본 reader에 집중합니다.
		// 실제 구현 시 finder와 통합을 고려해야 합니다.
		d.finder.find(callback)
	} else {
		d.ClearList()
		d.reader.Read(callback)
	}

	// --- 병렬 처리 시작 ---
	var wg sync.WaitGroup
	fsChan := make(chan *FileStat, len(names)) // 결과를 받을 버퍼 채널

	for _, name := range names {
		wg.Add(1)
		go func(n string) {
			defer wg.Done()
			// 각 파일 정보를 고루틴 내에서 비동기적으로 가져옴
			if fs := NewFileStat(d.Path, n); fs != nil {
				fsChan <- fs // 성공한 경우 채널로 결과 전송
			}
		}(name)
	}

	// 모든 고루틴이 끝날 때까지 기다렸다가 채널을 닫는 고루틴
	go func() {
		wg.Wait()
		close(fsChan)
	}()

	// 채널에서 결과를 받아 리스트에 추가
	for fs := range fsChan {
		d.AppendList(fs)
	}
	// --- 병렬 처리 종료 ---

	// 항상 상위폴더 '..'을 추가
	hasParent := false
	for _, e := range d.List() {
		if e.(*FileStat).Name() == ".." {
			hasParent = true
			break
		}
	}
	if !hasParent {
		d.AppendList(NewFileStat(d.Path, ".."))
	}
	sort.Sort(d)

	for _, e := range d.List() {
		if _, ok := marked[e.(*FileStat).Path()]; ok {
			e.(*FileStat).Mark()
		}
	}
}

func (d *Directory) reload() {
	if err := os.Chdir(d.Path); err != nil {
		message.Error(err)
		home, _ := os.UserHomeDir()
		d.Chdir(home)
		return
	}
	d.read()
}

// File returns a file on the cursor.
func (d *Directory) File() *FileStat {
	return d.CurrentContent().(*FileStat)
}

// Base returns the directory name.
func (d *Directory) Base() string { return filepath.Base(d.Path) }

func (d *Directory) sortBy(typ sortType) {
	d.Sort = typ
	name := d.File().Name()
	sort.Sort(d)
	d.SetCursorByName(name)
	d.SetOffsetCenteredCursor()
}

// SortName sorts files in ascending order by the file name.
func (d *Directory) SortName() { d.sortBy(sortName) }

// SortNameDec sorts files in descending order by the file name.
func (d *Directory) SortNameDec() { d.sortBy(sortNameRev) }

// SortMtime sorts files in ascending order by the modified time.
func (d *Directory) SortMtime() { d.sortBy(sortMtime) }

// SortMtimeDec sorts files in descending order by the modified time.
func (d *Directory) SortMtimeDec() { d.sortBy(sortMtimeRev) }

// SortSize sorts files in ascending order by the file size.
func (d *Directory) SortSize() { d.sortBy(sortSize) }

// SortSizeDec sorts files in descending order by the file size.
func (d *Directory) SortSizeDec() { d.sortBy(sortSizeRev) }

// SortExt sorts files in ascending order by the file extension.
func (d *Directory) SortExt() { d.sortBy(sortExt) }

// SortExtDec sorts files in descending order by the file extension.
func (d *Directory) SortExtDec() { d.sortBy(sortExtRev) }

// Less compares based on Sort.
func (d *Directory) Less(i, j int) bool {
	// ".." 폴더는 항상 맨 위에 배치
	if d.List()[i].(*FileStat).Name() == ".." {
		return true
	}
	if d.List()[j].(*FileStat).Name() == ".." {
		return false
	}

	if priorityDir {
		id := d.List()[i].(*FileStat).stat.IsDir()
		jd := d.List()[j].(*FileStat).stat.IsDir()
		if !(id && jd) && (id || jd) {
			return id
		}
	}
	switch d.Sort {
	case sortName:
		return d.List()[i].Name() < d.List()[j].Name()
	case sortNameRev:
		return d.List()[i].Name() > d.List()[j].Name()
	case sortMtime:
		return d.lessMtime(i, j)
	case sortMtimeRev:
		return d.lessMtime(j, i)
	case sortSize:
		return d.lessSize(i, j)
	case sortSizeRev:
		return d.lessSize(j, i)
	case sortExt:
		return d.lessExt(i, j)
	case sortExtRev:
		return d.lessExt(j, i)
	}
	return d.List()[i].Name() < d.List()[j].Name()
}

func (d *Directory) lessMtime(i, j int) bool {
	f1 := d.List()[i].(*FileStat)
	f2 := d.List()[j].(*FileStat)
	t1 := f1.ModTime().Unix()
	t2 := f2.ModTime().Unix()
	if t1 != t2 {
		return t1 < t2
	}
	return f1.Name() < f2.Name()
}

func (d *Directory) lessSize(i, j int) bool {
	f1 := d.List()[i].(*FileStat)
	f2 := d.List()[j].(*FileStat)
	s1 := f1.Size()
	s2 := f2.Size()
	if s1 != s2 {
		return s1 < s2
	}
	return f1.Name() < f2.Name()
}

func (d *Directory) lessExt(i, j int) bool {
	f1 := d.List()[i].(*FileStat)
	f2 := d.List()[j].(*FileStat)
	e1 := f1.Ext()
	e2 := f2.Ext()
	if e1 != e2 {
		return e1 < e2
	}
	return f1.Name() < f2.Name()
}

// IsMark reports whether even one file marked.
func (d *Directory) IsMark() bool {
	return d.MarkCount() != 0
}

// ToggleMark toggles the file mark on the cursor.
func (d *Directory) ToggleMark() {
	fs := d.CurrentContent().(*FileStat)
	if fs.Name() == ".." {
		d.MoveCursor(1)
	} else {
		fs.ToggleMark()
		d.MoveCursor(1)
	}
}

// InvertMark toggles all file marks.
func (d *Directory) InvertMark() {
	for _, e := range d.List() {
		if e.(*FileStat).Name() != ".." {
			e.(*FileStat).ToggleMark()
		}
	}
}

// MarkClear clears all file marks.
func (d *Directory) MarkClear() {
	for _, e := range d.List() {
		e.(*FileStat).Markoff()
	}
}

// MarkCount returns a number of marked files.
func (d *Directory) MarkCount() int {
	c := 0
	for _, e := range d.List() {
		if e.(*FileStat).IsMarked() {
			c++
		}
	}
	return c
}

// Markfiles returns marked file lists.
func (d *Directory) Markfiles() []*FileStat {
	if d.MarkCount() < 1 {
		return []*FileStat{d.File()}
	}
	markfiles := make([]*FileStat, 0, d.MarkCount())
	for _, e := range d.List() {
		if e.(*FileStat).IsMarked() {
			markfiles = append(markfiles, e.(*FileStat))
		}
	}
	return markfiles
}

// MarkfileNames returns marked file names.
func (d *Directory) MarkfileNames() []string {
	if d.MarkCount() < 1 {
		return []string{d.File().Name()}
	}
	markfiles := make([]string, 0, d.MarkCount())
	for _, e := range d.List() {
		if e.(*FileStat).IsMarked() {
			markfiles = append(markfiles, e.(*FileStat).Name())
		}
	}
	return markfiles
}

// MarkfilePaths returns marked file paths.
func (d *Directory) MarkfilePaths() []string {
	if d.MarkCount() < 1 {
		return []string{d.File().Path()}
	}
	markfiles := make([]string, 0, d.MarkCount())
	for _, e := range d.List() {
		if e.(*FileStat).IsMarked() {
			markfiles = append(markfiles, e.(*FileStat).Path())
		}
	}
	return markfiles
}

// MarkfileQuotedNames returns quoted file names for marked.
func (d *Directory) MarkfileQuotedNames() []string {
	if d.MarkCount() < 1 {
		return []string{util.Quote(d.File().Name())}
	}
	markfiles := make([]string, 0, d.MarkCount())
	for _, e := range d.List() {
		if e.(*FileStat).IsMarked() {
			markfiles = append(markfiles, util.Quote(e.(*FileStat).Name()))
		}
	}
	return markfiles
}

// MarkfileQuotedPaths returns quoted file paths for marked.
func (d *Directory) MarkfileQuotedPaths() []string {
	if d.MarkCount() < 1 {
		return []string{util.Quote(d.File().Path())}
	}
	markfiles := make([]string, 0, d.MarkCount())
	for _, e := range d.List() {
		if e.(*FileStat).IsMarked() {
			markfiles = append(markfiles, util.Quote(e.(*FileStat).Path()))
		}
	}
	return markfiles
}

// MarkfileDoubleQuotedPaths returns double-quoted file paths for marked.
func (d *Directory) MarkfileDoubleQuotedPaths() []string {
	if d.MarkCount() < 1 {
		return []string{fmt.Sprintf(`"%s"`, d.File().Path())}
	}
	markfiles := make([]string, 0, d.MarkCount())
	for _, e := range d.List() {
		if e.(*FileStat).IsMarked() {
			markfiles = append(markfiles, fmt.Sprintf(`"%s"`, e.(*FileStat).Path()))
		}
	}
	return markfiles
}

// MarkfileSingleQuotedPaths returns single-quoted file paths for marked.
func (d *Directory) MarkfileSingleQuotedPaths() []string {
	if d.MarkCount() < 1 {
		return []string{fmt.Sprintf(`'%s'`, d.File().Path())}
	}
	markfiles := make([]string, 0, d.MarkCount())
	for _, e := range d.List() {
		if e.(*FileStat).IsMarked() {
			markfiles = append(markfiles, fmt.Sprintf(`'%s'`, e.(*FileStat).Path()))
		}
	}
	return markfiles
}

// getSortDisplay returns a human-readable sort description
func (d *Directory) getSortDisplay() string {
	switch d.Sort {
	case sortName:
		return "sort [name↗]"
	case sortNameRev:
		return "sort [name↘]"
	case sortSize:
		return "sort [size↗]"
	case sortSizeRev:
		return "sort [size↘]"
	case sortMtime:
		return "sort [time↗]"
	case sortMtimeRev:
		return "sort [time↘]"
	case sortExt:
		return "sort [ext↗]"
	case sortExtRev:
		return "sort [ext↘]"
	default:
		return "sort [name↗]"
	}
}

func (d *Directory) drawFooter() {
	// 현재 위치 (0부터 시작, ..폴더가 0번)
	currentPos := d.Cursor()

	// 전체 파일 개수 계산 (..폴더 제외)
	totalFiles := len(d.List()) - 1 // ..폴더를 제외한 실제 파일 개수

	// 폴더 크기 계산
	var folderSize int64
	for _, file := range d.List() {
		if fs, ok := file.(*FileStat); ok {
			folderSize += fs.Size()
		}
	}

	// 선택된 파일 개수만 계산
	selectedCount := d.MarkCount()

	// 자릿수에 맞는 포맷팅 결정 (앞에 _로 자릿수 맞춤)
	var format string
	if currentPos < 10 {
		if totalFiles < 10 {
			format = "[__%d/__%d]"
		} else if totalFiles < 100 {
			format = "[__%d/_%d]"
		} else if totalFiles < 1000 {
			format = "[__%d/%d]"
		} else {
			format = "[__%d/%d]"
		}
	} else if currentPos < 100 {
		if totalFiles < 10 {
			format = "[_%d/__%d]"
		} else if totalFiles < 100 {
			format = "[_%d/_%d]"
		} else if totalFiles < 1000 {
			format = "[_%d/%d]"
		} else {
			format = "[_%d/%d]"
		}
	} else if currentPos < 1000 {
		if totalFiles < 10 {
			format = "[%d/__%d]"
		} else if totalFiles < 100 {
			format = "[%d/_%d]"
		} else if totalFiles < 1000 {
			format = "[%d/%d]"
		} else {
			format = "[%d/%d]"
		}
	} else {
		if totalFiles < 10 {
			format = "[%d/__%d]"
		} else if totalFiles < 100 {
			format = "[%d/_%d]"
		} else if totalFiles < 1000 {
			format = "[%d/%d]"
		} else {
			format = "[%d/%d]"
		}
	}

	// 기본 정보: [현재위치/전체파일개수] 폴더크기 (앞에 공백 추가)
	s := " " + fmt.Sprintf(format, currentPos, totalFiles) + " " + util.FormatSize(folderSize)

	// 정렬 방식 표시 추가
	s += " " + d.getSortDisplay()

	// 선택된 파일이 있으면 개수만 표시 (용량 제거)
	if selectedCount > 0 {
		var fileFormat string
		if selectedCount < 10 {
			fileFormat = " __%dfiles"
		} else if selectedCount < 100 {
			fileFormat = " _%dfiles"
		} else if selectedCount < 1000 {
			fileFormat = " %dfiles"
		} else {
			fileFormat = " %dfiles"
		}
		s += fmt.Sprintf(fileFormat, selectedCount)
	}

	// 하단부 정보 그리기 (기존 UI 요소는 유지)
	x, y := d.LeftBottom()

	// 오른쪽 pane에서 위치가 잘못 계산될 수 있으므로 더 정확한 위치 계산
	if x < 0 || y < 0 {
		// 기본값으로 설정
		x, y = d.LeftTop()
		y += d.Height() - 1
	}

	// 하단줄을 통째로 다시 그리기 (변동사항이 있을 때마다)
	width := d.Width()

	// 먼저 하단줄을 공백으로 지우기
	for i := 0; i < width; i++ {
		widget.SetCells(x+i, y, " ", look.Default())
	}

	// 새로운 내용 그리기
	widget.SetCells(x, y, s, look.Default())

	// 구분선 다시 그리기 (전체 하단줄)
	for i := 0; i < width; i++ {
		widget.SetCells(x+i, y, "-", look.Default())
	}

	// 하단부 정보를 구분선 위에 다시 그리기
	widget.SetCells(x, y, s, look.Default())
}

func (d *Directory) drawFiles(focus bool) {
	height := d.Height() - 2
	row := 1
	shift := 0
	width := d.Width() - 1
	if d.BorderStyle() == widget.AllBorder {
		shift++
		width--
	}
	for i := d.Offset(); i < d.Upper(); i++ {
		if row > height {
			break
		}
		x, y := d.LeftTop()
		y += row
		x += shift
		if focus && i == d.Cursor() {
			d.List()[i].Draw(x, y, width, true)
		} else {
			d.List()[i].Draw(x, y, width, false)
		}
		row++
	}
}

func (d *Directory) draw(focus bool) {
	d.AdjustCursor()
	d.AdjustOffset()
	d.Border()
	d.drawFiles(focus)
	d.drawFooter()
	if d.finder != nil {
		d.finder.Draw(focus)
	}
}

// Directory 입력 처리
func (d *Directory) Input(key string) {
	if d.EditingPath {
		// 경로 편집 모드 처리
		switch key {
		case "enter", "C-m":
			// 경로 변경 실행
			if d.PathEditText != "" {
				path := util.ExpandPath(d.PathEditText)
				d.Chdir(path)
			}
			d.EditingPath = false
			d.PathEditText = ""
			d.PathEditCursor = 0
			return
		case "esc", "C-[", "C-g":
			// 편집 모드 취소
			d.EditingPath = false
			d.PathEditText = ""
			d.PathEditCursor = 0
			return
		case "backspace", "C-h":
			// message.Info("백스페이스 입력됨: " + key)
			if d.PathEditCursor > 0 {
				d.PathEditText = d.PathEditText[:d.PathEditCursor-1] + d.PathEditText[d.PathEditCursor:]
				d.PathEditCursor--
			}
			return
		case "delete":
			if d.PathEditCursor < len(d.PathEditText) {
				d.PathEditText = d.PathEditText[:d.PathEditCursor] + d.PathEditText[d.PathEditCursor+1:]
			}
			return
		case "left":
			if d.PathEditCursor > 0 {
				d.PathEditCursor--
			}
			return
		case "right":
			if d.PathEditCursor < len(d.PathEditText) {
				d.PathEditCursor++
			}
			return
		case "home":
			d.PathEditCursor = 0
			return
		case "end":
			d.PathEditCursor = len(d.PathEditText)
			return
		}

		// 일반 문자 입력
		if len(key) == 1 {
			d.PathEditText = d.PathEditText[:d.PathEditCursor] + key + d.PathEditText[d.PathEditCursor:]
			d.PathEditCursor++
		}
		return
	}

	switch key {
	case "up":
		d.CursorUp()
	case "down":
		d.CursorDown()
	case "left":
		d.CursorToLeft()
	case "right":
		d.CursorToRight()
	case "home":
		d.MoveTop()
	case "end":
		d.MoveBottom()
	case "pgup":
		d.PageUp()
	case "pgdn":
		d.PageDown()
	case "enter":
		d.EnterDir()
	case "backspace":
		d.GoPreviousFolder()
	case "C-f":
		d.GoFowardFolder()
	case "space":
		d.ToggleMark()
	case "C-a":
		d.InvertMark()
	case "C-c":
		d.MarkClear()
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func TildePath(path string) string {
	home, err := os.UserHomeDir()
	if err == nil && (path == home || strings.HasPrefix(path, home+"/")) {
		return "~" + path[len(home):]
	}
	return path
}

func expandTilde(path string) string {
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[1:])
		}
	}
	return path
}

// Patch: ListBox에 실제 선택가능한 후보 개수를 저장
type SelectableListBox interface {
	SetSelectableCount(int)
	SelectableCount() int
}

type listBoxWithSelectable struct {
	*widget.ListBox
	selectableCount int
}

func (l *listBoxWithSelectable) SetSelectableCount(n int) { l.selectableCount = n }
func (l *listBoxWithSelectable) SelectableCount() int     { return l.selectableCount }
