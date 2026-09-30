package rtcm

import "fmt"

// StationARP holds RTCM 1005/1006 content.
type StationARP struct {
	MsgType    int
	StationID  int
	ITRFYear   int
	GPS        bool
	GLO        bool
	GAL        bool
	RefStation bool
	X, Y, Z    float64
	Height     float64
}

func DecodeARP(t int, p []byte) (*StationARP, error) {
	r := NewBitReader(p)
	r.Skip(12)
	s := &StationARP{MsgType: t}
	s.StationID = int(r.U(12))
	s.ITRFYear = int(r.U(6))
	s.GPS = r.U(1) == 1
	s.GLO = r.U(1) == 1
	s.GAL = r.U(1) == 1
	s.RefStation = r.U(1) == 1
	s.X = float64(r.S(38)) * 0.0001
	r.Skip(2) // single receiver oscillator indicator, reserved
	s.Y = float64(r.S(38)) * 0.0001
	r.Skip(2) // quarter cycle indicator
	s.Z = float64(r.S(38)) * 0.0001
	if t == 1006 {
		s.Height = float64(r.U(16)) * 0.0001
	}
	return s, r.Err()
}

// Descriptor holds RTCM 1007/1008/1033 content.
type Descriptor struct {
	StationID      int
	Antenna        string
	SetupID        int
	AntennaSerial  string
	Receiver       string
	Firmware       string
	ReceiverSerial string
}

func DecodeDescriptor(t int, p []byte) (*Descriptor, error) {
	r := NewBitReader(p)
	r.Skip(12)
	d := &Descriptor{StationID: int(r.U(12))}
	d.Antenna = r.String(int(r.U(8)))
	d.SetupID = int(r.U(8))
	if t == 1008 || t == 1033 {
		d.AntennaSerial = r.String(int(r.U(8)))
	}
	if t == 1033 {
		d.Receiver = r.String(int(r.U(8)))
		d.Firmware = r.String(int(r.U(8)))
		d.ReceiverSerial = r.String(int(r.U(8)))
	}
	return d, r.Err()
}

type MSMSignal struct {
	ID   int     `json:"id"`
	Code string  `json:"code"`
	CNR  float64 `json:"cnr"`
}

type MSMSat struct {
	ID      int         `json:"id"`
	PRN     string      `json:"prn"`
	Signals []MSMSignal `json:"signals"`
}

// MSM holds the header and per-cell C/N0 of an MSM1-7 message.
type MSM struct {
	MsgType   int
	System    string
	Level     int
	StationID int
	Epoch     uint32
	GloDay    int
	MultiMsg  bool
	IODS      int
	NSat      int
	NSig      int
	NCell     int
	Sats      []MSMSat
}

// MSMInfo reports the constellation and MSM level (1-7) of a message type.
func MSMInfo(t int) (string, int, bool) {
	if t < 1071 || t > 1137 {
		return "", 0, false
	}
	level := t % 10
	if level < 1 || level > 7 {
		return "", 0, false
	}
	switch t / 10 {
	case 107:
		return "GPS", level, true
	case 108:
		return "GLO", level, true
	case 109:
		return "GAL", level, true
	case 110:
		return "SBAS", level, true
	case 111:
		return "QZSS", level, true
	case 112:
		return "BDS", level, true
	case 113:
		return "NavIC", level, true
	}
	return "", 0, false
}

var msmSatBits = map[int]int{1: 10, 2: 10, 3: 10, 4: 18, 5: 36, 6: 18, 7: 36}

func DecodeMSM(t int, p []byte) (*MSM, error) {
	sys, level, ok := MSMInfo(t)
	if !ok {
		return nil, fmt.Errorf("非 MSM 消息 %d", t)
	}
	r := NewBitReader(p)
	r.Skip(12)
	m := &MSM{MsgType: t, System: sys, Level: level}
	m.StationID = int(r.U(12))
	if sys == "GLO" {
		m.GloDay = int(r.U(3))
		m.Epoch = uint32(r.U(27))
	} else {
		m.Epoch = uint32(r.U(30))
	}
	m.MultiMsg = r.U(1) == 1
	m.IODS = int(r.U(3))
	r.Skip(7 + 2 + 2 + 1 + 3) // reserved, clock steering, ext clock, smoothing
	satMask := r.U(64)
	sigMask := r.U(32)
	var sats, sigs []int
	for i := 0; i < 64; i++ {
		if satMask>>(63-uint(i))&1 == 1 {
			sats = append(sats, i+1)
		}
	}
	for i := 0; i < 32; i++ {
		if sigMask>>(31-uint(i))&1 == 1 {
			sigs = append(sigs, i+1)
		}
	}
	m.NSat, m.NSig = len(sats), len(sigs)
	if m.NSat*m.NSig > 64 {
		return nil, fmt.Errorf("MSM 单元数 %d 超出上限 64", m.NSat*m.NSig)
	}
	cells := make([]bool, m.NSat*m.NSig)
	for i := range cells {
		if r.U(1) == 1 {
			cells[i] = true
			m.NCell++
		}
	}
	if err := r.Err(); err != nil {
		return nil, err
	}

	r.Skip(msmSatBits[level] * m.NSat)
	cnr := make([]float64, m.NCell)
	switch level {
	case 4, 5:
		r.Skip(42 * m.NCell) // pseudorange 15, phaserange 22, lock 4, half-cycle 1
		for i := range cnr {
			cnr[i] = float64(r.U(6))
		}
	case 6, 7:
		r.Skip(55 * m.NCell) // pseudorange 20, phaserange 24, lock 10, half-cycle 1
		for i := range cnr {
			cnr[i] = float64(r.U(10)) * 0.0625
		}
	}
	if err := r.Err(); err != nil {
		return nil, err
	}

	k := 0
	for i, sid := range sats {
		sat := MSMSat{ID: sid, PRN: SatLabel(sys, sid), Signals: []MSMSignal{}}
		for j, sg := range sigs {
			if !cells[i*m.NSig+j] {
				continue
			}
			sat.Signals = append(sat.Signals, MSMSignal{ID: sg, Code: SignalCode(sys, sg), CNR: cnr[k]})
			k++
		}
		m.Sats = append(m.Sats, sat)
	}
	return m, nil
}

// EphemerisSat extracts the satellite of an ephemeris message.
func EphemerisSat(t int, p []byte) (sys, prn string, ok bool) {
	bits := 6
	switch t {
	case 1019:
		sys = "GPS"
	case 1020:
		sys = "GLO"
	case 1041:
		sys = "NavIC"
	case 1042:
		sys = "BDS"
	case 1044:
		sys, bits = "QZSS", 4
	case 1045, 1046:
		sys = "GAL"
	default:
		return "", "", false
	}
	if len(p)*8 < 12+bits {
		return "", "", false
	}
	return sys, SatLabel(sys, int(GetBitU(p, 12, bits))), true
}

// DecodeText decodes RTCM 1029 unicode text.
func DecodeText(p []byte) (int, string, error) {
	r := NewBitReader(p)
	r.Skip(12)
	station := int(r.U(12))
	r.Skip(16 + 17 + 7) // MJD, UTC seconds of day, character count
	text := r.String(int(r.U(8)))
	return station, text, r.Err()
}

func SatLabel(sys string, id int) string {
	switch sys {
	case "GPS":
		return fmt.Sprintf("G%02d", id)
	case "GLO":
		return fmt.Sprintf("R%02d", id)
	case "GAL":
		return fmt.Sprintf("E%02d", id)
	case "BDS":
		return fmt.Sprintf("C%02d", id)
	case "QZSS":
		return fmt.Sprintf("J%02d", id)
	case "SBAS":
		return fmt.Sprintf("S%02d", id+19)
	case "NavIC":
		return fmt.Sprintf("I%02d", id)
	}
	return fmt.Sprintf("?%02d", id)
}

// RINEX observation codes indexed by MSM signal ID.
var sigCodes = map[string]map[int]string{
	"GPS":   {2: "1C", 3: "1P", 4: "1W", 8: "2C", 9: "2P", 10: "2W", 15: "2S", 16: "2L", 17: "2X", 22: "5I", 23: "5Q", 24: "5X", 30: "1S", 31: "1L", 32: "1X"},
	"GLO":   {2: "1C", 3: "1P", 8: "2C", 9: "2P"},
	"GAL":   {2: "1C", 3: "1A", 4: "1B", 5: "1X", 6: "1Z", 8: "6C", 9: "6A", 10: "6B", 11: "6X", 12: "6Z", 14: "7I", 15: "7Q", 16: "7X", 18: "8I", 19: "8Q", 20: "8X", 22: "5I", 23: "5Q", 24: "5X"},
	"SBAS":  {2: "1C", 22: "5I", 23: "5Q", 24: "5X"},
	"QZSS":  {2: "1C", 9: "6S", 10: "6L", 11: "6X", 15: "2S", 16: "2L", 17: "2X", 22: "5I", 23: "5Q", 24: "5X", 30: "1S", 31: "1L", 32: "1X"},
	"BDS":   {2: "2I", 3: "2Q", 4: "2X", 8: "6I", 9: "6Q", 10: "6X", 14: "7I", 15: "7Q", 16: "7X", 22: "5D", 23: "5P", 24: "5X", 25: "7D", 30: "1D", 31: "1P", 32: "1X"},
	"NavIC": {22: "5A"},
}

func SignalCode(sys string, id int) string {
	if c, ok := sigCodes[sys][id]; ok {
		return c
	}
	return fmt.Sprintf("S%d", id)
}
