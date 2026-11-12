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
		fmt.Println("Elevation failed. Please run as administrator.")
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
		name := windows.UTF16ToString(me.Module[:])
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
		fmt.Println("[!] Requesting administrator privileges...")
		time.Sleep(500 * time.Millisecond)
		relaunchAsAdmin()
	}

	fmt.Println("=" + strings.Repeat("=", 60))
	fmt.Println("  Plutana-Go v" + VERSION)
	fmt.Println("=" + strings.Repeat("=", 60))
	fmt.Println()
	fmt.Println("  INSERT  - Toggle settings menu")
	fmt.Println("  END     - Quit")
	fmt.Println()
	fmt.Println("=" + strings.Repeat("=", 60))
	fmt.Println()

	var pid uint32
	firstWait := true
	for {
		p, err := findPID()
		if err == nil {
			pid = p
			break
		}
		if firstWait {
			fmt.Println("[~] Roblox is not running.")
			fmt.Println("[~] Please start Roblox and join a game.")
			fmt.Println("[~] Waiting...")
			firstWait = false
		} else {
			fmt.Println("[~] Still waiting for Roblox...")
		}
		time.Sleep(3 * time.Second)
	}
	fmt.Println()
	fmt.Printf("[+] Found Roblox! PID: %d\n", pid)

	mem, err := OpenMemory(pid)
	if err != nil {
		fmt.Println("[!] OpenProcess failed:", err)
		fmt.Println("[!] Make sure Roblox is fully loaded, then restart this program.")
		time.Sleep(5 * time.Second)
		return
	}
	defer mem.Close()

	base, err := getModuleBase(pid, "RobloxPlayerBeta.exe")
	if err != nil {
		fmt.Println("[!] Could not find RobloxPlayerBeta.exe module:", err)
		time.Sleep(5 * time.Second)
		return
	}
	mem.Base = base
	fmt.Printf("[+] Base address: 0x%X\n", base)

	off := LoadOffsets()
	g := NewGame(mem, off)
	Settings = LoadSettings()

	fmt.Println("[+] Starting overlay...")
	fmt.Println("[+] Press INSERT to open the settings menu.")
	fmt.Println("[+] Press END to quit.")
	fmt.Println()

	RunOverlay(g)
	fmt.Println("[+] Overlay closed. Goodbye.")
}