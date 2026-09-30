package geo

import "math"

const (
	wgsA  = 6378137.0
	wgsF  = 1 / 298.257223563
	wgsE2 = wgsF * (2 - wgsF)

	earthRadiusKm = 6371.0088
)

// ECEFToLLH converts WGS84 ECEF (m) to geodetic latitude/longitude (deg) and ellipsoidal height (m).
func ECEFToLLH(x, y, z float64) (lat, lon, h float64) {
	p := math.Hypot(x, y)
	if p == 0 && z == 0 {
		return 0, 0, 0
	}
	lon = math.Atan2(y, x)
	la := math.Atan2(z, p*(1-wgsE2))
	for i := 0; i < 10; i++ {
		s := math.Sin(la)
		n := wgsA / math.Sqrt(1-wgsE2*s*s)
		h = p/math.Cos(la) - n
		la = math.Atan2(z, p*(1-wgsE2*n/(n+h)))
	}
	s := math.Sin(la)
	n := wgsA / math.Sqrt(1-wgsE2*s*s)
	h = p/math.Cos(la) - n
	return la * 180 / math.Pi, lon * 180 / math.Pi, h
}

func rad(d float64) float64 { return d * math.Pi / 180 }

// Haversine returns the great-circle distance in km.
func Haversine(lat1, lon1, lat2, lon2 float64) float64 {
	dLat := rad(lat2 - lat1)
	dLon := rad(lon2 - lon1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(rad(lat1))*math.Cos(rad(lat2))*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * earthRadiusKm * math.Asin(math.Min(1, math.Sqrt(a)))
}

// Bearing returns the initial bearing from point 1 to point 2 in degrees (0-360).
func Bearing(lat1, lon1, lat2, lon2 float64) float64 {
	y := math.Sin(rad(lon2-lon1)) * math.Cos(rad(lat2))
	x := math.Cos(rad(lat1))*math.Sin(rad(lat2)) - math.Sin(rad(lat1))*math.Cos(rad(lat2))*math.Cos(rad(lon2-lon1))
	return math.Mod(math.Atan2(y, x)*180/math.Pi+360, 360)
}
