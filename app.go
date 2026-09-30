package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"ntrip-tester/internal/analyzer"
	"ntrip-tester/internal/ntrip"
)

type Stats struct {
	analyzer.Snapshot
	State        string  `json:"state"`
	StateMsg     string  `json:"stateMsg"`
	Host         string  `json:"host"`
	Mount        string  `json:"mount"`
	ConnectedFor float64 `json:"connectedFor"`
	Reconnects   int     `json:"reconnects"`
	GGASent      int     `json:"ggaSent"`
	LastGGA      string  `json:"lastGga"`
	Recording    bool    `json:"recording"`
	RecordPath   string  `json:"recordPath"`
	RecordBytes  int64   `json:"recordBytes"`
}

type LogEntry struct {
	Time    string `json:"time"`
	Level   string `json:"level"`
	Message string `json:"message"`
}

type App struct {
	ctx       context.Context
	an        *analyzer.Analyzer
	sessionID atomic.Int64

	mu          sync.Mutex
	cancel      context.CancelFunc
	state       string
	stateMsg    string
	host        string
	mount       string
	connectedAt time.Time
	reconnects  int
	ggaSent     int
	lastGGA     string

	recMu    sync.Mutex
	rec      *os.File
	recPath  string
	recBytes int64
}

func NewApp() *App {
	return &App{an: analyzer.New(), state: "idle", stateMsg: "未连接"}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	go a.loop(ctx)
}

func (a *App) shutdown(ctx context.Context) {
	a.Disconnect()
	a.StopRecording()
}

func (a *App) loop(ctx context.Context) {
	statsT := time.NewTicker(time.Second)
	defer statsT.Stop()
	consoleT := time.NewTicker(250 * time.Millisecond)
	defer consoleT.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-statsT.C:
			a.an.Tick()
			runtime.EventsEmit(a.ctx, "stats", a.stats())
		case <-consoleT.C:
			if lines := a.an.DrainConsole(); len(lines) > 0 {
				runtime.EventsEmit(a.ctx, "console", lines)
			}
		}
	}
}

func (a *App) stats() Stats {
	s := Stats{Snapshot: a.an.Snapshot()}
	a.mu.Lock()
	s.State, s.StateMsg, s.Host, s.Mount = a.state, a.stateMsg, a.host, a.mount
	if a.state == "connected" && !a.connectedAt.IsZero() {
		s.ConnectedFor = time.Since(a.connectedAt).Seconds()
	}
	s.Reconnects, s.GGASent, s.LastGGA = a.reconnects, a.ggaSent, a.lastGGA
	a.mu.Unlock()
	a.recMu.Lock()
	s.Recording, s.RecordPath, s.RecordBytes = a.rec != nil, a.recPath, a.recBytes
	a.recMu.Unlock()
	return s
}

func (a *App) log(level, format string, args ...any) {
	if a.ctx == nil {
		return
	}
	runtime.EventsEmit(a.ctx, "log", LogEntry{
		Time:    time.Now().Format("15:04:05"),
		Level:   level,
		Message: fmt.Sprintf(format, args...),
	})
}

// FetchSourceTable retrieves the caster sourcetable.
func (a *App) FetchSourceTable(cfg ntrip.Config) (*ntrip.SourceTable, error) {
	cfg.Normalize()
	a.log("info", "正在获取 %s:%d 的源列表…", cfg.Host, cfg.Port)
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	st, err := ntrip.FetchSourceTable(ctx, cfg)
	if err != nil {
		return nil, err
	}
	a.log("ok", "源列表获取成功：%d 个挂载点", len(st.Mounts))
	return st, nil
}

// Connect starts streaming from the configured mountpoint, replacing any existing session.
func (a *App) Connect(cfg ntrip.Config) error {
	cfg.Normalize()
	if cfg.Host == "" {
		return errors.New("请填写 Caster 地址")
	}
	if cfg.Mountpoint == "" {
		return errors.New("请填写或选择挂载点")
	}
	a.Disconnect()
	a.an.Reset()
	a.an.SetReference(cfg.Lat, cfg.Lon, cfg.HasPosition())

	ctx, cancel := context.WithCancel(a.ctx)
	id := a.sessionID.Add(1)
	a.mu.Lock()
	a.cancel = cancel
	a.host = fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	a.mount = cfg.Mountpoint
	a.reconnects, a.ggaSent, a.lastGGA = 0, 0, ""
	a.connectedAt = time.Time{}
	a.mu.Unlock()

	if cfg.GGAInterval > 0 && !cfg.HasPosition() {
		a.log("warn", "已设置 GGA 发送间隔但未填写经纬度，将不发送 GGA")
	}

	client := ntrip.NewClient(cfg, ntrip.Handler{
		OnData: func(b []byte) {
			if a.sessionID.Load() != id {
				return
			}
			a.an.Feed(b)
			a.record(b)
		},
		OnState: func(state, msg string) { a.setState(id, state, msg) },
		OnGGA: func(s string) {
			a.mu.Lock()
			if a.sessionID.Load() == id {
				a.ggaSent++
				a.lastGGA = s
			}
			a.mu.Unlock()
			a.log("gga", "GGA → %s", s)
		},
	})
	go client.Run(ctx)
	return nil
}

func (a *App) setState(id int64, state, msg string) {
	a.mu.Lock()
	if id != a.sessionID.Load() {
		a.mu.Unlock()
		return
	}
	a.state, a.stateMsg = state, msg
	if state == "connected" {
		a.connectedAt = time.Now()
	} else {
		a.connectedAt = time.Time{}
	}
	if state == "reconnecting" {
		a.reconnects++
	}
	a.mu.Unlock()

	level := map[string]string{"connected": "ok", "error": "error", "reconnecting": "warn"}[state]
	if level == "" {
		level = "info"
	}
	a.log(level, "%s", msg)
	runtime.EventsEmit(a.ctx, "state", map[string]string{"state": state, "message": msg})
}

// Disconnect stops the current session, if any.
func (a *App) Disconnect() {
	a.mu.Lock()
	cancel := a.cancel
	a.cancel = nil
	if cancel != nil {
		a.sessionID.Add(1)
		a.state, a.stateMsg = "idle", "已断开"
		a.connectedAt = time.Time{}
	}
	a.mu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	a.log("info", "已断开连接")
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "state", map[string]string{"state": "idle", "message": "已断开"})
	}
}

// ResetStats clears all accumulated statistics.
func (a *App) ResetStats() {
	a.an.Reset()
	a.log("info", "统计已清空")
}

// SetReference updates the rover reference position used for baseline distance.
func (a *App) SetReference(lat, lon, alt float64) {
	a.an.SetReference(lat, lon, lat != 0 || lon != 0)
}

// StartRecording asks for a file and records the raw stream into it.
func (a *App) StartRecording() (string, error) {
	a.mu.Lock()
	mount := a.mount
	a.mu.Unlock()
	if mount == "" {
		mount = "stream"
	}
	safe := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`\/:*?"<>|`, r) {
			return '_'
		}
		return r
	}, mount)
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "保存原始 RTCM 数据",
		DefaultFilename: fmt.Sprintf("ntrip-%s-%s.rtcm3", safe, time.Now().Format("20060102-150405")),
		Filters:         []runtime.FileFilter{{DisplayName: "RTCM3 数据 (*.rtcm3;*.bin)", Pattern: "*.rtcm3;*.bin"}},
	})
	if err != nil || path == "" {
		return "", err
	}
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	a.recMu.Lock()
	if a.rec != nil {
		a.rec.Close()
	}
	a.rec, a.recPath, a.recBytes = f, path, 0
	a.recMu.Unlock()
	a.log("ok", "开始记录原始数据到 %s", path)
	return path, nil
}

// StopRecording closes the current recording file.
func (a *App) StopRecording() {
	a.recMu.Lock()
	defer a.recMu.Unlock()
	if a.rec == nil {
		return
	}
	a.rec.Close()
	a.log("info", "停止记录，共写入 %d 字节：%s", a.recBytes, a.recPath)
	a.rec = nil
}

func (a *App) record(b []byte) {
	a.recMu.Lock()
	defer a.recMu.Unlock()
	if a.rec == nil {
		return
	}
	n, err := a.rec.Write(b)
	a.recBytes += int64(n)
	if err != nil {
		a.rec.Close()
		a.rec = nil
		a.log("error", "写入记录文件失败: %v", err)
	}
}
