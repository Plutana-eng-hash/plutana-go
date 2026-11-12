package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

var (
	overlayCmd   *exec.Cmd
	overlayMu    sync.Mutex
	launchBtn    *widget.Button
	statusLabel  *widget.Label
	mainWindow   fyne.Window
)

func runLauncherMode() {
	loadSettingsFromDisk()

	a := app.NewWithID("plutana.go")
	a.Settings().SetTheme(theme.DarkTheme())
	mainWindow = a.NewWindow("Plutana")
	mainWindow.Resize(fyne.NewSize(900, 660))
	mainWindow.CenterOnScreen()

	header := widget.NewLabelWithStyle("Plutana", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	version := widget.NewLabelWithStyle("v1.0 · Go", fyne.TextAlignTrailing, fyne.TextStyle{Italic: true})

	top := container.NewBorder(
		nil, widget.NewSeparator(),
		header, version,
	)

	tabs := container.NewAppTabs(
		container.NewTabItem("ESP", buildESPTab()),
		container.NewTabItem("AIM", buildAimTab()),
		container.NewTabItem("PLAYER", buildPlayerTab()),
	)

	statusLabel = widget.NewLabel("Roblox: not running")
	launchBtn = widget.NewButton("Launch Roblox", onLaunchClicked)
	launchBtn.Importance = widget.HighImportance

	bottom := container.NewBorder(nil, nil, statusLabel, launchBtn)

	mainWindow.SetContent(container.NewBorder(top, bottom, nil, nil, tabs))
	mainWindow.SetCloseIntercept(func() {
		stopOverlayChild()
		mainWindow.Close()
		a.Quit()
	})

	go monitorStatus()
	mainWindow.ShowAndRun()
}

func onLaunchClicked() {
	rp, _ := findProcessID("RobloxPlayerBeta.exe")
	if rp == 0 {
		dialog.ShowInformation("Roblox not running",
			"Start Roblox and join a game first, then click Launch Overlay again.",
			mainWindow)
		return
	}
	startOverlayChild()
}

func startOverlayChild() {
	overlayMu.Lock()
	defer overlayMu.Unlock()
	if overlayCmd != nil {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		dialog.ShowError(err, mainWindow)
		return
	}
	saveSettingsToDisk()
	cmd := exec.Command(exe, "--overlay")
	cmd.Dir = filepath.Dir(exe)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
	if err := cmd.Start(); err != nil {
		dialog.ShowError(fmt.Errorf("failed to start overlay: %v", err), mainWindow)
		return
	}
	overlayCmd = cmd
	go func(c *exec.Cmd) {
		c.Wait()
		overlayMu.Lock()
		if overlayCmd == c {
			overlayCmd = nil
		}
		overlayMu.Unlock()
	}(cmd)
}

func stopOverlayChild() {
	overlayMu.Lock()
	defer overlayMu.Unlock()
	if overlayCmd != nil && overlayCmd.Process != nil {
		overlayCmd.Process.Kill()
		overlayCmd = nil
	}
}

func monitorStatus() {
	for {
		rp, _ := findProcessID("RobloxPlayerBeta.exe")
		overlayMu.Lock()
		op := overlayCmd != nil
		overlayMu.Unlock()

		switch {
		case rp != 0 && op:
			statusLabel.SetText("Roblox: running  ·  Overlay: active")
			launchBtn.SetText("Overlay Running")
			launchBtn.Disable()
		case rp != 0:
			statusLabel.SetText("Roblox: running  ·  Overlay: stopped")
			launchBtn.SetText("Launch Overlay")
			launchBtn.Enable()
		default:
			statusLabel.SetText("Roblox: not running")
			launchBtn.SetText("Start Roblox First")
			launchBtn.Disable()
		}
		time.Sleep(1 * time.Second)
	}
}

func chk(label string, value *bool) *widget.Check {
	c := widget.NewCheck(label, func(v bool) {
		*value = v
		saveSettingsToDisk()
	})
	c.SetChecked(*value)
	return c
}

func sliderI32(label string, min, max float64, value *int32) fyne.CanvasObject {
	lbl := widget.NewLabel(fmt.Sprintf("%s: %d", label, *value))
	sl := widget.NewSlider(min, max)
	sl.Step = 1
	sl.SetValue(float64(*value))
	sl.OnChanged = func(v float64) {
		*value = int32(v)
		lbl.SetText(fmt.Sprintf("%s: %d", label, int32(v)))
		saveSettingsToDisk()
	}
	return container.NewVBox(lbl, sl)
}

func comboStr(label string, options []string, value *string) fyne.CanvasObject {
	lbl := widget.NewLabel(label)
	sel := widget.NewSelect(options, func(v string) {
		*value = v
		saveSettingsToDisk()
	})
	sel.SetSelected(*value)
	return container.NewVBox(lbl, sel)
}

func cardBox(title string, body fyne.CanvasObject) fyne.CanvasObject {
	hdr := widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	return container.NewBorder(
		container.NewVBox(hdr, widget.NewSeparator()),
		nil, nil, nil,
		container.NewVScroll(body),
	)
}

func buildESPTab() fyne.CanvasObject {
	master := cardBox("MASTER", container.NewVBox(
		chk("Enable ESP", &Settings.ESPEnabled),
	))
	visuals := cardBox("VISUALS", container.NewVBox(
		chk("Box ESP", &Settings.Box),
		chk("Corner Box", &Settings.CornerBox),
		chk("Box Fill", &Settings.BoxFill),
		sliderI32("Fill Alpha", 0, 255, &Settings.FillAlpha),
		chk("Dot ESP", &Settings.Dot),
		chk("Head Dot", &Settings.HeadDot),
		chk("Head Circle", &Settings.HeadCircle),
		chk("Skeleton", &Settings.Skeleton),
		chk("Skeleton Dots", &Settings.SkeletonDots),
		chk("Name ESP", &Settings.Name),
	))
	info := cardBox("INFO", container.NewVBox(
		chk("Health Bar", &Settings.Health),
		chk("Health Text", &Settings.HealthText),
		chk("Health Percent", &Settings.HealthPercent),
		chk("Distance", &Settings.Distance),
		chk("Distance in Meters", &Settings.DistanceInMeters),
	))
	filters := cardBox("FILTERS", container.NewVBox(
		chk("Team Check", &Settings.TeamCheck),
		sliderI32("Max Distance", 0, 2000, &Settings.MaxDistance),
	))
	overlay := cardBox("OVERLAY", container.NewVBox(
		chk("Tracers", &Settings.Tracers),
		chk("Tracer from Mouse", &Settings.TracerFromMouse),
		chk("Show FPS", &Settings.ShowFPS),
	))
	left := container.NewVBox(master, visuals)
	right := container.NewVBox(info, filters, overlay)
	return container.NewGridWithColumns(2, left, right)
}

func buildAimTab() fyne.CanvasObject {
	aim := cardBox("AIMBOT", container.NewVBox(
		chk("Enable Aimbot", &Settings.AimEnabled),
		comboStr("Aim Key", []string{"RMB", "LMB", "Shift", "Alt", "Ctrl", "E", "Q", "C", "Z", "X"}, &Settings.AimKey),
		comboStr("Target Part", []string{"Head", "Torso", "Closest"}, &Settings.AimTarget),
		chk("Ignore Teammates", &Settings.AimIgnoreTeam),
		sliderI32("Smoothness", 1, 20, &Settings.AimSmooth),
		sliderI32("Aim Max Distance", 0, 2000, &Settings.AimMaxDistance),
	))
	fov := cardBox("FOV", container.NewVBox(
		sliderI32("FOV Radius", 20, 600, &Settings.FOVRadius),
		chk("Show FOV Circle", &Settings.ShowFOV),
	))
	return container.NewGridWithColumns(2,
		container.NewVBox(aim),
		container.NewVBox(fov),
	)
}

func buildPlayerTab() fyne.CanvasObject {
	speed := cardBox("SPEED", container.NewVBox(
		chk("WalkSpeed Override", &Settings.WalkSpeedEnabled),
		sliderI32("WalkSpeed", 16, 100, &Settings.WalkSpeed),
		chk("Apply to Vehicles", &Settings.PlayerVehicle),
	))
	jump := cardBox("JUMP & MOVE", container.NewVBox(
		chk("JumpPower Override", &Settings.JumpPowerEnabled),
		sliderI32("JumpPower", 50, 200, &Settings.JumpPower),
		chk("HipHeight Override", &Settings.HipHeightEnabled),
		sliderI32("HipHeight", 0, 100, &Settings.HipHeight),
		chk("MaxSlopeAngle Override", &Settings.MaxSlopeEnabled),
		sliderI32("MaxSlopeAngle", 0, 89, &Settings.MaxSlope),
	))
	misc := cardBox("MISC", container.NewVBox(
		chk("Anti-Sit", &Settings.AntiSit),
		chk("Camera Max Zoom", &Settings.CameraMaxEnabled),
		sliderI32("Camera Zoom", 128, 2000, &Settings.CameraMax),
	))
	return container.NewGridWithColumns(2,
		container.NewVBox(speed),
		container.NewVBox(jump, misc),
	)
}

var _ = json.Marshal