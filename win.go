package main

import (
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

func findProcessID(name string) (uint32, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(snap)

	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))

	for {
		if err := windows.Process32Next(snap, &pe); err != nil {
			break
		}
		if strings.EqualFold(windows.UTF16ToString(pe.ExeFile[:]), name) {
			return pe.ProcessID, nil
		}
	}
	return 0, fmt.Errorf("%s not found", name)
}

func findPID() (uint32, error) {
	return findProcessID("RobloxPlayerBeta.exe")
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

func GetScreenSize() (int, int) {
	proc := windows.NewLazySystemDLL("user32.dll").NewProc("GetSystemMetrics")
	w, _, _ := proc.Call(0)
	h, _, _ := proc.Call(1)
	return int(int32(w)), int(int32(h))
}