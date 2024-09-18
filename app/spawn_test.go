package app

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// ifElse 함수 정의
func ifElse(condition bool, trueVal string, falseVal string) string {
	if condition {
		return trueVal
	}
	return falseVal
}

func TestExpandMacro(t *testing.T) {
	g := NewGoful("")
	g.Workspace().ReloadAll() // in home directory

	home, _ := os.UserHomeDir()
	macros := []struct {
		in  string
		out string
	}{
		{`%f`, `'..'`},
		{`%F`, fmt.Sprintf(`'%s'`, ifElse(runtime.GOOS == "windows", strings.Replace(strings.Replace(filepath.Dir(home), `\`, `/`, -1), `"`, `'`, -1), filepath.Dir(home)))},
		{`%x`, `'.'`},
		{`%X`, fmt.Sprintf(`'%s'`, ifElse(runtime.GOOS == "windows", strings.Replace(strings.Replace(filepath.Dir(home), `\`, `/`, -1), `"`, `'`, -1), filepath.Dir(home)))},
		{`%m`, `'..'`},
		{`%M`, fmt.Sprintf(`'%s'`, ifElse(runtime.GOOS == "windows", strings.Replace(strings.Replace(filepath.Dir(home), `\`, `/`, -1), `"`, `'`, -1), filepath.Dir(home)))},
		{`%d`, fmt.Sprintf(`'%s'`, ifElse(runtime.GOOS == "windows", strings.Replace(strings.Replace(filepath.Dir(home), `\`, `/`, -1), `"`, `'`, -1), filepath.Base(home)))},
		{`%D`, fmt.Sprintf(`'%s'`, ifElse(runtime.GOOS == "windows", strings.Replace(strings.Replace(home, `\`, `/`, -1), `"`, `'`, -1), home))},
		{`%d2`, fmt.Sprintf(`'%s'`, ifElse(runtime.GOOS == "windows", strings.Replace(strings.Replace(filepath.Dir(home), `\`, `/`, -1), `"`, `'`, -1), filepath.Base(home)))},
		{`%D2`, fmt.Sprintf(`'%s'`, ifElse(runtime.GOOS == "windows", strings.Replace(strings.Replace(home, `\`, `/`, -1), `"`, `'`, -1), home))},
		{`%~f`, `..`},
		{`%~F`, ifElse(runtime.GOOS == "windows", strings.Replace(strings.Replace(filepath.Dir(home), `\`, `/`, -1), `"`, `'`, -1), filepath.Dir(home))},
		{`%~x`, `.`},
		{`%~X`, ifElse(runtime.GOOS == "windows", strings.Replace(strings.Replace(filepath.Dir(home), `\`, `/`, -1), `"`, `'`, -1), filepath.Dir(home))},
		{`%~m`, ".."},
		{`%~M`, ifElse(runtime.GOOS == "windows", strings.Replace(strings.Replace(filepath.Dir(home), `\`, `/`, -1), `"`, `'`, -1), filepath.Dir(home))},
		{`%~d`, ifElse(runtime.GOOS == "windows", strings.Replace(strings.Replace(filepath.Dir(home), `\`, `/`, -1), `"`, `'`, -1), filepath.Base(home))},
		{`%~D`, ifElse(runtime.GOOS == "windows", strings.Replace(strings.Replace(home, `\`, `/`, -1), `"`, `'`, -1), home)},
		{`%~d2`, ifElse(runtime.GOOS == "windows", strings.Replace(strings.Replace(filepath.Dir(home), `\`, `/`, -1), `"`, `'`, -1), filepath.Base(home))},
		{`%~D2`, ifElse(runtime.GOOS == "windows", strings.Replace(strings.Replace(home, `\`, `/`, -1), `"`, `'`, -1), home)},
		{`%%%f`, `%%'..'`},
		{`%%%~f`, `%%..`},
		{`%~~f`, `%~~f`},
		{`\%f%f`, `%f".."`},
		{`\%~f%~f`, `%~f..`},
		{`%\f%f`, `%f'..'`},
		{`%\~f%~f`, `%~f..`},
		{"%AA%ff", `%AA'..'f`},
		{"%~A~A%~ff", `%~A~A..f`},
		{"%m %f", `'..' '..'`},
		{"%~f %f %~m", `.. '..' ..`},
	}

	for _, macro := range macros {
		ret, _ := g.expandMacro(macro.in)
		if ret != macro.out {
			t.Errorf("%s -> %s result %s\n", macro.in, macro.out, ret)
		}
	}
}
