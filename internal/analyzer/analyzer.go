package analyzer

import (
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"ntrip-tester/internal/geo"
	"ntrip-tester/internal/rtcm"
)

const (
	rateWindow  = 10
	maxConsole  = 1000
	maxTexts    = 20
	leapSeconds = 18
)

var (
	gpsEpoch    = time.Date(1980, 1, 6, 0, 0, 0, 0, time.UTC)
	systemOrder = []string{"GPS", "GLO", "GAL", "BDS", "QZSS", "SBAS", "NavIC"}
)

type MsgStat struct {
	Type     int     `json:"type"`
	Name     string  `json:"name"`
	Count    int64   `json:"count"`
	Bytes    int64   `json:"bytes"`
	Rate     float64 `json:"rate"`
	Interval float64 `json:"interval"`
	LastAgo  float64 `json:"lastAgo"`
	MinLen   int     `json:"minLen"`
	MaxLen   int     `json:"maxLen"`
	Info     string  `json:"info"`
}

type msgAcc struct {
	stat        MsgStat
	first, last time.Time
	secCount    int
	ring        [rateWindow]int
	ringN       int
	ringIdx     int
}

type SigStat struct {
	Code string  `json:"code"`
	CNR  float64 `json:"cnr"`
}

type SatStat struct {
	ID      int       `json:"id"`
	PRN     string    `json:"prn"`
	Signals []SigStat `json:"signals"`
}

type SysStat struct {
	System    string    `json:"system"`
	MsgType   int       `json:"msgType"`
	NumSats   int       `json:"numSats"`
	NumCells  int       `json:"numCells"`
	Signals   []string  `json:"signals"`
	LatencyMs float64   `json:"latencyMs"`
	AgeSec    float64   `json:"ageSec"`
	Sats      []SatStat `json:"sats"`
}

type sysState struct {
	msgType   int
	epoch     uint32
	gloDay    int
	sats      map[int]rtcm.MSMSat
	updated   time.Time
	latencyMs float64
}

type StationInfo struct {
	StationID      int      `json:"stationId"`
	HasARP         bool     `json:"hasArp"`
	ARPType        int      `json:"arpType"`
	ITRFYear       int      `json:"itrfYear"`
	Systems        []string `json:"systems"`
	RefStation     bool     `json:"refStation"`
	X              float64  `json:"x"`
	Y              float64  `json:"y"`
	Z              float64  `json:"z"`
	Lat            float64  `json:"lat"`
	Lon            float64  `json:"lon"`
	Height         float64  `json:"height"`
	AntennaHeight  float64  `json:"antennaHeight"`
	Antenna        string   `json:"antenna"`
	AntennaSerial  string   `json:"antennaSerial"`
	SetupID        int      `json:"setupId"`
	Receiver       string   `json:"receiver"`
	Firmware       string   `json:"firmware"`
	ReceiverSerial string   `json:"receiverSerial"`
	HasDistance    bool     `json:"hasDistance"`
	DistanceKm     float64  `json:"distanceKm"`
	Bearing        float64  `json:"bearing"`
}

type EphStat struct {
	System  string  `json:"system"`
	PRN     string  `json:"prn"`
	MsgType int     `json:"msgType"`
	Count   int64   `json:"count"`
	AgeSec  float64 `json:"ageSec"`
}

type ephAcc struct {
	stat EphStat
	last time.Time
}

type TextMsg struct {
	Time    string `json:"time"`
	Station int    `json:"station"`
	Text    string `json:"text"`
}

type ConsoleLine struct {
	Time    string `json:"time"`
	Kind    string `json:"kind"`
	Type    int    `json:"type"`
	Name    string `json:"name"`
	Len     int    `json:"len"`
	Summary string `json:"summary"`
	Hex     string `json:"hex"`
}

type Snapshot struct {
	Uptime       float64      `json:"uptime"`
	TotalBytes   int64        `json:"totalBytes"`
	RtcmBytes    int64        `json:"rtcmBytes"`
	GarbageBytes int64        `json:"garbageBytes"`
	Frames       int64        `json:"frames"`
	CRCErrors    int64        `json:"crcErrors"`
	Bitrate      float64      `json:"bitrate"`
	AvgBitrate   float64      `json:"avgBitrate"`
	PeakBitrate  float64      `json:"peakBitrate"`
	LastDataAgo  float64      `json:"lastDataAgo"`
	Messages     []MsgStat    `json:"messages"`
	Systems      []SysStat    `json:"systems"`
	Station      *StationInfo `json:"station"`
	Ephemeris    []EphStat    `json:"ephemeris"`
	Texts        []TextMsg    `json:"texts"`
}

// Analyzer accumulates statistics of an RTCM3 byte stream. Safe for concurrent use.
type Analyzer struct {
	mu     sync.Mutex
	framer *rtcm.Framer

	firstData, lastData time.Time
	totalBytes          int64
	rtcmBytes           int64
	garbageBytes        int64
	frames              int64
	crcErrors           int64
	secBytes            int64
	bitrate             float64
	peakBitrate         float64

	msgs    map[int]*msgAcc
	systems map[string]*sysState
	station *StationInfo
	eph     map[string]*ephAcc
	texts   []TextMsg
	console []ConsoleLine

	refLat, refLon float64
	refOK          bool
}

func New() *Analyzer {
	a := &Analyzer{}
	a.Reset()
	return a
}

// Reset clears all statistics but keeps the reference position.
func (a *Analyzer) Reset() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.framer = &rtcm.Framer{OnFrame: a.onFrame, OnCRCError: a.onCRCError, OnGarbage: a.onGarbage}
	a.firstData, a.lastData = time.Time{}, time.Time{}
	a.totalBytes, a.rtcmBytes, a.garbageBytes, a.frames, a.crcErrors, a.secBytes = 0, 0, 0, 0, 0, 0
	a.bitrate, a.peakBitrate = 0, 0
	a.msgs = map[int]*msgAcc{}
	a.systems = map[string]*sysState{}
	a.station = nil
	a.eph = map[string]*ephAcc{}
	a.texts = nil
	a.console = nil
}

func (a *Analyzer) SetReference(lat, lon float64, ok bool) {
	a.mu.Lock()
	a.refLat, a.refLon, a.refOK = lat, lon, ok
	a.mu.Unlock()
}

func (a *Analyzer) Feed(p []byte) {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	if a.firstData.IsZero() {
		a.firstData = now
	}
	a.lastData = now
	a.totalBytes += int64(len(p))
	a.secBytes += int64(len(p))
	a.framer.Write(p)
}

// Tick must be called once per second to update rate statistics.
func (a *Analyzer) Tick() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.bitrate = float64(a.secBytes * 8)
	if a.bitrate > a.peakBitrate {
		a.peakBitrate = a.bitrate
	}
	a.secBytes = 0
	for _, m := range a.msgs {
		m.ring[m.ringIdx] = m.secCount
		m.ringIdx = (m.ringIdx + 1) % rateWindow
		if m.ringN < rateWindow {
			m.ringN++
		}
		m.secCount = 0
	}
}

func (a *Analyzer) DrainConsole() []ConsoleLine {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := a.console
	a.console = nil
	return out
}

func (a *Analyzer) pushConsole(l ConsoleLine) {
	if len(a.console) >= maxConsole {
		n := copy(a.console, a.console[maxConsole/2:])
		a.console = a.console[:n]
	}
	a.console = append(a.console, l)
}

func (a *Analyzer) onCRCError(length int) {
	a.crcErrors++
	a.pushConsole(ConsoleLine{Time: time.Now().Format("15:04:05.000"), Kind: "warn", Summary: fmt.Sprintf("CRC 校验失败（声明长度 %d 字节），重新同步", length)})
}

func (a *Analyzer) onGarbage(n int) {
	a.garbageBytes += int64(n)
	a.pushConsole(ConsoleLine{Time: time.Now().Format("15:04:05.000"), Kind: "warn", Summary: fmt.Sprintf("跳过 %d 字节非 RTCM3 数据", n)})
}

func (a *Analyzer) onFrame(fr rtcm.Frame) {
	now := time.Now()
	a.frames++
	a.rtcmBytes += int64(len(fr.Raw))
	l := len(fr.Payload)
	m := a.msgs[fr.Type]
	if m == nil {
		m = &msgAcc{stat: MsgStat{Type: fr.Type, Name: rtcm.MessageName(fr.Type), MinLen: l}, first: now}
		a.msgs[fr.Type] = m
	}
	m.stat.Count++
	m.stat.Bytes += int64(len(fr.Raw))
	if l < m.stat.MinLen {
		m.stat.MinLen = l
	}
	if l > m.stat.MaxLen {
		m.stat.MaxLen = l
	}
	m.last = now
	m.secCount++
	summary := a.decode(fr, now, m)
	a.pushConsole(ConsoleLine{
		Time: now.Format("15:04:05.000"), Kind: "msg", Type: fr.Type, Name: m.stat.Name,
		Len: l, Summary: summary, Hex: hexPreview(fr.Raw, 32),
	})
}

func (a *Analyzer) stationInfo() *StationInfo {
	if a.station == nil {
		a.station = &StationInfo{Systems: []string{}}
	}
	return a.station
}

func (a *Analyzer) decode(fr rtcm.Frame, now time.Time, m *msgAcc) string {
	t, p := fr.Type, fr.Payload
	_, _, isMSM := rtcm.MSMInfo(t)
	switch {
	case t == 1005 || t == 1006:
		arp, err := rtcm.DecodeARP(t, p)
		if err != nil {
			return "解码失败: " + err.Error()
		}
		st := a.stationInfo()
		st.StationID, st.HasARP, st.ARPType, st.ITRFYear, st.RefStation = arp.StationID, true, t, arp.ITRFYear, arp.RefStation
		st.X, st.Y, st.Z = arp.X, arp.Y, arp.Z
		st.Lat, st.Lon, st.Height = geo.ECEFToLLH(arp.X, arp.Y, arp.Z)
		st.Systems = st.Systems[:0]
		if arp.GPS {
			st.Systems = append(st.Systems, "GPS")
		}
		if arp.GLO {
			st.Systems = append(st.Systems, "GLONASS")
		}
		if arp.GAL {
			st.Systems = append(st.Systems, "Galileo")
		}
		if t == 1006 {
			st.AntennaHeight = arp.Height
		}
		m.stat.Info = fmt.Sprintf("站号 %d · %.6f, %.6f", arp.StationID, st.Lat, st.Lon)
		return fmt.Sprintf("站号 %d ECEF(%.4f, %.4f, %.4f) → LLH(%.8f, %.8f, %.3f)", arp.StationID, arp.X, arp.Y, arp.Z, st.Lat, st.Lon, st.Height)

	case t == 1007 || t == 1008 || t == 1033:
		d, err := rtcm.DecodeDescriptor(t, p)
		if err != nil {
			return "解码失败: " + err.Error()
		}
		st := a.stationInfo()
		st.StationID, st.Antenna, st.SetupID = d.StationID, d.Antenna, d.SetupID
		if t != 1007 {
			st.AntennaSerial = d.AntennaSerial
		}
		if t == 1033 {
			st.Receiver, st.Firmware, st.ReceiverSerial = d.Receiver, d.Firmware, d.ReceiverSerial
			m.stat.Info = strings.TrimSpace(d.Receiver + " " + d.Firmware)
			return fmt.Sprintf("站号 %d 天线 %q 接收机 %q 固件 %q", d.StationID, d.Antenna, d.Receiver, d.Firmware)
		}
		m.stat.Info = d.Antenna
		return fmt.Sprintf("站号 %d 天线 %q 序列号 %q", d.StationID, d.Antenna, d.AntennaSerial)

	case isMSM:
		msm, err := rtcm.DecodeMSM(t, p)
		if err != nil {
			return "解码失败: " + err.Error()
		}
		s := a.systems[msm.System]
		if s == nil {
			s = &sysState{sats: map[int]rtcm.MSMSat{}}
			a.systems[msm.System] = s
		}
		if s.epoch != msm.Epoch || s.gloDay != msm.GloDay {
			s.sats = map[int]rtcm.MSMSat{}
		}
		s.msgType, s.epoch, s.gloDay, s.updated = t, msm.Epoch, msm.GloDay, now
		for _, sat := range msm.Sats {
			s.sats[sat.ID] = sat
		}
		s.latencyMs = msmLatency(msm.System, msm.Epoch, now)
		m.stat.Info = fmt.Sprintf("%d 星 · %d 信号 · %d 观测", msm.NSat, msm.NSig, msm.NCell)
		more := ""
		if msm.MultiMsg {
			more = " (多消息)"
		}
		return fmt.Sprintf("站号 %d 历元 %s 卫星 %d 信号 %d 观测 %d 延迟≈%.0fms%s",
			msm.StationID, epochString(msm), msm.NSat, msm.NSig, msm.NCell, s.latencyMs, more)

	case t == 1029:
		station, text, err := rtcm.DecodeText(p)
		if err != nil {
			return "解码失败: " + err.Error()
		}
		a.texts = append(a.texts, TextMsg{Time: now.Format("15:04:05"), Station: station, Text: text})
		if len(a.texts) > maxTexts {
			a.texts = a.texts[len(a.texts)-maxTexts:]
		}
		return fmt.Sprintf("站号 %d: %s", station, text)
	}

	if sys, prn, ok := rtcm.EphemerisSat(t, p); ok {
		e := a.eph[prn]
		if e == nil {
			e = &ephAcc{stat: EphStat{System: sys, PRN: prn}}
			a.eph[prn] = e
		}
		e.stat.Count++
		e.stat.MsgType = t
		e.last = now
		m.stat.Info = fmt.Sprintf("最近 %s", prn)
		return "卫星 " + prn
	}
	return ""
}

func epochString(m *rtcm.MSM) string {
	if m.System == "GLO" {
		ms := m.Epoch
		return fmt.Sprintf("周%d %02d:%02d:%02d.%03d", m.GloDay, ms/3600000, ms/60000%60, ms/1000%60, ms%1000)
	}
	return fmt.Sprintf("TOW %.3f", float64(m.Epoch)/1000)
}

// msmLatency estimates (local clock - epoch time) in milliseconds.
func msmLatency(sys string, epoch uint32, now time.Time) float64 {
	if sys == "GLO" {
		mt := now.UTC().Add(3 * time.Hour)
		dayStart := time.Date(mt.Year(), mt.Month(), mt.Day(), 0, 0, 0, 0, time.UTC)
		ms := float64(mt.Sub(dayStart).Milliseconds())
		return wrap(ms-float64(epoch), 86400000)
	}
	gpst := now.UTC().Sub(gpsEpoch) + leapSeconds*time.Second
	tow := float64((gpst % (7 * 24 * time.Hour)).Milliseconds())
	if sys == "BDS" {
		tow -= 14000
	}
	return wrap(tow-float64(epoch), 604800000)
}

func wrap(d, period float64) float64 {
	for d > period/2 {
		d -= period
	}
	for d < -period/2 {
		d += period
	}
	return d
}

func hexPreview(b []byte, n int) string {
	if len(b) < n {
		n = len(b)
	}
	s := strings.ToUpper(hex.EncodeToString(b[:n]))
	var sb strings.Builder
	for i := 0; i < len(s); i += 2 {
		if i > 0 {
			sb.WriteByte(' ')
		}
		sb.WriteString(s[i : i+2])
	}
	if len(b) > n {
		sb.WriteString(fmt.Sprintf(" … (%d 字节)", len(b)))
	}
	return sb.String()
}

func sysRank(s string) int {
	for i, v := range systemOrder {
		if v == s {
			return i
		}
	}
	return len(systemOrder)
}

func (a *Analyzer) Snapshot() Snapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	s := Snapshot{
		TotalBytes: a.totalBytes, RtcmBytes: a.rtcmBytes, GarbageBytes: a.garbageBytes,
		Frames: a.frames, CRCErrors: a.crcErrors, Bitrate: a.bitrate, PeakBitrate: a.peakBitrate,
		LastDataAgo: -1,
		Messages:    make([]MsgStat, 0, len(a.msgs)),
		Systems:     make([]SysStat, 0, len(a.systems)),
		Ephemeris:   make([]EphStat, 0, len(a.eph)),
		Texts:       append([]TextMsg{}, a.texts...),
	}
	if !a.firstData.IsZero() {
		s.Uptime = now.Sub(a.firstData).Seconds()
		if s.Uptime > 0 {
			s.AvgBitrate = float64(a.totalBytes*8) / s.Uptime
		}
		s.LastDataAgo = now.Sub(a.lastData).Seconds()
	}

	for _, m := range a.msgs {
		st := m.stat
		if m.ringN > 0 {
			sum := 0
			for i := 0; i < m.ringN; i++ {
				sum += m.ring[i]
			}
			st.Rate = float64(sum) / float64(m.ringN)
		}
		if st.Count > 1 {
			st.Interval = m.last.Sub(m.first).Seconds() / float64(st.Count-1)
		}
		st.LastAgo = now.Sub(m.last).Seconds()
		s.Messages = append(s.Messages, st)
	}
	sort.Slice(s.Messages, func(i, j int) bool { return s.Messages[i].Type < s.Messages[j].Type })

	for name, ss := range a.systems {
		out := SysStat{System: name, MsgType: ss.msgType, LatencyMs: ss.latencyMs, AgeSec: now.Sub(ss.updated).Seconds(), Signals: []string{}, Sats: []SatStat{}}
		ids := make([]int, 0, len(ss.sats))
		for id := range ss.sats {
			ids = append(ids, id)
		}
		sort.Ints(ids)
		seen := map[string]bool{}
		for _, id := range ids {
			sat := ss.sats[id]
			o := SatStat{ID: id, PRN: sat.PRN, Signals: make([]SigStat, 0, len(sat.Signals))}
			for _, g := range sat.Signals {
				o.Signals = append(o.Signals, SigStat{Code: g.Code, CNR: g.CNR})
				if !seen[g.Code] {
					seen[g.Code] = true
					out.Signals = append(out.Signals, g.Code)
				}
			}
			out.NumCells += len(sat.Signals)
			out.Sats = append(out.Sats, o)
		}
		out.NumSats = len(out.Sats)
		s.Systems = append(s.Systems, out)
	}
	sort.Slice(s.Systems, func(i, j int) bool { return sysRank(s.Systems[i].System) < sysRank(s.Systems[j].System) })

	if a.station != nil {
		st := *a.station
		st.Systems = append([]string{}, a.station.Systems...)
		if a.refOK && st.HasARP {
			st.HasDistance = true
			st.DistanceKm = geo.Haversine(a.refLat, a.refLon, st.Lat, st.Lon)
			st.Bearing = geo.Bearing(a.refLat, a.refLon, st.Lat, st.Lon)
		}
		s.Station = &st
	}

	for _, e := range a.eph {
		es := e.stat
		es.AgeSec = now.Sub(e.last).Seconds()
		s.Ephemeris = append(s.Ephemeris, es)
	}
	sort.Slice(s.Ephemeris, func(i, j int) bool {
		ri, rj := sysRank(s.Ephemeris[i].System), sysRank(s.Ephemeris[j].System)
		if ri != rj {
			return ri < rj
		}
		return s.Ephemeris[i].PRN < s.Ephemeris[j].PRN
	})
	return s
}
