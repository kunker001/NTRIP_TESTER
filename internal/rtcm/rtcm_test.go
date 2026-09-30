package rtcm

import (
	"math"
	"testing"
)

type bitWriter struct {
	buf []byte
	pos int
}

func (w *bitWriter) put(v int64, n int) {
	for i := n - 1; i >= 0; i-- {
		if w.pos/8 >= len(w.buf) {
			w.buf = append(w.buf, 0)
		}
		if (uint64(v)>>uint(i))&1 == 1 {
			w.buf[w.pos/8] |= 1 << (7 - uint(w.pos%8))
		}
		w.pos++
	}
}

func frameOf(payload []byte) []byte {
	f := []byte{0xD3, byte(len(payload) >> 8), byte(len(payload))}
	f = append(f, payload...)
	crc := CRC24Q(f)
	return append(f, byte(crc>>16), byte(crc>>8), byte(crc))
}

func TestCRC24Q(t *testing.T) {
	if got := CRC24Q([]byte("123456789")); got != 0xCDE703 {
		t.Fatalf("CRC24Q = %06X, want CDE703", got)
	}
}

func TestFramerAnd1005(t *testing.T) {
	w := &bitWriter{}
	w.put(1005, 12)
	w.put(2690, 12)
	w.put(0, 6)
	w.put(1, 1) // GPS
	w.put(1, 1) // GLO
	w.put(0, 1) // GAL
	w.put(0, 1) // ref station
	w.put(-26946854730, 38)
	w.put(0, 2)
	w.put(-43607185420, 38)
	w.put(0, 2)
	w.put(38187236480, 38)

	var got []Frame
	f := &Framer{OnFrame: func(fr Frame) { got = append(got, fr) }}
	stream := append([]byte{0x00, 0xD3, 0xFF}, frameOf(w.buf)...)
	f.Write(stream[:5])
	f.Write(stream[5:])
	if len(got) != 1 || got[0].Type != 1005 {
		t.Fatalf("frames = %+v", got)
	}
	arp, err := DecodeARP(1005, got[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	if arp.StationID != 2690 || !arp.GPS || !arp.GLO || arp.GAL {
		t.Fatalf("header mismatch: %+v", arp)
	}
	if math.Abs(arp.X+2694685.4730) > 1e-6 || math.Abs(arp.Y+4360718.5420) > 1e-6 || math.Abs(arp.Z-3818723.6480) > 1e-6 {
		t.Fatalf("ECEF mismatch: %+v", arp)
	}
}

func TestMSM4CNR(t *testing.T) {
	w := &bitWriter{}
	w.put(1074, 12)
	w.put(1, 12)
	w.put(123456789, 30)
	w.put(0, 1+3+7+2+2+1+3)
	w.put(int64(uint64(1)<<(64-5)), 64) // satellite 5
	w.put(int64(uint32(1)<<(32-2)|uint32(1)<<(32-16)), 32)
	w.put(0b11, 2) // both cells present
	w.put(0, 18)   // satellite data
	w.put(0, 42*2) // pseudorange, phaserange, lock, half-cycle
	w.put(45, 6)
	w.put(38, 6)

	m, err := DecodeMSM(1074, w.buf)
	if err != nil {
		t.Fatal(err)
	}
	if m.Epoch != 123456789 || m.NSat != 1 || m.NSig != 2 || m.NCell != 2 {
		t.Fatalf("header mismatch: %+v", m)
	}
	s := m.Sats[0]
	if s.PRN != "G05" || s.Signals[0].Code != "1C" || s.Signals[0].CNR != 45 || s.Signals[1].Code != "2L" || s.Signals[1].CNR != 38 {
		t.Fatalf("sat mismatch: %+v", s)
	}
}
