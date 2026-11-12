//go:build windows

package main

import (
	"os"
	"syscall"
)

const enableVirtualTerminalProcessing = 0x0004

var (
	kernel32       = syscall.NewLazyDLL("kernel32.dll")
	setConsoleMode = kernel32.NewProc("SetConsoleMode")
)

func enableColor(file *os.File) bool {
	handle := syscall.Handle(file.Fd())
	var mode uint32
	if err := syscall.GetConsoleMode(handle, &mode); err != nil {
		return false
	}
	if mode&enableVirtualTerminalProcessing != 0 {
		return true
	}
	result, _, _ := setConsoleMode.Call(
		uintptr(handle),
		uintptr(mode|enableVirtualTerminalProcessing),
	)
	return result != 0
}
