package main

import (
	"fmt"
	"os"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func isAdmin() bool {
	proc := windows.NewLazySystemDLL("shell32.dll").NewProc("IsUserAnAdmin")
	r, _, _ := proc.Call()
	return r != 0
}

func relaunchAsAdmin() {
	proc := windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteW")
	exe, _ := os.Executable()
	verb, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString(exe)
	args, _ := windows.UTF16PtrFromString("")
	dir, _ := windows.UTF16PtrFromString("")
	r, _, _ := proc.Call(
		0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(file)),
		uintptr(unsafe.Pointer(args)),
		uintptr(unsafe.Pointer(dir)),
		1,
	)
	if r <= 32 {
		fmt.Println("Elevation failed. Run as administrator.")
		time.Sleep(3 * time.Second)
		os.Exit(1)
	}
	os.Exit(0)
}

func findPID() (uint32, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(snap)

	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))

	for {
		err := windows.Process32Next(snap, &pe)
		if err != nil {
			break
		}
		name := windows.UTF16ToString(pe.ExeFile[:])
		if name == "RobloxPlayerBeta.exe" {
			return pe.ProcessID, nil
		}
	}
	return 0, fmt.Errorf("Roblox not found")
}

func getModuleBase(pid uint32, moduleName string) (uintptr, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPMODULE|windows.TH32CS_SNAPMODULE32, pid)
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(snap)

	var me windows.ModuleEntry32
	me.Size = uint32(unsafe.Sizeof(me))

	if err := windows.Module32First(snap, &me); err != nil {
		return 0, err
	}

	for {
		name := windows.UTF16ToString(me.SzModule[:])
		if strings.EqualFold(name, moduleName) {
			return me.ModBaseAddr, nil
		}
		if err := windows.Module32Next(snap, &me); err != nil {
			break
		}
	}
	return 0, fmt.Errorf("module %s not found", moduleName)
}

func main() {
	if !isAdmin() {
		relaunchAsAdmin()
	}

	fmt.Println("=" + strings.Repeat("=", 58))
	fmt.Println("Plutana-Go v" + VERSION)
	fmt.Println("=" + strings.Repeat("=", 58))

	var pid uint32
	for {
		p, err := findPID()
		if err == nil {
			pid = p
			break
		}
		fmt.Println("[~] Waiting for Roblox...")
		time.Sleep(1 * time.Second)
	}
	fmt.Printf("[+] Roblox PID: %d\n", pid)

	mem, err := OpenMemory(pid)
	if err != nil {
		fmt.Println("[!] OpenProcess failed:", err)
		return
	}
	defer mem.Close()

	base, err := getModuleBase(pid, "RobloxPlayerBeta.exe")
	if err != nil {
		fmt.Println("[!] Base address failed:", err)
		return
	}
	mem.Base = base
	fmt.Printf("[+] Base: 0x%X\n", base)

	off := LoadOffsets()
	g := NewGame(mem, off)
	Settings = LoadSettings()

	RunOverlay(g)
}