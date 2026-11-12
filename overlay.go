package main

import (
	"encoding/json"
	"fmt"
	"image/color"
	"math"
	"os"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/AllenDang/giu"
	imgui "github.com/AllenDang/imgui-go"
	"golang.org/x/sys/windows"
)

type RECT struct{ Left, Top, Right, Bottom int32 }
type POINT struct{ X, Y int32 }

const (
	VK_INSERT         = 0x2D
	VK_END            = 0x23
	WS_EX_LAYERED     = 0x00080000
	WS_EX_TRANSPARENT = 0x00000020
	WS_EX_NOACTIVATE  = 0x08000000
	WS_EX_TOOLWINDOW  = 0x00000080
)

var (
	screenW, screenH      int
	insertDown, endDown   bool
	espVisible            = true
	overlayHWND           uintptr
	robloxHWND            uintptr
	lastSettingsReload    time.Time
	lastRobloxLookup      time.Time
	lastFrameTime         time.Time
	frameTimes            []float64
	u32                   = windows.NewLazySystemDLL("user32.dll")
	pGetClientRect        = u32.NewProc("GetClientRect")
	pClientToScreen       = u32.NewProc("ClientToScreen")
	pSetWindowPos         = u32.NewProc("SetWindowPos")
	pEnumWindows          = u32.NewProc("EnumWindows")
	pIsWindowVisible      = u32.NewProc("IsWindowVisible")
	pGetWindowThreadPID   = u32.NewProc("GetWindowThreadProcessId")
	pIsWindow             = u32.NewProc("IsWindow")
	pGetWindowLongPtrW    = u32.NewProc("GetWindowLongPtrW")
	pSetWindowLongPtrW    = u32.NewProc("SetWindowLongPtrW")
	pSetLayeredAttrs      = u32.NewProc("SetLayeredWindowAttributes")
)

func runOverlayMode() {
	loadSettingsFromDisk()

	var pid uint32
	for {
		p, err := findPID()
		if err == nil {
			pid = p
			break
		}
		time.Sleep(1 * time.Second)
	}

	mem, err := OpenMemory(pid)
	if err != nil {
		time.Sleep(2 * time.Second)
		os.Exit(1)
	}
	defer mem.Close()

	base, err := getModuleBase(pid, "RobloxPlayerBeta.exe")
	if err != nil {
		time.Sleep(2 * time.Second)
		os.Exit(1)
	}
	mem.Base = base

	off := LoadOffsets()
	g := NewGame(mem, off)

	sw, sh := GetScreenSize()
	if sw == 0 {
		sw = 1920
	}
	if sh == 0 {
		sh = 1080
	}

	wnd := giu.NewMasterWindow("PlutanaOverlay", sw, sh, giu.MasterWindowFlagsNotResizable)
	wnd.SetBgColor(color.RGBA{0, 0, 0, 255})
	lastFrameTime = time.Now()

	wnd.Run(func() {
		renderOverlay(g)
	})
}

func findRobloxMainWindow() uintptr {
	pid, _ := findPID()
	if pid == 0 {
		return 0
	}
	var found uintptr
	cb := syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
		if r, _, _ := pIsWindowVisible.Call(hwnd); r == 0 {
			return 1
		}
		var wpid uint32
		pGetWindowThreadPID.Call(hwnd, uintptr(unsafe.Pointer(&wpid)))
		if wpid != pid {
			return 1
		}
		var rc RECT
		pGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
		if rc.Right-rc.Left < 400 || rc.Bottom-rc.Top < 300 {
			return 1
		}
		found = hwnd
		return 0
	})
	pEnumWindows.Call(cb, 0)
	return found
}

func syncOverlayToRoblox() {
	now := time.Now()
	if robloxHWND == 0 || now.Sub(lastRobloxLookup) > 2*time.Second {
		lastRobloxLookup = now
		if robloxHWND != 0 {
			if r, _, _ := pIsWindow.Call(robloxHWND); r == 0 {
				robloxHWND = 0
			}
		}
		if robloxHWND == 0 {
			robloxHWND = findRobloxMainWindow()
		}
	}
	if robloxHWND == 0 || overlayHWND == 0 {
		return
	}
	var rc RECT
	pGetClientRect.Call(robloxHWND, uintptr(unsafe.Pointer(&rc)))
	var pt POINT
	pClientToScreen.Call(robloxHWND, uintptr(unsafe.Pointer(&pt)))
	w := int(rc.Right - rc.Left)
	h := int(rc.Bottom - rc.Top)
	if w <= 0 || h <= 0 {
		return
	}
	screenW = w
	screenH = h
	pSetWindowPos.Call(overlayHWND, uintptr(^uintptr(0)),
		uintptr(pt.X), uintptr(pt.Y), uintptr(w), uintptr(h),
		0x0010)
}

func enableColorKeyAndClickThrough(hwnd uintptr) {
	var gwlExStyle int32 = -20
	cur, _, _ := pGetWindowLongPtrW.Call(hwnd, uintptr(gwlExStyle))
	style := uintptr(int64(int32(cur)))
	style |= WS_EX_LAYERED | WS_EX_TRANSPARENT | WS_EX_NOACTIVATE | WS_EX_TOOLWINDOW
	pSetWindowLongPtrW.Call(hwnd, uintptr(gwlExStyle), style)
	pSetLayeredAttrs.Call(hwnd, 0x00000000, 0, 0x00000001)
}

func findWindowByTitle(title string) uintptr {
	proc := u32.NewProc("FindWindowW")
	u16, _ := windows.UTF16PtrFromString(title)
	h, _, _ := proc.Call(0, uintptr(unsafe.Pointer(u16)))
	return h
}

func calcFPS() float64 {
	now := time.Now()
	if !lastFrameTime.IsZero() {
		dt := now.Sub(lastFrameTime).Seconds()
		if dt > 0 && dt < 0.5 {
			frameTimes = append(frameTimes, dt)
			if len(frameTimes) > 90 {
				frameTimes = frameTimes[1:]
			}
		}
	}
	lastFrameTime = now
	if len(frameTimes) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range frameTimes {
		sum += v
	}
	return float64(len(frameTimes)) / sum
}

func toVec2(x, y float32) imgui.Vec2    { return imgui.Vec2{X: x, Y: y} }
func toVec4(c [4]float32) imgui.Vec4    { return imgui.Vec4{X: c[0], Y: c[1], Z: c[2], W: c[3]} }
func solidVec4(r, g, b, a float32) imgui.Vec4 { return imgui.Vec4{X: r, Y: g, Z: b, W: a} }

func getBoneList(r15 bool) [][2]string {
	if r15 {
		return [][2]string{
			{"Head", "UpperTorso"}, {"UpperTorso", "LowerTorso"},
			{"UpperTorso", "LeftUpperArm"}, {"LeftUpperArm", "LeftLowerArm"}, {"LeftLowerArm", "LeftHand"},
			{"UpperTorso", "RightUpperArm"}, {"RightUpperArm", "RightLowerArm"}, {"RightLowerArm", "RightHand"},
			{"LowerTorso", "LeftUpperLeg"}, {"LeftUpperLeg", "LeftLowerLeg"}, {"LeftLowerLeg", "LeftFoot"},
			{"LowerTorso", "RightUpperLeg"}, {"RightUpperLeg", "RightLowerLeg"}, {"RightLowerLeg", "RightFoot"},
		}
	}
	return [][2]string{
		{"Head", "Torso"}, {"Torso", "Left Arm"}, {"Torso", "Right Arm"},
		{"Torso", "Left Leg"}, {"Torso", "Right Leg"},
	}
}

func renderOverlay(g *Game) {
	if overlayHWND == 0 {
		overlayHWND = findWindowByTitle("PlutanaOverlay")
		if overlayHWND != 0 {
			enableColorKeyAndClickThrough(overlayHWND)
		}
	}

	// END quits, INSERT toggles ESP on/off
	end := GetAsyncKeyState(VK_END)
	if end && !endDown {
		os.Exit(0)
	}
	endDown = end
	ins := GetAsyncKeyState(VK_INSERT)
	if ins && !insertDown {
		espVisible = !espVisible
	}
	insertDown = ins

	// Reload settings every 1s so launcher changes apply
	if time.Since(lastSettingsReload) > time.Second {
		lastSettingsReload = time.Now()
		loadSettingsFromDisk()
	}

	// Sync overlay window to Roblox client rect
	syncOverlayToRoblox()

	// Fullscreen invisible ImGui window so we have a valid drawlist
	giu.Window("##canvas").
		Flags(giu.WindowFlagsNoTitleBar|giu.WindowFlagsNoBackground|
			giu.WindowFlagsNoMove|giu.WindowFlagsNoResize|giu.WindowFlagsNoScrollbar|
			giu.WindowFlagsNoBringToFrontOnFocus|giu.WindowFlagsNoCollapse|giu.WindowFlagsNoSavedSettings).
		Pos(0, 0).
		Size(float32(screenW), float32(screenH)).
		Layout(giu.Dummy(0, 0))

	dl := giu.GetCanvas().DrawList

	if !g.ProbeGame() {
		giu.Update()
		return
	}

	vm := g.ViewMatrix()
	lp := g.LocalPlayer()
	lt := g.Team(lp)
	lpChar := g.Character(lp)
	lpHum := g.Humanoid(lpChar)
	if lpHum != 0 {
		ApplyPlayer(g, lp, lpHum)
	}

	players := g.GetChildren(g.Players)
	if Settings.AimEnabled {
		DoAim(g, players, lp, lt, vm, screenW, screenH)
	}

	var lpHeadX, lpHeadY float32
	lpHeadValid := false
	if lpChar != 0 {
		lcd := g.CharData(lpChar)
		if lcd != nil {
			if head, ok := lcd.Parts["Head"]; ok {
				lpHeadX, lpHeadY, _, lpHeadValid = g.Pos(head)
			}
		}
	}

	if espVisible && Settings.ShowFOV && Settings.AimEnabled {
		dl.AddCircle(toVec2(float32(screenW)/2, float32(screenH)/2),
			float32(Settings.FOVRadius), toVec4(Settings.ColFOV), 64, 1.5)
	}

	if espVisible && Settings.ESPEnabled {
		for _, player := range players {
			if player == lp {
				continue
			}
			if Settings.TeamCheck && lt != 0 && g.Team(player) == lt {
				continue
			}
			ch := g.Character(player)
			if ch == 0 {
				continue
			}
			cd := g.CharData(ch)
			if cd == nil {
				continue
			}
			chp := g.mem.ReadF32(cd.Hum + g.off.Health)
			if chp <= 0 {
				continue
			}

			type pt struct {
				name string
				x, y float32
			}
			pts := make([]pt, 0, len(cd.Parts))
			partMap := make(map[string]pt)
			minX := float32(math.MaxFloat32)
			minY := float32(math.MaxFloat32)
			maxX := float32(-math.MaxFloat32)
			maxY := float32(-math.MaxFloat32)
			for name, part := range cd.Parts {
				px, py, pz, ok := g.Pos(part)
				if !ok {
					continue
				}
				sx, sy, vis := g.WorldToScreen(vm, px, py, pz, float32(screenW), float32(screenH))
				if !vis {
					continue
				}
				p := pt{name, sx, sy}
				pts = append(pts, p)
				partMap[name] = p
				if sx < minX {
					minX = sx
				}
				if sx > maxX {
					maxX = sx
				}
				if sy < minY {
					minY = sy
				}
				if sy > maxY {
					maxY = sy
				}
			}
			if len(pts) < 2 || maxX < 0 {
				continue
			}

			dist := float32(-1)
			if lpHeadValid {
				if hp3, ok := partMap["Head"]; ok {
					dx, dy := hp3.x-lpHeadX, hp3.y-lpHeadY
					dist = float32(math.Sqrt(float64(dx*dx + dy*dy)))
				}
			}
			if Settings.MaxDistance > 0 && dist > float32(Settings.MaxDistance) {
				continue
			}

			bx, by := minX-4, minY-4
			bw, bh := maxX-minX+8, maxY-minY+8
			cx, cy := bx+bw/2, by+bh/2

			if Settings.BoxFill {
				fill := toVec4(Settings.ColBox)
				fill.W = float32(Settings.FillAlpha) / 255
				dl.AddRectFilled(toVec2(bx, by), toVec2(bx+bw, by+bh), fill, 0, 0)
			}
			if Settings.Box {
				if Settings.CornerBox {
					cl := bw / 4
					if bh/4 < cl {
						cl = bh / 4
					}
					c := toVec4(Settings.ColBox)
					dl.AddLine(toVec2(bx, by), toVec2(bx+cl, by), c, 1.5)
					dl.AddLine(toVec2(bx, by), toVec2(bx, by+cl), c, 1.5)
					dl.AddLine(toVec2(bx+bw-cl, by), toVec2(bx+bw, by), c, 1.5)
					dl.AddLine(toVec2(bx+bw, by), toVec2(bx+bw, by+cl), c, 1.5)
					dl.AddLine(toVec2(bx, by+bh-cl), toVec2(bx, by+bh), c, 1.5)
					dl.AddLine(toVec2(bx, by+bh), toVec2(bx+cl, by+bh), c, 1.5)
					dl.AddLine(toVec2(bx+bw-cl, by+bh), toVec2(bx+bw, by+bh), c, 1.5)
					dl.AddLine(toVec2(bx+bw, by+bh-cl), toVec2(bx+bw, by+bh), c, 1.5)
				} else {
					dl.AddRect(toVec2(bx, by), toVec2(bx+bw, by+bh), toVec4(Settings.ColBox), 0, 0, 1.5)
				}
			}
			if Settings.Dot {
				dl.AddCircleFilled(toVec2(cx, cy), 3, toVec4(Settings.ColDot))
			}
			if Settings.Health {
				ratio := chp / cd.MaxHP
				barX := bx - 6
				barH := bh * ratio
				dl.AddRectFilled(toVec2(barX-1, by-1), toVec2(barX+4, by+bh+1), solidVec4(0, 0, 0, 1), 0, 0)
				dl.AddRectFilled(toVec2(barX, by+bh-barH), toVec2(barX+3, by+bh), solidVec4(1-ratio, ratio, 0, 1), 0, 0)
			}
			if Settings.HealthText {
				txt := fmt.Sprintf("%.0f", chp)
				if Settings.HealthPercent {
					txt = fmt.Sprintf("%.0f (%.0f%%)", chp, chp/cd.MaxHP*100)
				}
				dl.AddText(toVec2(bx+bw+4, cy-6), toVec4(Settings.ColHealth), txt)
			}
			if Settings.Distance && dist > 0 {
				dval := dist
				unit := ""
				if Settings.DistanceInMeters {
					dval = dist * 0.28
					unit = "m"
				}
				dl.AddText(toVec2(bx, by+bh+2), toVec4(Settings.ColDist), fmt.Sprintf("[%.0f%s]", dval, unit))
			}
			if Settings.Skeleton {
				c := toVec4(Settings.ColSkel)
				for _, b := range getBoneList(cd.R15) {
					p1, ok1 := partMap[b[0]]
					p2, ok2 := partMap[b[1]]
					if ok1 && ok2 {
						dl.AddLine(toVec2(p1.x, p1.y), toVec2(p2.x, p2.y), c, 1.5)
					}
				}
			}
			if Settings.SkeletonDots {
				c := toVec4(Settings.ColSkel)
				for _, p := range pts {
					dl.AddCircleFilled(toVec2(p.x, p.y), 2.5, c)
				}
			}
			if Settings.Name {
				if nm := g.GetName(player); nm != "" {
					dl.AddText(toVec2(bx, by-16), toVec4(Settings.ColName), nm)
				}
			}
			if Settings.Tracers {
				var tx, ty float32
				if Settings.TracerFromMouse {
					var pt struct{ X, Y int32 }
					u32.NewProc("GetCursorPos").Call(uintptr(unsafe.Pointer(&pt)))
					tx, ty = float32(pt.X), float32(pt.Y)
				} else {
					tx, ty = float32(screenW)/2, float32(screenH)
				}
				if head, ok := partMap["Head"]; ok {
					dl.AddLine(toVec2(tx, ty), toVec2(head.x, head.y), toVec4(Settings.ColTracer), 1.0)
				}
			}
		}
	}

	if espVisible && Settings.ShowFPS {
		dl.AddRectFilled(toVec2(8, 8), toVec2(140, 30), solidVec4(0, 0, 0, 0.78), 0, 0)
		dl.AddText(toVec2(15, 12), solidVec4(1, 1, 1, 1), fmt.Sprintf("FPS: %.0f", calcFPS()))
	}

	giu.Update()
}

var _ = json.Marshal
var _ = strings.EqualFold