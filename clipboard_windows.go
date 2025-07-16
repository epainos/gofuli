//go:build windows

package main

func copyFilesToMacClipboard(files []string) error {
	// 윈도우에서는 아무 동작도 하지 않음
	return nil
}
