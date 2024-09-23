// Package app is goful application components.
package app

import (
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
