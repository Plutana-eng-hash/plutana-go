package main

import (
	"encoding/json"
	"os"
	"path/filepath"
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
	ESPEnabled: true, Box: true, FillAlpha: 45, ShowFPS: false, TeamCheck: true,
	AimKey: "RMB", AimSmooth: 5, AimTarget: "Head", AimIgnoreTeam: true,
	ShowFOV: true, FOVRadius: 150, WalkSpeed: 16, JumpPower: 50, PlayerVehicle: true,
	HipHeight: 2, MaxSlope: 89, CameraMax: 128,
	ColBox:    [4]float32{1, 1, 1, 1},
	ColDot:    [4]float32{1, 0.24, 0.24, 1},
	ColSkel:   [4]float32{1, 0.78, 0, 1},
	ColFOV:    [4]float32{1, 1, 1, 0.78},
	ColName:   [4]float32{1, 1, 1, 1},
	ColTracer: [4]float32{1, 1, 1, 1},
	ColDist:   [4]float32{1, 1, 1, 1},
	ColHead:   [4]float32{1, 0.31, 0.31, 1},
	ColHealth: [4]float32{1, 1, 1, 1},
}

func settingsFilePath() string {
	exe, _ := os.Executable()
	return filepath.Join(filepath.Dir(exe), "settings.json")
}

func loadSettingsFromDisk() {
	data, err := os.ReadFile(settingsFilePath())
	if err != nil {
		return
	}
	json.Unmarshal(data, &Settings)
}

func saveSettingsToDisk() {
	data, err := json.MarshalIndent(Settings, "", "  ")
	if err != nil {
		return
	}
	os.WriteFile(settingsFilePath(), data, 0644)
}