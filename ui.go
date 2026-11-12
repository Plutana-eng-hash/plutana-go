package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"time"
	"unsafe"

	"github.com/AllenDang/giu"
	imgui "github.com/AllenDang/imgui-go"
	"golang.org/x/sys/windows"
)

type SettingsStruct struct {
	ESPEnabled       bool    `json:"esp_enabled"`
	Box              bool    `json:"box"`
	CornerBox        bool    `json:"corner_box"`
	BoxFill          bool    `json:"box_fill"`
	FillAlpha        int32   `json:"fill_alpha"`
	Dot              bool    `json:"dot"`
	HeadDot          bool    `json:"head_dot"`
	HeadCircle       bool    `json:"head_circle"`
	Skeleton         bool    `json:"skeleton"`
	SkeletonDots     bool    `json:"skeleton_dots"`
	Name             bool    `json:"name"`
	Health           bool    `json:"health"`
	HealthText       bool    `json:"health_text"`
	HealthPercent    bool    `json:"health_percent"`
	Distance         bool    `json:"distance"`
	DistanceInMeters bool    `json:"distance_meters"`
	Tracers          bool    `json:"tracers"`
	TracerFromMouse  bool    `json:"tracer_mouse"`
	ShowFPS          bool    `json:"show_fps"`
	TeamCheck        bool    `json:"team_check"`
	MaxDistance      int32   `json:"max_distance"`

	AimEnabled     bool   `json:"aim_enabled"`
	AimKey         string `json:"aim_key"`
	AimSmooth      int32  `json:"aim_smooth"`
	AimTarget      string `json:"aim_target"`
	AimIgnoreTeam  bool   `json:"aim_ignore_team"`
	AimMaxDistance int32  `json:"aim_max_distance"`
	ShowFOV        bool   `json:"aim_show_fov"`
	FOVRadius      int32  `json:"aim_fov_radius"`

	WalkSpeedEnabled bool  `json:"walk_speed_enabled"`
	WalkSpeed        int32 `json:"walk_speed"`
	JumpPowerEnabled bool  `json:"jump_power_enabled"`
	JumpPower        int32 `json:"jump_power"`
	PlayerVehicle    bool  `json:"player_vehicle"`
	HipHeightEnabled bool  `json:"hip_height_enabled"`
	HipHeight        int32 `json:"hip_height"`
	MaxSlopeEnabled  bool  `json:"max_slope_enabled"`
	MaxSlope         int32 `json:"max_slope"`
	AntiSit          bool  `json:"anti_sit"`
	CameraMaxEnabled bool  `json:"camera_max_enabled"`
	CameraMax        int32 `json:"camera_max"`

	ColBox    [4]float32 `json:"col_box"`
	ColDot    [4]float32 `json:"col_dot"`
	ColSkel   [4]float32 `json:"col_skel"`
	ColFOV    [4]float32 `json:"col_fov"`
	ColName   [4]float32 `json:"col_name"`
	ColTracer [4]float32 `json:"col_tracer"`
	ColDist   [4]float32 `json:"col_dist"`
	ColHead   [4]float32 `json:"col_head"`
	ColHealth [4]float32 `json:"col_health_text"`
}

var Settings = SettingsStruct{
	ESPEnabled:    true,
	Box:           true,
	FillAlpha:     45,
	ShowFPS:       true,
	TeamCheck:     true,
	AimKey:        "RMB",
	AimSmooth:     5,
	AimTarget:     "Head",
	AimIgnoreTeam: true,
	ShowFOV:       true,
	FOVRadius:     150,
	WalkSpeed:     16,
	JumpPower:     50,
	PlayerVehicle: true,
	HipHeight:     2,
	MaxSlope:      89,
	CameraMax:     128,
	ColBox:        [4]float32{1, 1, 1, 1},
	ColDot:        [4]float32{1, 0.24, 0.24, 1},
	ColSkel:       [4]float32{1, 0.78, 0, 1},
	ColFOV:        [4]float32{1, 1, 1, 0.78},
	ColName:       [4]float32{1, 1, 1, 1},
	ColTracer:     [4]float32{1, 1, 1, 1},
	ColDist:       [4]float32{1, 1, 1, 1},
	ColHead:       [4]float32{1, 0.31, 0.31, 1},
	ColHealth:     [4]float32{1, 1, 1, 1},
}

const settingsPath = "settings_go.json"

func LoadSettings() SettingsStruct {
	s := Settings
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return s
	}
	json.Unmarshal(data, &s)
	return s
}

func SaveSettings() {
	data, err := json.MarshalIndent(Settings, "", "  ")
	if err != nil {
		return
	}
	os.WriteFile(settingsPath, data, 0644)
}

func toVec2(x, y float32) imgui.Vec2 {
	return imgui.Vec2{X: x, Y: y}
}

func toVec4(c [4]float32) imgui.Vec4 {
	return imgui.Vec4{X: c[0], Y: c[1], Z: c[2], W: c[3]}
}

func solidVec4(r, g, b, a float32) imgui.Vec4 {
	return imgui.Vec4{X: r, Y: g, Z: b, W: a}
}

const (
	VK_INSERT         = 0x2D
	VK_END            = 0x23
	WS_EX_LAYERED     = 0x00080000
	WS_EX_TRANSPARENT = 0x00000020
	WS_EX_NOACTIVATE  = 0x08000000
	WS_EX_TOOLWINDOW  = 0x00000080
)

var (
	screenW, screenH int
	menuVisible      bool
	insertDown       bool
	endDown          bool
	lastSaveTime     time.Time
	overlayHWND      uintptr
	lastFrameTime    time.Time
	frameTimes       []float64
)

func GetScreenSize() (int, int) {
	proc := lazyUser32().NewProc("GetSystemMetrics")
	w, _, _ := proc.Call(0)
	h, _, _ := proc.Call(1)
	return int(int32(w)), int(int32(h))
}

func findWindowByTitle(title string) uintptr {
	user32 := windows.NewLazySystemDLL("user32.dll")
	proc := user32.NewProc("FindWindowW")
	u16, _ := windows.UTF16PtrFromString(title)
	h, _, _ := proc.Call(0, uintptr(unsafe.Pointer(u16)))
	return h
}

func setClickThrough(hwnd uintptr, clickThrough bool) {
	user32 := windows.NewLazySystemDLL("user32.dll")
	getStyle := user32.NewProc("GetWindowLongPtrW")
	setStyle := user32.NewProc("SetWindowLongPtrW")

	var gwlExStyle int32 = -20
	cur, _, _ := getStyle.Call(hwnd, uintptr(gwlExStyle))
	style := uintptr(int64(int32(cur)))
	style |= WS_EX_LAYERED | WS_EX_NOACTIVATE | WS_EX_TOOLWINDOW
	if clickThrough {
		style |= WS_EX_TRANSPARENT
	} else {
		style &^= WS_EX_TRANSPARENT
	}
	setStyle.Call(hwnd, uintptr(gwlExStyle), style)
}

func setTopmost(hwnd uintptr) {
	proc := windows.NewLazySystemDLL("user32.dll").NewProc("SetWindowPos")
	proc.Call(hwnd, uintptr(^uintptr(0)), 0, 0, 0, 0, 0x0001|0x0002|0x0040)
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

func drawFPS(dl imgui.DrawList, fps float64) {
	text := fmt.Sprintf("FPS: %.0f", fps)
	dl.AddRectFilled(toVec2(8, 8), toVec2(140, 30), solidVec4(0, 0, 0, 0.78), 0, 0)
	dl.AddText(toVec2(15, 12), solidVec4(1, 1, 1, 1), text)
}

func drawDiag(dl imgui.DrawList, g *Game) {
	y := float32(36)
	lines := []struct {
		txt string
		col imgui.Vec4
	}{
		{fmt.Sprintf("Base 0x%X", g.mem.Base), solidVec4(0.86, 0.86, 0.86, 1)},
		{fmt.Sprintf("DM 0x%X", g.DM), solidVec4(0.86, 0.86, 0.86, 1)},
		{fmt.Sprintf("VE 0x%X", g.VE), solidVec4(0.86, 0.86, 0.86, 1)},
	}
	for _, line := range lines {
		dl.AddRectFilled(toVec2(8, y-2), toVec2(250, y+18), solidVec4(0, 0, 0, 0.78), 0, 0)
		dl.AddText(toVec2(12, y), line.col, line.txt)
		y += 22
	}
}

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

func RenderFrame(g *Game) {
	if overlayHWND == 0 {
		overlayHWND = findWindowByTitle("Plutana-Go")
		if overlayHWND != 0 {
			setTopmost(overlayHWND)
			setClickThrough(overlayHWND, true)
		}
	}

	insertState := GetAsyncKeyState(VK_INSERT)
	if insertState && !insertDown {
		menuVisible = !menuVisible
		if overlayHWND != 0 {
			setClickThrough(overlayHWND, !menuVisible)
		}
	}
	insertDown = insertState

	endState := GetAsyncKeyState(VK_END)
	if endState && !endDown {
		fmt.Println("[+] END pressed. Quitting.")
		os.Exit(0)
	}
	endDown = endState

	if !g.ProbeGame() {
		giu.Window("Plutana-Wait").Layout(
			giu.Label("Waiting for game..."),
			giu.Label(fmt.Sprintf("Base 0x%X  DM 0x%X  VE 0x%X", g.mem.Base, g.DM, g.VE)),
		)
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

	dl := giu.GetCanvas().DrawList

	if Settings.ShowFOV && Settings.AimEnabled {
		cx := float32(screenW) / 2
		cy := float32(screenH) / 2
		dl.AddCircle(toVec2(cx, cy), float32(Settings.FOVRadius), toVec4(Settings.ColFOV), 64, 1.5)
	}

	if Settings.ESPEnabled {
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
					dx := hp3.x - lpHeadX
					dy := hp3.y - lpHeadY
					dist = float32(math.Sqrt(float64(dx*dx + dy*dy)))
				}
			}
			if Settings.MaxDistance > 0 && dist > 0 && dist > float32(Settings.MaxDistance) {
				continue
			}

			bx := minX - 4
			by := minY - 4
			bw := maxX - minX + 8
			bh := maxY - minY + 8
			cx := bx + bw/2
			cy := by + bh/2

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
				gCol := solidVec4(1-ratio, ratio, 0, 1)
				dl.AddRectFilled(toVec2(barX, by+bh-barH), toVec2(barX+3, by+bh), gCol, 0, 0)
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
				bones := getBoneList(cd.R15)
				c := toVec4(Settings.ColSkel)
				for _, b := range bones {
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
				nm := g.GetName(player)
				if nm != "" {
					dl.AddText(toVec2(bx, by-16), toVec4(Settings.ColName), nm)
				}
			}

			if Settings.Tracers {
				var tx, ty float32
				if Settings.TracerFromMouse {
					var pt struct{ X, Y int32 }
					proc := windows.NewLazySystemDLL("user32.dll").NewProc("GetCursorPos")
					proc.Call(uintptr(unsafe.Pointer(&pt)))
					tx = float32(pt.X)
					ty = float32(pt.Y)
				} else {
					tx = float32(screenW) / 2
					ty = float32(screenH)
				}
				if head, ok := partMap["Head"]; ok {
					dl.AddLine(toVec2(tx, ty), toVec2(head.x, head.y), toVec4(Settings.ColTracer), 1.0)
				}
			}
		}
	}

	if Settings.ShowFPS {
		drawFPS(dl, calcFPS())
	}
	drawDiag(dl, g)

	if menuVisible {
		drawMenu()
	}

	if time.Since(lastSaveTime) > 2*time.Second {
		SaveSettings()
		lastSaveTime = time.Now()
	}
}

var (
	aimKeyIndex    int32
	aimTargetIndex int32
)

func drawMenu() {
	giu.Window("Plutana Settings").
		Size(500, 600).
		Layout(
			giu.TabBar().TabItems(
				giu.TabItem("ESP").Layout(espTab()...),
				giu.TabItem("AIM").Layout(aimTab()...),
				giu.TabItem("PLAYER").Layout(playerTab()...),
			),
		)
}

func espTab() []giu.Widget {
	return []giu.Widget{
		giu.Checkbox("Enable ESP", &Settings.ESPEnabled),
		giu.Checkbox("Box", &Settings.Box),
		giu.Checkbox("Corner Box", &Settings.CornerBox),
		giu.Checkbox("Box Fill", &Settings.BoxFill),
		giu.SliderInt(&Settings.FillAlpha, 0, 255).Label("Fill Alpha"),
		giu.Checkbox("Dot", &Settings.Dot),
		giu.Checkbox("Head Dot", &Settings.HeadDot),
		giu.Checkbox("Skeleton", &Settings.Skeleton),
		giu.Checkbox("Skeleton Dots", &Settings.SkeletonDots),
		giu.Checkbox("Name", &Settings.Name),
		giu.Checkbox("Health Bar", &Settings.Health),
		giu.Checkbox("Health Text", &Settings.HealthText),
		giu.Checkbox("Health Percent", &Settings.HealthPercent),
		giu.Checkbox("Distance", &Settings.Distance),
		giu.Checkbox("Distance in Meters", &Settings.DistanceInMeters),
		giu.Checkbox("Tracers", &Settings.Tracers),
		giu.Checkbox("Tracer from Mouse", &Settings.TracerFromMouse),
		giu.Checkbox("Show FPS", &Settings.ShowFPS),
		giu.Checkbox("Team Check", &Settings.TeamCheck),
		giu.SliderInt(&Settings.MaxDistance, 0, 2000).Label("Max Distance"),
	}
}

func aimTab() []giu.Widget {
	return []giu.Widget{
		giu.Checkbox("Enable Aimbot", &Settings.AimEnabled),
		giu.Combo("Aim Key", Settings.AimKey, []string{"RMB", "LMB", "Shift", "Alt", "Ctrl", "E", "Q", "C", "Z", "X"}, &aimKeyIndex),
		giu.Combo("Target", Settings.AimTarget, []string{"Head", "Torso", "Closest"}, &aimTargetIndex),
		giu.Checkbox("Ignore Teammates", &Settings.AimIgnoreTeam),
		giu.SliderInt(&Settings.AimSmooth, 1, 20).Label("Smoothness"),
		giu.SliderInt(&Settings.AimMaxDistance, 0, 2000).Label("Max Distance"),
		giu.SliderInt(&Settings.FOVRadius, 20, 600).Label("FOV Radius"),
		giu.Checkbox("Show FOV Circle", &Settings.ShowFOV),
	}
}

func playerTab() []giu.Widget {
	return []giu.Widget{
		giu.Checkbox("WalkSpeed Override", &Settings.WalkSpeedEnabled),
		giu.SliderInt(&Settings.WalkSpeed, 16, 100).Label("WalkSpeed"),
		giu.Label("Effective max = base x 2.0 (min 40)"),
		giu.Checkbox("Apply to Vehicles", &Settings.PlayerVehicle),
		giu.Separator(),
		giu.Checkbox("JumpPower Override", &Settings.JumpPowerEnabled),
		giu.SliderInt(&Settings.JumpPower, 50, 200).Label("JumpPower"),
		giu.Label("Effective max = base x 2.0 (min 100)"),
		giu.Separator(),
		giu.Checkbox("HipHeight Override", &Settings.HipHeightEnabled),
		giu.SliderInt(&Settings.HipHeight, 0, 100).Label("HipHeight"),
		giu.Separator(),
		giu.Checkbox("MaxSlopeAngle Override", &Settings.MaxSlopeEnabled),
		giu.SliderInt(&Settings.MaxSlope, 0, 89).Label("MaxSlopeAngle"),
		giu.Separator(),
		giu.Checkbox("Anti-Sit", &Settings.AntiSit),
		giu.Checkbox("Camera Max Zoom", &Settings.CameraMaxEnabled),
		giu.SliderInt(&Settings.CameraMax, 128, 2000).Label("Camera Zoom"),
	}
}

func RunOverlay(g *Game) {
	screenW, screenH = GetScreenSize()
	if screenW == 0 {
		screenW = 1920
	}
	if screenH == 0 {
		screenH = 1080
	}

	wnd := giu.NewMasterWindow(
		"Plutana-Go",
		screenW,
		screenH,
		giu.MasterWindowFlagsNotResizable|giu.MasterWindowFlagsTransparent,
	)
	lastSaveTime = time.Now()

	wnd.Run(func() {
		RenderFrame(g)
	})
}