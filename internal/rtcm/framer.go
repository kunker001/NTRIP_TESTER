package rtcm

var crcTable [256]uint32

func init() {
	for i := 0; i < 256; i++ {
		crc := uint32(i) << 16
		for j := 0; j < 8; j++ {
			crc <<= 1
			if crc&0x1000000 != 0 {
				crc ^= 0x1864CFB
			}
		}
		crcTable[i] = crc & 0xFFFFFF
	}
}

// CRC24Q computes the Qualcomm CRC-24 used by RTCM3.
func CRC24Q(data []byte) uint32 {
	var crc uint32
	for _, b := range data {
		crc = ((crc << 8) & 0xFFFFFF) ^ crcTable[byte(crc>>16)^b]
	}
	return crc
}

// Frame is a single CRC-validated RTCM3 frame.
type Frame struct {
	Type    int
	Payload []byte
	Raw     []byte
}

// Framer extracts RTCM3 frames from an arbitrary byte stream.
type Framer struct {
	buf        []byte
	OnFrame    func(Frame)
	OnCRCError func(length int)
	OnGarbage  func(n int)
}

func (f *Framer) Write(p []byte) {
	f.buf = append(f.buf, p...)
	i, garbage := 0, 0
	flushGarbage := func() {
		if garbage > 0 && f.OnGarbage != nil {
			f.OnGarbage(garbage)
		}
		garbage = 0
	}
	for {
		for i < len(f.buf) && f.buf[i] != 0xD3 {
			i++
			garbage++
		}
		if len(f.buf)-i < 3 {
			break
		}
		if f.buf[i+1]&0xFC != 0 {
			i++
			garbage++
			continue
		}
		length := int(f.buf[i+1]&0x03)<<8 | int(f.buf[i+2])
		total := 3 + length + 3
		if len(f.buf)-i < total {
			break
		}
		frame := f.buf[i : i+total]
		crc := uint32(frame[total-3])<<16 | uint32(frame[total-2])<<8 | uint32(frame[total-1])
		if CRC24Q(frame[:total-3]) != crc {
			if f.OnCRCError != nil {
				f.OnCRCError(length)
			}
			i++
			garbage++
			continue
		}
		flushGarbage()
		raw := make([]byte, total)
		copy(raw, frame)
		fr := Frame{Raw: raw, Payload: raw[3 : 3+length]}
		if length >= 2 {
			fr.Type = int(GetBitU(fr.Payload, 0, 12))
		}
		if f.OnFrame != nil {
			f.OnFrame(fr)
		}
		i += total
	}
	flushGarbage()
	f.buf = append(f.buf[:0], f.buf[i:]...)
}
