//go:build linux

package main

func copyFilesToMacClipboard(files []string) error {
	// 리눅스에서는 아무 동작도 하지 않음
	return nil
}
