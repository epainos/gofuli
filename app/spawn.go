package app

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/anmitsu/goful/message"
	"github.com/anmitsu/goful/util"
	"github.com/anmitsu/goful/widget"
	// "github.com/f1bonacc1/glippy"
)

// Spawn a process by the shell or the terminal.
func (g *Goful) Spawn(cmd string) {
	cmd, background := g.expandMacro(cmd)
	var args []string
	if background {
		args = g.shell(cmd)
	} else {
		args = g.terminal(cmd)
	}
	execCmd := exec.Command(args[0], args[1:]...)
	message.Info(strings.Join(execCmd.Args, " "))
	if err := spawn(execCmd); err != nil {
		message.Error(err)
	}
}

func spawn(cmd *exec.Cmd) error {
	var bufout bytes.Buffer
	cmd.Stdout = &bufout
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() {
		var errWait = cmd.Wait()
		var stderr = &exec.ExitError{}
		switch {
		case errors.As(errWait, &stderr):
			message.Errorf("%q: %s", cmd, stderr.Stderr)
			return
		case errWait != nil:
			message.Errorf("%q: %v", cmd, errWait)
			return
		}
		if bufout.Len() > 0 {
			message.Info(bufout.String())
		}
	}()
	return nil
}

// SpawnSuspend spawns a process and suspends screen.
func (g *Goful) SpawnSuspend(cmd string) {
	cmd, _ = g.expandMacro(cmd)
	args := g.shell(cmd)
	execCmd := exec.Command(args[0], args[1:]...)
	execCmd.Stdin = os.Stdin
	execCmd.Stdout = os.Stdout
	execCmd.Stderr = os.Stderr
	widget.Fini()
	defer func(cmd string) {
		widget.Init()
		message.Info(cmd)
	}(strings.Join(execCmd.Args, " "))
	_ = execCmd.Run()

	shell := exec.Command(args[0])
	shell.Stdin = os.Stdin
	shell.Stdout = os.Stdout
	shell.Stderr = os.Stderr
	_ = shell.Run()
}

const (
	macroPrefix             = '%'
	macroEscape             = '\\' // \ is an escape sequence
	macroNonQuote           = '~'  // %~ is expanded non quote
	macroFile               = 'f'  // %f %~f are expanded a file name on the cursor
	macroExtension          = 'e'  // %f %~f are expanded a file extension on the cursor
	macroFilePath           = 'F'  // %F %~F are expanded a file path on the cursor
	macroFileWithoutExt     = 'x'  // %x %~x are expanded a file name excluded the extension on the cursor
	macroFileWithoutExtPath = 'X'  // %x %~X are expanded a file path excluded the extension on the cursor
	macroMarkfile           = 'm'  // %m %~m are expanded mark file names joined by spaces
	macroMarkfilePath       = 'M'  // %M %~M are expanded mark file paths joined by spaces
	macroDir                = 'd'  // %d %~d are expanded a directory name on the cursor
	macroDirPath            = 'D'  // %D %~D are expanded a directory path on the cursor
	macroNextDir            = '2'  // %d2 %D2 %~d2 %~D2 are expanded the neighbor directory name or path
	macroTimestampDay       = 'T'  // 20241231
	macroTimestampHour      = 't'  // 154501
	macroRunBackground      = '&'  // %& is a flag runned in background
)

func (g *Goful) expandMacro(cmd string) (result string, background bool) {
	data := []byte(cmd)
	ret := make([]byte, len(data))
	copy(ret, data)

	background = false
	escape := false
	prefix := false
	nonQuote := false
	offset := 0
	for i, b := range data {
		if escape { // skip the escape sequence
			ret = widget.DeleteBytes(ret, offset-1, 1)
			escape = false
			continue
		}

		if prefix {
			if b == macroNonQuote {
				if nonQuote { // continuous ~ is not expand
					prefix = false
					nonQuote = false
					offset++
				} else {
					nonQuote = true
				}
				continue
			}
			prefix = false
			src := ""
			macrolen := 2
			if nonQuote {
				macrolen++
			}
			switch b {
			case macroFile:
				src = g.File().Name()
				// glippy.Set(src)
				if !nonQuote {
					src = util.Quote(src)
				}
			case macroExtension:
				src = g.File().Ext()
				if !nonQuote {
					src = util.Quote(src)
				}
			case macroFilePath:
				src = g.File().Path()
				if !nonQuote {
					src = util.Quote(src)
				}
			case macroFileWithoutExt:
				src = util.RemoveExt(g.File().Name())
				if !nonQuote {
					src = util.Quote(src)
				}
			case macroFileWithoutExtPath:
				src = util.RemoveExt(g.File().Path())
				if !nonQuote {
					src = util.Quote(src)
				}
			case macroMarkfile:
				if !nonQuote {
					src = strings.Join(g.Dir().MarkfileQuotedNames(), " ")
				} else {
					src = strings.Join(g.Dir().MarkfileNames(), " ")
				}
				// glippy.Set((src))
			case macroMarkfilePath:
				if !nonQuote {
					src = strings.Join(g.Dir().MarkfileQuotedPaths(), " ")
				} else {
					src = strings.Join(g.Dir().MarkfilePaths(), " ")
				}
				// glippy.Set((src))

			case macroDir:
				if i != len(data)-1 && data[i+1] == macroNextDir {
					src = g.Workspace().NextDir().Base()
					macrolen++
				} else {
					src = g.Dir().Base()
				}
				if !nonQuote {
					src = util.Quote(src)
				}
			case macroDirPath:
				if i != len(data)-1 && data[i+1] == macroNextDir {
					src = g.Workspace().NextDir().Path
					macrolen++
				} else {
					src = g.Dir().Path
				}
				if !nonQuote {
					src = util.Quote(src)
				}
			case macroTimestampHour:
				timestamp := time.Now().Format("150405")
				src = timestamp
			case macroTimestampDay:
				timestamp := time.Now().Format("241231")
				src = timestamp
			case macroRunBackground:
				background = true
			default:
				if nonQuote {
					nonQuote = false
					offset++
				}
				goto other
			}
			ret = widget.DeleteBytes(ret, offset-1, macrolen)
			ret = widget.InsertBytes(ret, []byte(src), offset-1)
			offset += len(src) - macrolen
			offset++
			if nonQuote {
				nonQuote = false
				offset++
			}
			continue
		}
	other:
		switch b {
		case macroPrefix:
			prefix = true
		case macroEscape:
			escape = true
		}
		offset++
	}
	return string(ret), background
}
