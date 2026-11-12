package main

import (
    "fmt"
    "os"
    "time"
    "unsafe"

    "golang.org/x/sys/windows"
)

func isAdmin() bool {
    shell32 := windows.NewLazyDLL("shell32.dll")
    proc := shell32.NewProc("IsUserAnAdmin")
    r, _, _ := proc.Call()
    return r != 0
}

func relaunchAsAdmin() {
    shell32 := windows.NewLazyDLL("shell32.dll")
    shellExecute := shell32.NewProc("ShellExecuteW")
    exe, _ := os.Executable()
    verb, _ := windows.UTF16PtrFromString("runas")
    file, _ := windows.UTF16PtrFromString(exe)
    args, _ := windows.UTF16PtrFromString("")
    dir, _ := windows.UTF16PtrFromString("")
    r, _, _ := shellExecute.Call(
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

func getModuleBase(pid uint32, module string) (uintptr, error) {
    h, err := windows.OpenProcess(windows.PROCESS_QUERY_INFORMATION|windows.PROCESS_VM_READ, false, pid)
    if err != nil {
        return 0, err
    }
    defer windows.CloseHandle(h)

    var mods [1024]windows.Handle
    var needed uint32
    err = windows.EnumProcessModules(h, &mods[0], uint32(len(mods))*uint32(unsafe.Sizeof(mods[0])), &needed)
    if err != nil {
        return 0, err
    }
    count := int(needed / uint32(unsafe.Sizeof(mods[0])))
    for i := 0; i < count; i++ {
        var buf [windows.MAX_PATH]uint16
        err := windows.GetModuleBaseName(h, mods[i], &buf[0], windows.MAX_PATH)
        if err != nil {
            continue
        }
        if windows.UTF16ToString(buf[:]) == module {
            var info windows.ModuleInfo
            err := windows.GetModuleInformation(h, mods[i], &info, uint32(unsafe.Sizeof(info)))
            if err != nil {
                return 0, err
            }
            return info.BaseOfDll, nil
        }
    }
    return 0, fmt.Errorf("module %s not found", module)
}

func main() {
    if !isAdmin() {
        relaunchAsAdmin()
    }

    fmt.Println("=" + fmt.Sprintf("%58s", "") + "=")
    fmt.Println("Plutana-Go v1.0")
    fmt.Println("=" + fmt.Sprintf("%58s", "") + "=")

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