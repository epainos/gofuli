//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation -framework AppKit
#import <Foundation/Foundation.h>
#import <AppKit/AppKit.h>

int copyFilesToPasteboard(char** filePaths, int count) {
    @autoreleasepool {
        NSPasteboard *pasteboard = [NSPasteboard generalPasteboard];
        [pasteboard clearContents];

        NSMutableArray *fileURLs = [[NSMutableArray alloc] init];
        NSMutableArray *pathStrings = [[NSMutableArray alloc] init]; // 경로 문자열을 담을 배열

        for (int i = 0; i < count; i++) {
            NSString *filePath = [NSString stringWithUTF8String:filePaths[i]];
            NSURL *fileURL = [NSURL fileURLWithPath:filePath];
            [fileURLs addObject:fileURL];
            [pathStrings addObject:filePath]; // 문자열 배열에도 추가
        }

        if ([fileURLs count] == 0) {
            return 0;
        }

        // 1. 파일 URL 객체를 쓴다 (Finder용)
        BOOL success = [pasteboard writeObjects:fileURLs];
        if (!success) {
            return 0;
        }

        // 2. 모든 파일 경로를 '\n'으로 합쳐서 하나의 문자열로 만든다 (glippy, pbpaste용)
        NSString *allPathsString = [pathStrings componentsJoinedByString:@"\n"];
        [pasteboard setString:allPathsString forType:NSPasteboardTypeString];

        return 1;
    }
}
*/
import "C"

import (
    "fmt"
    "os"
    "path/filepath"
    "unsafe"
)

// copyFilesToMacClipboard는 macOS에서 파일들을 클립보드에 복사합니다
func copyFilesToMacClipboard(filePathsOri []string) error {
    // 파일 존재 확인
    var filePaths []string
    for _, file := range filePathsOri {
        if fileExists(file) {
            filePaths = append(filePaths, file)
        }
    }

    if len(filePaths) == 0 {
        return fmt.Errorf("복사할 파일(경로)이 없습니다")
    }

    // 절대 경로로 변환
    var absPaths []string
    for _, path := range filePaths {
        absPath, err := filepath.Abs(path)
        if err != nil {
            return fmt.Errorf("경로 변환 실패 (%s): %v", path, err)
        }
        absPaths = append(absPaths, absPath)
    }

    // C 문자열 배열로 변환
    cPaths := make([]*C.char, len(absPaths))
    for i, path := range absPaths {
        cPaths[i] = C.CString(path)
        defer C.free(unsafe.Pointer(cPaths[i]))
    }

    // 배열의 첫 번째 요소 포인터 가져오기
    var cPathsPtr **C.char
    if len(cPaths) > 0 {
        cPathsPtr = &cPaths[0]
    }

    // Objective-C 함수 호출
    success := C.copyFilesToPasteboard(cPathsPtr, C.int(len(cPaths)))

    if success == 0 {
        return fmt.Errorf("클립보드 복사에 실패했습니다")
    }

    return nil
}

// fileExists는 파일 존재 여부를 확인합니다
func fileExists(path string) bool {
    _, err := os.Stat(path)
    return err == nil
}