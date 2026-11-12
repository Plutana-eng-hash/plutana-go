package main

import (
	"os"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

func isAdmin() bool {
	r, _, _ := windows.NewLazySystemDLL("shell32.dll").NewProc("IsUserAnAdmin").Call()
	return r != 0
}

func relaunchAsAdmin() {
	proc := windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteW")
	exe, _ := os.Executable()
	verb, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString(exe)
	args, _ := windows.UTF16PtrFromString(strings.Join(os.Args[1:], " "))
	dir, _ := windows.UTF16PtrFromString("")
	proc.Call(0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(file)),
		uintptr(unsafe.Pointer(args)),
		uintptr(unsafe.Pointer(dir)),
		1)
	os.Exit(0)
}

func main() {
	if !isAdmin() {
		relaunchAsAdmin()
	}

	for _, a := range os.Args[1:] {
		if a == "--overlay" {
			runOverlayMode()
			return
		}
	}
	runLauncherMode()
}