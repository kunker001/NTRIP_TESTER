package ntrip

import (
	"strconv"
	"strings"
)

// Mount is a STR record of an NTRIP sourcetable.
type Mount struct {
	Name          string  `json:"name"`
	Identifier    string  `json:"identifier"`
	Format        string  `json:"format"`
	FormatDetails string  `json:"formatDetails"`
	Carrier       int     `json:"carrier"`
	NavSystem     string  `json:"navSystem"`
	Network       string  `json:"network"`
	Country       string  `json:"country"`
	Lat           float64 `json:"lat"`
	Lon           float64 `json:"lon"`
	NMEA          bool    `json:"nmea"`
	Solution      int     `json:"solution"`
	Generator     string  `json:"generator"`
	Compression   string  `json:"compression"`
	Auth          string  `json:"auth"`
	Fee           bool    `json:"fee"`
	Bitrate       int     `json:"bitrate"`
	Misc          string  `json:"misc"`
}

type SourceTable struct {
	Mounts   []Mount  `json:"mounts"`
	Casters  []string `json:"casters"`
	Networks []string `json:"networks"`
	Raw      string   `json:"raw"`
}

func ParseSourceTable(raw string) *SourceTable {
	st := &SourceTable{Mounts: []Mount{}, Casters: []string{}, Networks: []string{}, Raw: raw}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimRight(line, "\r")
		switch {
		case strings.HasPrefix(line, "STR;"):
			st.Mounts = append(st.Mounts, parseSTR(strings.Split(line, ";")))
		case strings.HasPrefix(line, "CAS;"):
			st.Casters = append(st.Casters, line)
		case strings.HasPrefix(line, "NET;"):
			st.Networks = append(st.Networks, line)
		}
	}
	return st
}

func parseSTR(f []string) Mount {
	get := func(i int) string {
		if i < len(f) {
			return strings.TrimSpace(f[i])
		}
		return ""
	}
	atoi := func(s string) int { v, _ := strconv.Atoi(s); return v }
	atof := func(s string) float64 { v, _ := strconv.ParseFloat(s, 64); return v }
	m := Mount{
		Name:          get(1),
		Identifier:    get(2),
		Format:        get(3),
		FormatDetails: get(4),
		Carrier:       atoi(get(5)),
		NavSystem:     get(6),
		Network:       get(7),
		Country:       get(8),
		Lat:           atof(get(9)),
		Lon:           atof(get(10)),
		NMEA:          get(11) == "1",
		Solution:      atoi(get(12)),
		Generator:     get(13),
		Compression:   get(14),
		Auth:          get(15),
		Fee:           strings.EqualFold(get(16), "Y"),
		Bitrate:       atoi(get(17)),
	}
	if len(f) > 18 {
		m.Misc = strings.Join(f[18:], ";")
	}
	if m.Lon > 180 {
		m.Lon -= 360
	}
	return m
}
