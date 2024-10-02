// Package menu provides the menu window widget.
package menu

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/epainos/gofuli/message"
	"github.com/epainos/gofuli/util"
	"github.com/epainos/gofuli/widget"
	"github.com/f1bonacc1/glippy"
)

var menusMap = map[string][]*menuItem{}

// Add menu items as label, acceleration key and callback function and
// the number of arguments `a' must be a multiple of three.
func Add(name string, a ...interface{}) {
	if len(a)%3 != 0 {
		panic("items must be a multiple of three")
	}
	items := menusMap[name]
	for i := 0; i < len(a); i += 3 {
		accel := a[i].(string)
		label := a[i+1].(string)
		callback := a[i+2].(func())
		items = append(items, &menuItem{accel, label, callback})
	}
	menusMap[name] = items
}

// Remove a menu item by its acceleration key
func Remove(name string, accel string) {
	items := menusMap[name]
	for i, item := range items {
		if item.accel == accel {
			// print(i)
			// Remove the item by copying the slice without the element at index i
			if i == len(items)-1 {
				items = items[:i]
			} else {
				items = append(items[:i], items[i+1:]...)
			}
			break
		}
	}
	menusMap[name] = items
}
func (w *Menu) RemoveMenuInWindow() {
	title := w.Title()
	accel := menusMap[w.Title()][w.Cursor()].accel
	nameToDel := menusMap[w.Title()][w.Cursor()].label
	glippy.Set("menu:" + title + ":" + accel + ":" + nameToDel)

	if !(title == "myApp" || title == "myBookmark") {
		message.Errorf("Default menu cannot be removed... 기본 메뉴는 삭제할 수 없어요.")
		return
	} else if accel == "+" || accel == "-" {
		message.Errorf("'add', 'del' cannot be removed... 추가,삭제는 삭제할 수 없어요.")
	} else {
		// glippy.Set("menu:" + w.Title() + ":" + accel)
		DelMyAppFromListFile("~/.goful/"+title, accel)
		message.Info("삭제 완료: " + nameToDel)

		Remove(title, accel)
		w.Exit()
	}
}

var keymap func(*Menu) widget.Keymap

// Config the keymap function for a menu.
func Config(config func(*Menu) widget.Keymap) {
	keymap = config
}

type menuItem struct {
	accel    string
	label    string
	callback func()
}

// Menu is a list box to execute for a acceleration key.
type Menu struct {
	*widget.ListBox
	filer widget.Widget
}

// New creates a new menu based on filer widget sizes.
func New(name string, filer widget.Widget) (*Menu, error) {
	items, ok := menusMap[name]
	if !ok {
		return nil, fmt.Errorf("not found menu `%s'", name)
	}
	x, y := filer.LeftBottom()
	width := filer.Width()
	height := len(items) + 2
	if max := filer.Height() / 2; height > max {
		height = max
	}
	menu := &Menu{
		ListBox: widget.NewListBox(x, y-height+1, width, height, name),
		filer:   filer,
	}
	for _, item := range items {
		s := fmt.Sprintf("%-3s %s", item.accel, item.label)
		menu.AppendString(s)
	}
	return menu, nil
}

// Resize the menu window.
func (w *Menu) Resize(x, y, width, height int) {
	h := len(menusMap[w.Title()]) + 2
	if max := height / 2; h > max {
		h = max
	}
	w.ListBox.Resize(x, height-h, width, h)
}

// Exec executes a menu item on the cursor and exits the menu.
func (w *Menu) Exec() {
	w.Exit()
	menusMap[w.Title()][w.Cursor()].callback()
	// glippy.Set("menu:" + w.Title() + ":" + menusMap[w.Title()][w.Cursor()].accel)
}

// Input to the list box or execute a menu item with the acceleration key.
func (w *Menu) Input(key string) {
	keymap := keymap(w)
	if callback, ok := keymap[key]; ok {
		callback()
	} else {
		for _, item := range menusMap[w.Title()] {
			if item.accel == key {
				w.Exit()
				item.callback()
			}
		}
	}
}

// Exit the menu mode.
func (w *Menu) Exit() { w.filer.Disconnect() }

// Next implements widget.Widget.
func (w *Menu) Next() widget.Widget { return widget.Nil() }

// Disconnect implements widget.Widget.
func (w *Menu) Disconnect() {}

// DelMyAppFromListFile is a function to delete a shortcut from the myApp file.
func DelMyAppFromListFile(path string, shortcutToDel string) {
	// if path == "" {
	// 	path = "~/.goful/myApp"
	// }
	file, err := os.OpenFile(util.ExpandPath(path), os.O_RDWR, 0644)
	if err != nil {
		fmt.Println("파일 열기 실패:", err)
		return
	}
	defer file.Close()

	fileInfo, err := os.Stat(util.ExpandPath(path))
	if err != nil {
		fmt.Println("파일 정보 가져오기 실패:", err)
		return
	}

	if fileInfo.Size() == 0 {
		fmt.Println("파일이 비어있습니다.")
		return
	}

	var temp []byte
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, shortcutToDel) {
			temp = append(temp, line...)
			temp = append(temp, '\n')
		}
	}

	if scanner.Err() != nil {
		fmt.Println("파일 읽기 오류:", scanner.Err())
		return
	}

	// 파일 크기 조절
	if err := file.Truncate(0); err != nil {
		fmt.Println("파일 크기 조절 실패:", err)
		return
	}

	// 파일 처음부터 다시 쓰기
	_, err = file.WriteAt(temp, 0)
	if err != nil {
		fmt.Println("파일 쓰기 실패:", err)
		return
	}

	fmt.Println("삭제 완료")

}
