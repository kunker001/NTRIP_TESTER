package ntrip

import (
	"bufio"
	"io"
	"strings"
	"testing"
	"time"
)

func TestBuildGGA(t *testing.T) {
	ts := time.Date(2026, 9, 29, 9, 30, 15, 0, time.UTC)
	got := BuildGGA(ts, 31.5, -121.25, 10, 5)
	want := "$GPGGA,093015.00,3130.0000000,N,12115.0000000,W,1,12,1.0,10.000,M,5.000,M,,*"
	if !strings.HasPrefix(got, want) || !strings.HasSuffix(got, "\r\n") {
		t.Fatalf("GGA = %q", got)
	}
}

func TestReadResponseICY(t *testing.T) {
	br := bufio.NewReader(strings.NewReader("ICY 200 OK\r\n\r\n\xD3\x00\x13"))
	r, err := readResponse(br)
	if err != nil || r.kind != kindStream {
		t.Fatalf("resp=%+v err=%v", r, err)
	}
	b, _ := io.ReadAll(r.body)
	if len(b) != 3 || b[0] != 0xD3 {
		t.Fatalf("body = % X", b)
	}
}

func TestReadResponseChunkedV2(t *testing.T) {
	raw := "HTTP/1.1 200 OK\r\nContent-Type: gnss/data\r\nTransfer-Encoding: chunked\r\n\r\n3\r\n\xD3\x00\x13\r\n0\r\n\r\n"
	r, err := readResponse(bufio.NewReader(strings.NewReader(raw)))
	if err != nil || r.kind != kindStream {
		t.Fatalf("resp=%+v err=%v", r, err)
	}
	b, _ := io.ReadAll(r.body)
	if len(b) != 3 || b[0] != 0xD3 {
		t.Fatalf("body = % X", b)
	}
}

func TestSourceTable(t *testing.T) {
	raw := "SOURCETABLE 200 OK\r\nContent-Type: text/plain\r\n\r\n" +
		"STR;MP1;Shanghai;RTCM 3.3;1004(1),1005(10);2;GPS+GLO;NET;CHN;31.23;121.47;1;0;sNTRIP;none;B;N;9600;misc\r\n" +
		"ENDSOURCETABLE\r\n"
	br := bufio.NewReader(strings.NewReader(raw))
	r, err := readResponse(br)
	if err != nil || r.kind != kindSourceTable {
		t.Fatalf("resp=%+v err=%v", r, err)
	}
	body, err := readSourceTableBody(r.body)
	if err != nil {
		t.Fatal(err)
	}
	st := ParseSourceTable(body)
	if len(st.Mounts) != 1 {
		t.Fatalf("mounts = %+v", st.Mounts)
	}
	m := st.Mounts[0]
	if m.Name != "MP1" || !m.NMEA || m.Lat != 31.23 || m.Lon != 121.47 || m.Bitrate != 9600 || m.Auth != "B" {
		t.Fatalf("mount = %+v", m)
	}
}
