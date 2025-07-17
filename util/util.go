// Package util provides misc utility functions.
package util

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/mattn/go-runewidth"
	"golang.org/x/text/unicode/norm"
)

// ExpandPath expands path beginning of ~  to the home directory.
// Not use for file operations because unexpected behavior when exist a file beginning of ~
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
	return name
}

// AbbrPath abbreviates path beginning of home directory to ~.
func AbbrPath(name string) string {
	home, _ := os.UserHomeDir()
	lenhome := len(home)
	if len(name) >= lenhome && name[:lenhome] == home {
		return "~" + name[lenhome:]
	}
	if runtime.GOOS == "windows" {
		name = strings.Replace(strings.Replace(name, `\`, `/`, -1), `"`, `'`, -1)
	}
	return name
}

// ShortenPath returns a shorten path to be shorter than width.
func ShortenPath(path string, width int) string {
	if width < runewidth.StringWidth(path) {
		root := filepath.VolumeName(path)
		names := strings.Split(path, string(filepath.Separator))
		for i, name := range names[:len(names)-1] {
			if name == root {
				if name == "" {
					names[i] = string(filepath.Separator)
				}
				continue
			}
			for _, r := range name {
				names[i] = string(r)
				break
			}
			path = filepath.Join(names...)
			if runewidth.StringWidth(path) <= width {
				break
			}
		}
	}
	if runtime.GOOS == "windows" {
		path = strings.Replace(strings.Replace(path, `\`, `/`, -1), `"`, `'`, -1)
	}
	return path
}

// RemoveExt: return filename. removes extension from the name.
func RemoveExt(name string) string {
	if ext := filepath.Ext(name); ext != name {
		return name[:len(name)-len(ext)]
	}
	return name
}

// GetExt: returns extension only.
func GetExt(name string) string {
	if ext := filepath.Ext(name); ext != name {
		return ext
	}
	return ""
}

// GetFullPath: returns full path.
func GetFullPath(name string) string {
	if runtime.GOOS == "windows" {
		name = strings.Replace(strings.Replace(name, `\`, `/`, -1), `"`, `'`, -1)
	}
	return name
}

// GetParentPath: returns parent path.
func GetParentPath(name string) string {
	if runtime.GOOS == "windows" {
		return strings.Replace(strings.Replace(filepath.Dir(name), `\`, `/`, -1), `"`, `'`, -1)
	}
	return filepath.Dir(name)
}

// SplitWithSep splits string with separator.
func SplitWithSep(s, sep string) []string {
	n := strings.Count(s, sep)*2 + 1
	a := make([]string, n)
	c := sep[0]
	start := 0
	na := 0
	for i := 0; i+len(sep) <= len(s) && na+1 < n; i++ {
		if s[i] == c && (len(sep) == 1 || s[i:i+len(sep)] == sep) {
			a[na] = s[start:i]
			na++
			a[na] = s[i : i+len(sep)]
			na++

			start = i + len(sep)
			i += len(sep) - 1
		}
	}
	a[na] = s[start:]
	return a[0 : na+1]
}

// Quote encloses string double quotes and escapes by backslash if this string contains double quotes.
func Quote(s string) string {
	if runtime.GOOS == "windows" {
		return fmt.Sprintf(`'%s'`, strings.Replace(strings.Replace(s, `\`, `/`, -1), `"`, `'`, -1)) //윈도우에서 역슬래시랑 따옴표가 문제가 있음. 파워쉘은 그냥 슬래시와 홑따옴표도 가능하니 그냥 싹바꾸는 것이 편함
	} else {
		return fmt.Sprintf(`'%s'`, strings.Replace(s, `"`, `\"`, -1)) //원래는 쌍따옴표인데, 그냥 홑따옴표로 통일함
	}

}

// FormatSize returns formated to SI prefix unit with 3-digit alignment.
func FormatSize(n int64) string {
	const (
		Tb = 1024 * 1024 * 1024 * 1024
		Gb = 1024 * 1024 * 1024
		Mb = 1024 * 1024
		kb = 1024
	)

	var size float64
	var unit string

	if n > Tb {
		size = float64(n) / Tb
		unit = "T"
	} else if n > Gb {
		size = float64(n) / Gb
		unit = "G"
	} else if n > Mb {
		size = float64(n) / Mb
		unit = "M"
	} else if n > kb {
		size = float64(n) / kb
		unit = "k"
	} else {
		size = float64(n)
		unit = "b"
	}

	// 3자리 맞춤을 위한 포맷팅
	if size < 10 {
		return fmt.Sprintf("__%.1f%s", size, unit)
	} else if size < 100 {
		return fmt.Sprintf("_%.1f%s", size, unit)
	} else if size < 1000 {
		return fmt.Sprintf("%.1f%s", size, unit)
	} else {
		// 1000 이상일 때는 다음 단위로 변환
		if unit == "k" {
			return fmt.Sprintf("__%.1fM", size/1024)
		} else if unit == "M" {
			return fmt.Sprintf("__%.1fG", size/1024)
		} else if unit == "G" {
			return fmt.Sprintf("__%.1fT", size/1024)
		} else {
			return fmt.Sprintf("__%.1fk", size/1024)
		}
	}
}

// FormatSizeForPane returns formated size for pane display without underscore prefix and with 'b' suffix for bytes.
func FormatSizeForPane(n int64) string {
	const (
		Tb = 1024 * 1024 * 1024 * 1024
		Gb = 1024 * 1024 * 1024
		Mb = 1024 * 1024
		kb = 1024
	)

	var size float64
	var unit string

	if n > Tb {
		size = float64(n) / Tb
		unit = "T"
	} else if n > Gb {
		size = float64(n) / Gb
		unit = "G"
	} else if n > Mb {
		size = float64(n) / Mb
		unit = "M"
	} else if n > kb {
		size = float64(n) / kb
		unit = "k"
	} else {
		size = float64(n)
		unit = "b"
	}

	// pane용 포맷팅 (언더스코어 없이, b 단위 추가)
	if size < 10 {
		return fmt.Sprintf(" %.1f%s", size, unit)
	} else if size < 100 {
		return fmt.Sprintf("%.1f%s", size, unit)
	} else if size < 1000 {
		return fmt.Sprintf("%.1f%s", size, unit)
	} else {
		// 1000 이상일 때는 다음 단위로 변환
		if unit == "kb" {
			return fmt.Sprintf(" %.1fMb", size/1024)
		} else if unit == "Mb" {
			return fmt.Sprintf(" %.1fGb", size/1024)
		} else if unit == "Gb" {
			return fmt.Sprintf(" %.1fTb", size/1024)
		} else {
			return fmt.Sprintf(" %.1fkb", size/1024)
		}
	}
}

func searchPath(results map[string]bool, path string) (map[string]bool, error) {
	dir, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("searching paths: %w", err)
	}
	defer dir.Close()
	for {
		names, err := dir.Readdirnames(100)
		if err == io.EOF {
			break
		} else if err != nil {
			return results, err
		}
		for _, name := range names {
			results[name] = true
		}
	}
	return results, nil
}

// SearchCommands returns map with key is command name in $PATH.
func SearchCommands() (map[string]bool, error) {
	results := map[string]bool{}
	for _, path := range strings.Split(os.Getenv("PATH"), string(os.PathListSeparator)) {
		if results, err := searchPath(results, path); err != nil {
			if !os.IsNotExist(err) {
				return results, err
			}
		}
	}
	if runtime.GOOS == "windows" {
		for key := range results {
			if filepath.Ext(key) == ".exe" {
				results[RemoveExt(key)] = true
			}
		}
	}
	return results, nil
}

// CalcSizeCount calculates files total size and count, excluding themself of
// directories and links.
func CalcSizeCount(src ...string) (int64, int) {
	size := int64(0)
	count := 0
	for _, s := range src {
		_ = filepath.Walk(s, func(path string, fi os.FileInfo, err error) error {
			if fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 {
				return nil
			}
			size += fi.Size()
			count++
			return nil
		})
	}
	return size, count
}

// NormalizeFileName normalizes Unicode filename from NFD to NFC
// This is particularly useful on macOS where filenames are stored in NFD format
// causing Korean characters to appear as separated components
func NormalizeFileName(name string) string {
	if runtime.GOOS == "darwin" {
		// NFD (정규분해) → NFC (정규결합) 변환
		return norm.NFC.String(name)
	}
	return name
}
