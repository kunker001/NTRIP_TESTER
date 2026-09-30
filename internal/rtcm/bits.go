package rtcm

import (
	"errors"
	"strings"
)

var ErrShort = errors.New("消息长度不足")

// GetBitU reads n bits (MSB first) starting at bit position pos.
func GetBitU(buf []byte, pos, n int) uint64 {
	var v uint64
	for i := pos; i < pos+n; i++ {
		v = v<<1 | uint64(buf[i>>3]>>(7-uint(i&7))&1)
	}
	return v
}

// BitReader is a bounds-checked sequential bit reader. Reads past the end
// return zero and mark the reader as bad; check Err() after decoding.
type BitReader struct {
	buf []byte
	pos int
	bad bool
}

func NewBitReader(b []byte) *BitReader { return &BitReader{buf: b} }

func (r *BitReader) U(n int) uint64 {
	if r.pos+n > len(r.buf)*8 {
		r.bad = true
		r.pos += n
		return 0
	}
	v := GetBitU(r.buf, r.pos, n)
	r.pos += n
	return v
}

func (r *BitReader) S(n int) int64 {
	v := r.U(n)
	if n > 0 && n < 64 && v&(1<<uint(n-1)) != 0 {
		return int64(v) - int64(1)<<uint(n)
	}
	return int64(v)
}

func (r *BitReader) Skip(n int) {
	r.pos += n
	if r.pos > len(r.buf)*8 {
		r.bad = true
	}
}

func (r *BitReader) String(n int) string {
	b := make([]byte, 0, n)
	for i := 0; i < n; i++ {
		c := byte(r.U(8))
		if c == 0 {
			continue
		}
		b = append(b, c)
	}
	return strings.TrimSpace(string(b))
}

func (r *BitReader) Err() error {
	if r.bad {
		return ErrShort
	}
	return nil
}
