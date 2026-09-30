package ntrip

import (
	"fmt"
	"math"
	"time"
)

// BuildGGA builds an NMEA GGA sentence (with CRLF) for a fixed position.
func BuildGGA(t time.Time, lat, lon, alt, sep float64) string {
	t = t.UTC()
	ns, ew := "N", "E"
	if lat < 0 {
		ns, lat = "S", -lat
	}
	if lon < 0 {
		ew, lon = "W", -lon
	}
	latDeg := math.Floor(lat)
	lonDeg := math.Floor(lon)
	body := fmt.Sprintf("GPGGA,%02d%02d%02d.00,%02d%010.7f,%s,%03d%010.7f,%s,1,12,1.0,%.3f,M,%.3f,M,,",
		t.Hour(), t.Minute(), t.Second(),
		int(latDeg), (lat-latDeg)*60, ns,
		int(lonDeg), (lon-lonDeg)*60, ew,
		alt, sep)
	var cs byte
	for i := 0; i < len(body); i++ {
		cs ^= body[i]
	}
	return fmt.Sprintf("$%s*%02X\r\n", body, cs)
}
