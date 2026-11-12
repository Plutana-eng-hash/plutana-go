package main

import (
    "encoding/binary"
    "encoding/json"
    "math"
    "net/http"
    "os"
    "strconv"
    "strings"
    "time"
    "unsafe"

    "golang.org/x/sys/windows"
)

const (
    VERSION          = "1.0"
    TTLGen           = 0.75
    TTLPlayer        = 1.2
    TTLTeam          = 2.5
    TTLChar          = 0.35
    BaseCaptureDelay = 3
    WSRampRate       = 25.0
    JPRampRate       = 100.0
    WSWriterEps      = 0.2
    JPWriterEps      = 0.5
    SafeWSMult       = 2.0
    SafeWSMinCap     = 40.0
    SafeJPMult       = 2.0
    SafeJPMinCap     = 100.0
)

var HTTPClient = &http.Client{Timeout: 5 * time.Second}

type Memory struct {
    Handle windows.Handle
    PID    uint32
    Base   uintptr
}

func OpenMemory(pid uint32) (*Memory, error) {
    h, err := windows.OpenProcess(
        windows.PROCESS_VM_READ|windows.PROCESS_VM_WRITE|windows.PROCESS_VM_OPERATION|windows.PROCESS_QUERY_INFORMATION,
        false, pid,
    )
    if err != nil {
        return nil, err
    }
    return &Memory{Handle: h, PID: pid}, nil
}

func (m *Memory) Close() {
    if m.Handle != 0 {
        windows.CloseHandle(m.Handle)
        m.Handle = 0
    }
}

func (m *Memory) ReadBytes(addr uintptr, size int) []byte {
    if addr == 0 || addr > 0x7FFFFFFFFFFF || size <= 0 {
        return nil
    }
    buf := make([]byte, size)
    var read uintptr
    err := windows.ReadProcessMemory(m.Handle, addr, &buf[0], uintptr(size), &read)
    if err != nil || int(read) != size {
        return nil
    }
    return buf
}

func (m *Memory) WriteBytes(addr uintptr, data []byte) bool {
    if addr == 0 || addr > 0x7FFFFFFFFFFF || len(data) == 0 {
        return false
    }
    var written uintptr
    err := windows.WriteProcessMemory(m.Handle, addr, &data[0], uintptr(len(data)), &written)
    return err == nil && int(written) == len(data)
}

func (m *Memory) ReadU64(addr uintptr) uint64 {
    b := m.ReadBytes(addr, 8)
    if b == nil {
        return 0
    }
    return binary.LittleEndian.Uint64(b)
}

func (m *Memory) ReadI32(addr uintptr) int32 {
    b := m.ReadBytes(addr, 4)
    if b == nil {
        return 0
    }
    return int32(binary.LittleEndian.Uint32(b))
}

func (m *Memory) ReadU8(addr uintptr) byte {
    b := m.ReadBytes(addr, 1)
    if b == nil {
        return 0
    }
    return b[0]
}

func (m *Memory) ReadF32(addr uintptr) float32 {
    b := m.ReadBytes(addr, 4)
    if b == nil {
        return 0
    }
    return math.Float32frombits(binary.LittleEndian.Uint32(b))
}

func (m *Memory) WriteF32(addr uintptr, v float32) bool {
    buf := make([]byte, 4)
    binary.LittleEndian.PutUint32(buf, math.Float32bits(v))
    return m.WriteBytes(addr, buf)
}

func (m *Memory) WriteU8(addr uintptr, v byte) bool {
    return m.WriteBytes(addr, []byte{v})
}

func (m *Memory) ReadRBXString(addr uintptr) string {
    if addr == 0 {
        return ""
    }
    length := m.ReadI32(addr + 0x10)
    if length < 0 || length > 4096 {
        return ""
    }
    if length > 15 {
        ptr := m.ReadU64(addr)
        if ptr == 0 {
            return ""
        }
        data := m.ReadBytes(uintptr(ptr), int(length))
        return string(data)
    }
    data := m.ReadBytes(addr, int(length))
    return string(data)
}

type Offsets struct {
    NameContainer          uintptr
    Children               uintptr
    ChildrenEnd            uintptr
    LocalPlayer            uintptr
    ModelInstance          uintptr
    UserID                 uintptr
    Primitive              uintptr
    Position               uintptr
    Health                 uintptr
    MaxHealth              uintptr
    Team                   uintptr
    WalkSpeed              uintptr
    WalkSpeedCheck         uintptr
    JumpPower              uintptr
    HipHeight              uintptr
    MaxSlopeAngle          uintptr
    Sit                    uintptr
    CameraMaxZoomDistance  uintptr
    CameraMinZoomDistance  uintptr
    VisualEnginePointer    uintptr
    ViewMatrix             uintptr
    FakeDataModelPointer   uintptr
    FakeDataModelToDM      uintptr
}

func DefaultOffsets() *Offsets {
    return &Offsets{
        NameContainer:         0x68,
        Children:              0x70,
        ChildrenEnd:           0x8,
        LocalPlayer:           0x118,
        ModelInstance:         0x2A0,
        UserID:                0xB8,
        Primitive:             0x178,
        Position:              0xD4,
        Health:                0x184,
        MaxHealth:             0x19C,
        Team:                  0x2E0,
        WalkSpeed:             0x1C4,
        WalkSpeedCheck:        0x39C,
        JumpPower:             0x198,
        HipHeight:             0x188,
        MaxSlopeAngle:         0x1A0,
        Sit:                   0x1D3,
        CameraMaxZoomDistance: 0x370,
        CameraMinZoomDistance: 0x374,
        VisualEnginePointer:   0x8942E38,
        ViewMatrix:            0x1B0,
        FakeDataModelPointer:  0x8EB6960,
        FakeDataModelToDM:     0x1F8,
    }
}

func LoadOffsets() *Offsets {
    off := DefaultOffsets()

    data, err := os.ReadFile("offsets.json")
    if err != nil {
        resp, rerr := HTTPClient.Get("https://raw.githubusercontent.com/roqols/omega/main/offsets.json")
        if rerr == nil && resp.StatusCode == 200 {
            buf := make([]byte, 65536)
            n, _ := resp.Body.Read(buf)
            resp.Body.Close()
            data = buf[:n]
        }
    }
    if len(data) == 0 {
        return off
    }

    var raw map[string]string
    if err := json.Unmarshal(data, &raw); err != nil {
        return off
    }

    parse := func(key string, def uintptr) uintptr {
        v, ok := raw[key]
        if !ok || v == "" {
            return def
        }
        v = strings.TrimPrefix(v, "0x")
        n, err := strconv.ParseUint(v, 16, 64)
        if err != nil {
            return def
        }
        return uintptr(n)
    }

    off.NameContainer = parse("NameContainer", off.NameContainer)
    off.Children = parse("Children", off.Children)
    off.ChildrenEnd = parse("ChildrenEnd", off.ChildrenEnd)
    off.LocalPlayer = parse("LocalPlayer", off.LocalPlayer)
    off.ModelInstance = parse("ModelInstance", off.ModelInstance)
    off.UserID = parse("UserId", off.UserID)
    off.Primitive = parse("Primitive", off.Primitive)
    off.Position = parse("Position", off.Position)
    off.Health = parse("Health", off.Health)
    off.MaxHealth = parse("MaxHealth", off.MaxHealth)
    off.Team = parse("Team", off.Team)
    off.WalkSpeed = parse("WalkSpeed", off.WalkSpeed)
    off.WalkSpeedCheck = parse("WalkSpeedCheck", off.WalkSpeedCheck)
    off.JumpPower = parse("JumpPower", off.JumpPower)
    off.HipHeight = parse("HipHeight", off.HipHeight)
    off.MaxSlopeAngle = parse("MaxSlopeAngle", off.MaxSlopeAngle)
    off.Sit = parse("Sit", off.Sit)
    off.CameraMaxZoomDistance = parse("CameraMaxZoomDistance", off.CameraMaxZoomDistance)
    off.CameraMinZoomDistance = parse("CameraMinZoomDistance", off.CameraMinZoomDistance)
    off.VisualEnginePointer = parse("VisualEnginePointer", off.VisualEnginePointer)
    off.ViewMatrix = parse("viewmatrix", off.ViewMatrix)
    off.FakeDataModelPointer = parse("FakeDataModelPointer", off.FakeDataModelPointer)
    off.FakeDataModelToDM = parse("FakeDataModelToDataModel", off.FakeDataModelToDM)
    return off
}

type Game struct {
    mem *Memory
    off *Offsets

    DM      uintptr
    Players uintptr
    VE      uintptr

    nameCache     map[uintptr]cachedStr
    classCache    map[uintptr]cachedStr
    childrenCache map[uintptr]cachedChildren
    charCache     map[uintptr]cachedUptr
    humCache      map[uintptr]cachedUptr
    teamCache     map[uintptr]cachedUptr
    charDataCache map[uintptr]*CharData
    posCache      map[uintptr]cachedPos

    localPlayer     uintptr
    localPlayerTime time.Time
}

type cachedStr struct {
    val string
    at  time.Time
}
type cachedUptr struct {
    val uintptr
    at  time.Time
}
type cachedPos struct {
    x, y, z float32
    at      time.Time
}
type cachedChildren struct {
    val []uintptr
    at  time.Time
}

type CharData struct {
    Parts map[string]uintptr
    R15   bool
    Hum   uintptr
    MaxHP float32
    At    time.Time
}

func NewGame(mem *Memory, off *Offsets) *Game {
    return &Game{
        mem:           mem,
        off:           off,
        nameCache:     make(map[uintptr]cachedStr),
        classCache:    make(map[uintptr]cachedStr),
        childrenCache: make(map[uintptr]cachedChildren),
        charCache:     make(map[uintptr]cachedUptr),
        humCache:      make(map[uintptr]cachedUptr),
        teamCache:     make(map[uintptr]cachedUptr),
        charDataCache: make(map[uintptr]*CharData),
        posCache:      make(map[uintptr]cachedPos),
    }
}

func (g *Game) ProbeGame() bool {
    if g.DM != 0 && g.Players != 0 && g.VE != 0 {
        return true
    }
    fdm := g.mem.ReadU64(g.mem.Base + g.off.FakeDataModelPointer)
    if fdm == 0 || fdm > 0x7FFFFFFFFFFF {
        return false
    }
    dm := g.mem.ReadU64(uintptr(fdm) + g.off.FakeDataModelToDM)
    if dm == 0 || dm > 0x7FFFFFFFFFFF {
        return false
    }
    g.DM = uintptr(dm)
    g.Players = g.FindChildOfClass(g.DM, "Players")
    if g.Players == 0 {
        return false
    }
    ve := g.mem.ReadU64(g.mem.Base + g.off.VisualEnginePointer)
    if ve == 0 || ve > 0x7FFFFFFFFFFF {
        return false
    }
    g.VE = uintptr(ve)
    return true
}

func (g *Game) GetName(inst uintptr) string {
    if inst == 0 {
        return ""
    }
    if c, ok := g.nameCache[inst]; ok && time.Since(c.at) < TTLGen*time.Second {
        return c.val
    }
    ptr := g.mem.ReadU64(inst + g.off.NameContainer)
    if ptr == 0 {
        g.nameCache[inst] = cachedStr{"", time.Now()}
        return ""
    }
    v := g.mem.ReadRBXString(uintptr(ptr))
    v = strings.TrimRight(v, "\x00")
    g.nameCache[inst] = cachedStr{v, time.Now()}
    return v
}

func (g *Game) GetChildren(inst uintptr) []uintptr {
    if inst == 0 {
        return nil
    }
    if c, ok := g.childrenCache[inst]; ok && time.Since(c.at) < TTLGen*time.Second {
        return c.val
    }
    start := g.mem.ReadU64(inst + g.off.Children)
    if start == 0 {
        g.childrenCache[inst] = cachedChildren{nil, time.Now()}
        return nil
    }
    end := g.mem.ReadU64(uintptr(start) + g.off.ChildrenEnd)
    beg := g.mem.ReadU64(uintptr(start))
    if beg == 0 || end <= beg {
        g.childrenCache[inst] = cachedChildren{nil, time.Now()}
        return nil
    }
    diff := end - beg
    if diff > 65536 || diff%16 != 0 {
        g.childrenCache[inst] = cachedChildren{nil, time.Now()}
        return nil
    }
    count := int(diff / 16)
    result := make([]uintptr, 0, count)
    for i := 0; i < count; i++ {
        ptr := g.mem.ReadU64(uintptr(beg) + uintptr(i)*16)
        if ptr != 0 {
            result = append(result, uintptr(ptr))
        }
    }
    g.childrenCache[inst] = cachedChildren{result, time.Now()}
    return result
}

func (g *Game) ClassName(inst uintptr) string {
    if inst == 0 {
        return ""
    }
    if c, ok := g.classCache[inst]; ok && time.Since(c.at) < TTLGen*time.Second {
        return c.val
    }
    p := g.mem.ReadU64(inst + 0x18)
    if p == 0 {
        g.classCache[inst] = cachedStr{"", time.Now()}
        return ""
    }
    p2 := g.mem.ReadU64(uintptr(p) + 0x8)
    if p2 == 0 {
        g.classCache[inst] = cachedStr{"", time.Now()}
        return ""
    }
    fl := g.mem.ReadU64(uintptr(p2) + 0x18)
    if fl == 0x1F {
        p2 = g.mem.ReadU64(uintptr(p2))
    }
    v := g.mem.ReadRBXString(uintptr(p2))
    g.classCache[inst] = cachedStr{v, time.Now()}
    return v
}

func (g *Game) FindChildOfClass(inst uintptr, cls string) uintptr {
    for _, c := range g.GetChildren(inst) {
        if g.ClassName(c) == cls {
            return c
        }
    }
    return 0
}

func (g *Game) LocalPlayer() uintptr {
    if time.Since(g.localPlayerTime) < TTLPlayer*time.Second && g.localPlayer != 0 {
        return g.localPlayer
    }
    v := g.mem.ReadU64(g.Players + g.off.LocalPlayer)
    g.localPlayer = uintptr(v)
    g.localPlayerTime = time.Now()
    return g.localPlayer
}

func (g *Game) Character(player uintptr) uintptr {
    if player == 0 {
        return 0
    }
    if c, ok := g.charCache[player]; ok && time.Since(c.at) < TTLChar*time.Second {
        return c.val
    }
    v := uintptr(g.mem.ReadU64(player + g.off.ModelInstance))
    g.charCache[player] = cachedUptr{v, time.Now()}
    return v
}

func (g *Game) Humanoid(char uintptr) uintptr {
    if char == 0 {
        return 0
    }
    if c, ok := g.humCache[char]; ok && time.Since(c.at) < TTLGen*time.Second {
        return c.val
    }
    v := g.FindChildOfClass(char, "Humanoid")
    g.humCache[char] = cachedUptr{v, time.Now()}
    return v
}

func (g *Game) Team(player uintptr) uintptr {
    if player == 0 {
        return 0
    }
    if c, ok := g.teamCache[player]; ok && time.Since(c.at) < TTLTeam*time.Second {
        return c.val
    }
    v := uintptr(g.mem.ReadU64(player + g.off.Team))
    g.teamCache[player] = cachedUptr{v, time.Now()}
    return v
}

func (g *Game) Pos(part uintptr) (float32, float32, float32, bool) {
    if part == 0 {
        return 0, 0, 0, false
    }
    if c, ok := g.posCache[part]; ok && time.Since(c.at) < TTLGen*time.Second {
        return c.x, c.y, c.z, true
    }
    prim := g.mem.ReadU64(part + g.off.Primitive)
    if prim == 0 {
        return 0, 0, 0, false
    }
    b := g.mem.ReadBytes(uintptr(prim)+g.off.Position, 12)
    if b == nil {
        return 0, 0, 0, false
    }
    x := math.Float32frombits(binary.LittleEndian.Uint32(b[0:4]))
    y := math.Float32frombits(binary.LittleEndian.Uint32(b[4:8]))
    z := math.Float32frombits(binary.LittleEndian.Uint32(b[8:12]))
    g.posCache[part] = cachedPos{x, y, z, time.Now()}
    return x, y, z, true
}

func (g *Game) CharData(char uintptr) *CharData {
    if char == 0 {
        return nil
    }
    if c, ok := g.charDataCache[char]; ok && time.Since(c.At) < TTLGen*time.Second {
        if g.mem.ReadF32(c.Hum+g.off.Health) <= 0 {
            delete(g.charDataCache, char)
            return nil
        }
        return c
    }
    kids := g.GetChildren(char)
    if len(kids) == 0 {
        return nil
    }
    names := make([]string, len(kids))
    for i, k := range kids {
        names[i] = g.GetName(k)
    }
    r15 := false
    for _, n := range names {
        if n == "UpperTorso" {
            r15 = true
            break
        }
    }
    var parts map[string]uintptr
    if r15 {
        parts = make(map[string]uintptr)
        for i, n := range names {
            switch n {
            case "Head", "UpperTorso", "LowerTorso",
                "LeftUpperArm", "LeftLowerArm", "LeftHand",
                "RightUpperArm", "RightLowerArm", "RightHand",
                "LeftUpperLeg", "LeftLowerLeg", "LeftFoot",
                "RightUpperLeg", "RightLowerLeg", "RightFoot":
                parts[n] = kids[i]
            }
        }
    } else {
        parts = make(map[string]uintptr)
        for i, n := range names {
            switch n {
            case "Head", "Torso", "Left Arm", "Right Arm", "Left Leg", "Right Leg":
                parts[n] = kids[i]
            }
        }
    }
    hum := g.Humanoid(char)
    if hum == 0 || len(parts) < 2 {
        return nil
    }
    mhp := g.mem.ReadF32(hum + g.off.MaxHealth)
    if mhp <= 0 {
        return nil
    }
    cd := &CharData{Parts: parts, R15: r15, Hum: hum, MaxHP: mhp, At: time.Now()}
    g.charDataCache[char] = cd
    return cd
}

type Matrix4 [4][4]float32

func (g *Game) WorldToScreen(vm Matrix4, x, y, z, screenW, screenH float32) (float32, float32, bool) {
    cw := vm[3][0]*x + vm[3][1]*y + vm[3][2]*z + vm[3][3]
    if cw <= 0.001 {
        return 0, 0, false
    }
    cx := vm[0][0]*x + vm[0][1]*y + vm[0][2]*z + vm[0][3]
    cy := vm[1][0]*x + vm[1][1]*y + vm[1][2]*z + vm[1][3]
    ndcX := cx / cw
    ndcY := cy / cw
    sx := (ndcX + 1.0) * 0.5 * screenW
    sy := (1.0 - ndcY) * 0.5 * screenH
    if ndcX < -1.05 || ndcX > 1.05 || ndcY < -1.05 || ndcY > 1.05 {
        return 0, 0, false
    }
    return sx, sy, true
}

func (g *Game) ViewMatrix() Matrix4 {
    var vm Matrix4
    raw := g.mem.ReadBytes(g.VE+g.off.ViewMatrix, 64)
    if raw == nil {
        return vm
    }
    for i := 0; i < 16; i++ {
        vm[i/4][i%4] = math.Float32frombits(binary.LittleEndian.Uint32(raw[i*4 : (i+1)*4]))
    }
    return vm
}

var _ = unsafe.Sizeof(0)

type PlayerState struct {
    Hum           uintptr
    BaseWS        float32
    BaseWSChk     float32
    BaseJP        float32
    BaseHip       float32
    BaseSlope     float32
    BaseCamMax    float32
    BaseCamMin    float32
    CurWS         float32
    CurJP         float32
    WasWS         bool
    WasJP         bool
    WasHip        bool
    WasSlope      bool
    WasCam        bool
    LastWriteWS   float32
    LastWriteWSChk float32
    LastWriteJP   float32
    LastHip       float32
    LastSlope     float32
    LastCamMax    float32
    LastCamMin    float32
    Frames        int
    LastT         time.Time
}

var pstate = PlayerState{
    BaseWS: -1, BaseWSChk: -1, BaseJP: -1, BaseHip: -1, BaseSlope: -1,
    BaseCamMax: -1, BaseCamMin: -1, CurWS: -1, CurJP: -1,
    LastWriteWS: -999, LastWriteWSChk: -999, LastWriteJP: -999,
    LastHip: -999, LastSlope: -999, LastCamMax: -999, LastCamMin: -999,
}

func ResetPState(hum uintptr) {
    pstate = PlayerState{
        Hum:           hum,
        BaseWS:        -1, BaseWSChk: -1, BaseJP: -1, BaseHip: -1, BaseSlope: -1,
        BaseCamMax:    -1, BaseCamMin: -1, CurWS: -1, CurJP: -1,
        LastWriteWS:   -999, LastWriteWSChk: -999, LastWriteJP: -999,
        LastHip:       -999, LastSlope: -999, LastCamMax: -999, LastCamMin: -999,
    }
}

func ApplyPlayer(g *Game, lp, hum uintptr) {
    if hum == 0 {
        return
    }
    p := &pstate
    now := time.Now()
    var dt float64 = 0.016
    if !p.LastT.IsZero() {
        dt = now.Sub(p.LastT).Seconds()
        if dt > 0.25 {
            dt = 0.25
        }
        if dt < 0.001 {
            dt = 0.001
        }
    }
    p.LastT = now

    if p.Hum != hum {
        ResetPState(hum)
        p = &pstate
    }
    p.Frames++

    if p.Frames >= BaseCaptureDelay {
        if p.BaseWS < 0 {
            bw := g.mem.ReadF32(hum + g.off.WalkSpeed)
            if bw < 5 || bw > 200 {
                bw = 16
            }
            p.BaseWS = bw
            p.CurWS = bw
            p.LastWriteWS = -999
        }
        if p.BaseWSChk < 0 {
            raw := g.mem.ReadBytes(hum+g.off.WalkSpeedCheck, 4)
            if raw != nil {
                v := math.Float32frombits(binary.LittleEndian.Uint32(raw))
                if !math.IsNaN(float64(v)) && !math.IsInf(float64(v), 0) && v >= 0 && v <= 10000 {
                    p.BaseWSChk = v
                } else {
                    p.BaseWSChk = p.BaseWS
                }
            } else {
                p.BaseWSChk = p.BaseWS
            }
            p.LastWriteWSChk = -999
        }
        if p.BaseJP < 0 {
            bj := g.mem.ReadF32(hum + g.off.JumpPower)
            if bj < 20 || bj > 500 {
                bj = 50
            }
            p.BaseJP = bj
            p.CurJP = bj
            p.LastWriteJP = -999
        }
        if p.BaseHip < 0 {
            bh := g.mem.ReadF32(hum + g.off.HipHeight)
            if bh < -100 || bh > 100 {
                bh = 2
            }
            p.BaseHip = bh
        }
        if p.BaseSlope < 0 {
            bs := g.mem.ReadF32(hum + g.off.MaxSlopeAngle)
            if bs < 0 || bs > 90 {
                bs = 89
            }
            p.BaseSlope = bs
        }
        if p.BaseCamMax < 0 {
            bc := g.mem.ReadF32(lp + g.off.CameraMaxZoomDistance)
            if bc <= 0 {
                bc = 128
            }
            p.BaseCamMax = bc
        }
        if p.BaseCamMin < 0 {
            bcm := g.mem.ReadF32(lp + g.off.CameraMinZoomDistance)
            if bcm < 0 {
                bcm = 0.5
            }
            p.BaseCamMin = bcm
        }
    }

    if p.BaseWS < 0 {
        return
    }

    isSitting := g.mem.ReadU8(hum+g.off.Sit) != 0
    allowMove := Settings.PlayerVehicle || !isSitting

    wantWS := Settings.WalkSpeedEnabled && allowMove
    var targetWS float32 = -1
    if wantWS {
        targetWS = float32(Settings.WalkSpeed)
        p.WasWS = true
    } else if p.WasWS {
        targetWS = p.BaseWS
    }

    if targetWS >= 0 {
        capWS := p.BaseWS * SafeWSMult
        if capWS < SafeWSMinCap {
            capWS = SafeWSMinCap
        }
        if targetWS > capWS {
            targetWS = capWS
        }
        cur := p.CurWS
        ramp := float32(WSRampRate * dt)
        if cur < targetWS {
            cur += ramp
            if cur > targetWS {
                cur = targetWS
            }
        } else if cur > targetWS {
            cur -= ramp
            if cur < targetWS {
                cur = targetWS
            }
        }
        p.CurWS = cur

        actualWS := g.mem.ReadF32(hum + g.off.WalkSpeed)
        if math.Abs(float64(actualWS-cur)) > WSWriterEps {
            g.mem.WriteF32(hum+g.off.WalkSpeed, cur)
            p.LastWriteWS = cur
        }

        if p.BaseWSChk >= 0 {
            rawChk := g.mem.ReadBytes(hum+g.off.WalkSpeedCheck, 4)
            if rawChk != nil {
                curChk := math.Float32frombits(binary.LittleEndian.Uint32(rawChk))
                if !math.IsNaN(float64(curChk)) && !math.IsInf(float64(curChk), 0) && curChk >= 0 && curChk <= 10000 {
                    var wantChk float32
                    if wantWS {
                        wantChk = cur
                    } else {
                        wantChk = p.BaseWSChk
                    }
                    if math.Abs(float64(curChk-wantChk)) > WSWriterEps {
                        g.mem.WriteF32(hum+g.off.WalkSpeedCheck, wantChk)
                        p.LastWriteWSChk = wantChk
                    }
                }
            }
        }

        if !wantWS && math.Abs(float64(cur-p.BaseWS)) < WSWriterEps {
            p.WasWS = false
        }
    }

    wantJP := Settings.JumpPowerEnabled
    var targetJP float32 = -1
    if wantJP {
        targetJP = float32(Settings.JumpPower)
        p.WasJP = true
    } else if p.WasJP {
        targetJP = p.BaseJP
    }

    if targetJP >= 0 {
        capJP := p.BaseJP * SafeJPMult
        if capJP < SafeJPMinCap {
            capJP = SafeJPMinCap
        }
        if targetJP > capJP {
            targetJP = capJP
        }
        curj := p.CurJP
        rampj := float32(JPRampRate * dt)
        if curj < targetJP {
            curj += rampj
            if curj > targetJP {
                curj = targetJP
            }
        } else if curj > targetJP {
            curj -= rampj
            if curj < targetJP {
                curj = targetJP
            }
        }
        p.CurJP = curj
        if math.Abs(float64(curj-p.LastWriteJP)) > JPWriterEps {
            g.mem.WriteF32(hum+g.off.JumpPower, curj)
            p.LastWriteJP = curj
        }
        if !wantJP && math.Abs(float64(curj-p.BaseJP)) < JPWriterEps {
            p.WasJP = false
        }
    }

    if Settings.HipHeightEnabled {
        v := float32(Settings.HipHeight)
        if math.Abs(float64(v-p.LastHip)) > 0.01 {
            g.mem.WriteF32(hum+g.off.HipHeight, v)
            p.LastHip = v
        }
        p.WasHip = true
    } else if p.WasHip {
        g.mem.WriteF32(hum+g.off.HipHeight, p.BaseHip)
        p.LastHip = -999
        p.WasHip = false
    }

    if Settings.MaxSlopeEnabled {
        v := float32(Settings.MaxSlope)
        if math.Abs(float64(v-p.LastSlope)) > 0.01 {
            g.mem.WriteF32(hum+g.off.MaxSlopeAngle, v)
            p.LastSlope = v
        }
        p.WasSlope = true
    } else if p.WasSlope {
        g.mem.WriteF32(hum+g.off.MaxSlopeAngle, p.BaseSlope)
        p.LastSlope = -999
        p.WasSlope = false
    }

    if Settings.AntiSit && isSitting {
        g.mem.WriteU8(hum+g.off.Sit, 0)
    }

    if Settings.CameraMaxEnabled {
        vmax := float32(Settings.CameraMax)
        vmin := float32(0.5)
        if math.Abs(float64(vmax-p.LastCamMax)) > 0.01 {
            g.mem.WriteF32(lp+g.off.CameraMaxZoomDistance, vmax)
            p.LastCamMax = vmax
        }
        if math.Abs(float64(vmin-p.LastCamMin)) > 0.01 {
            g.mem.WriteF32(lp+g.off.CameraMinZoomDistance, vmin)
            p.LastCamMin = vmin
        }
        p.WasCam = true
    } else if p.WasCam {
        g.mem.WriteF32(lp+g.off.CameraMaxZoomDistance, p.BaseCamMax)
        g.mem.WriteF32(lp+g.off.CameraMinZoomDistance, p.BaseCamMin)
        p.LastCamMax = -999
        p.LastCamMin = -999
        p.WasCam = false
    }
}

var aimKeyVK = map[string]uint32{
    "RMB": 0x02, "LMB": 0x01, "Shift": 0x10, "Alt": 0x12, "Ctrl": 0x11,
    "E": 0x45, "Q": 0x51, "C": 0x43, "Z": 0x5A, "X": 0x58,
}

func GetAsyncKeyState(vk uint32) bool {
    user32 := windows.NewLazyDLL("user32.dll")
    proc := user32.NewProc("GetAsyncKeyState")
    r, _, _ := proc.Call(uintptr(vk))
    return int16(r&0xFFFF) < 0
}

func DoAim(g *Game, players []uintptr, lp, lt uintptr, vm Matrix4, screenW, screenH int) {
    if !Settings.AimEnabled {
        return
    }
    vk, ok := aimKeyVK[Settings.AimKey]
    if !ok || !GetAsyncKeyState(vk) {
        return
    }

    cx := float32(screenW) / 2
    cy := float32(screenH) / 2
    fov := float32(Settings.FOVRadius)
    bestD2 := fov * fov
    found := false
    var tx, ty float32

    for _, player := range players {
        if player == lp {
            continue
        }
        if Settings.AimIgnoreTeam && lt != 0 && g.Team(player) == lt {
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
        var part uintptr
        switch Settings.AimTarget {
        case "Head":
            part = cd.Parts["Head"]
        case "Torso":
            part = cd.Parts["UpperTorso"]
            if part == 0 {
                part = cd.Parts["Torso"]
            }
            if part == 0 {
                part = cd.Parts["LowerTorso"]
            }
        default:
            for _, n := range []string{"Head", "UpperTorso", "LowerTorso", "Torso"} {
                if p, ok := cd.Parts[n]; ok {
                    part = p
                    break
                }
            }
        }
        if part == 0 {
            continue
        }
        px, py, pz, ok := g.Pos(part)
        if !ok {
            continue
        }
        sx, sy, vis := g.WorldToScreen(vm, px, py, pz, float32(screenW), float32(screenH))
        if !vis {
            continue
        }
        d2 := (sx-cx)*(sx-cx) + (sy-cy)*(sy-cy)
        if d2 < bestD2 {
            bestD2 = d2
            tx, ty = sx, sy
            found = true
        }
    }

    if !found {
        return
    }
    sm := float32(Settings.AimSmooth)
    if sm < 1 {
        sm = 1
    }
    dx := int32(math.Round(float64((tx - cx) / sm)))
    dy := int32(math.Round(float64((ty - cy) / sm)))
    if dx == 0 && dy == 0 {
        return
    }
    user32 := windows.NewLazyDLL("user32.dll")
    proc := user32.NewProc("mouse_event")
    proc.Call(0x0001, uintptr(dx), uintptr(dy), 0, 0)
}