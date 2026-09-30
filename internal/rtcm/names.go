package rtcm

import "fmt"

var names = map[int]string{
	1001: "GPS L1 RTK 观测值",
	1002: "GPS L1 RTK 扩展观测值",
	1003: "GPS L1/L2 RTK 观测值",
	1004: "GPS L1/L2 RTK 扩展观测值",
	1005: "基准站 ARP 坐标",
	1006: "基准站 ARP 坐标 + 天线高",
	1007: "天线描述符",
	1008: "天线描述符 + 序列号",
	1009: "GLONASS L1 RTK 观测值",
	1010: "GLONASS L1 RTK 扩展观测值",
	1011: "GLONASS L1/L2 RTK 观测值",
	1012: "GLONASS L1/L2 RTK 扩展观测值",
	1013: "系统参数",
	1019: "GPS 星历",
	1020: "GLONASS 星历",
	1021: "Helmert/Molodenski 坐标转换",
	1022: "Molodenski-Badekas 坐标转换",
	1023: "残差（椭球格网）",
	1024: "残差（平面格网）",
	1025: "投影参数",
	1026: "投影参数 LCC2SP",
	1027: "投影参数 OM",
	1029: "Unicode 文本",
	1030: "GPS 网络 RTK 残差",
	1031: "GLONASS 网络 RTK 残差",
	1032: "物理参考站坐标",
	1033: "接收机与天线描述符",
	1034: "GPS 网络 FKP 梯度",
	1035: "GLONASS 网络 FKP 梯度",
	1037: "GLONASS 电离层修正差",
	1038: "GLONASS 几何修正差",
	1039: "GLONASS 组合修正差",
	1041: "NavIC 星历",
	1042: "北斗 星历",
	1043: "SBAS 星历",
	1044: "QZSS 星历",
	1045: "Galileo F/NAV 星历",
	1046: "Galileo I/NAV 星历",
	1057: "SSR GPS 轨道改正",
	1058: "SSR GPS 钟差改正",
	1059: "SSR GPS 码偏差",
	1060: "SSR GPS 轨道+钟差",
	1061: "SSR GPS URA",
	1062: "SSR GPS 高频钟差",
	1063: "SSR GLONASS 轨道改正",
	1064: "SSR GLONASS 钟差改正",
	1065: "SSR GLONASS 码偏差",
	1066: "SSR GLONASS 轨道+钟差",
	1067: "SSR GLONASS URA",
	1068: "SSR GLONASS 高频钟差",
	1230: "GLONASS 码相位偏差",
	4072: "u-blox 专有",
	4073: "Mitsubishi 专有 (CLAS)",
	4075: "Alberding 专有",
	4076: "IGS SSR",
	4077: "Hemisphere 专有",
	4078: "ComNav 专有",
	4082: "CRCSI 专有",
	4088: "IfEN 专有",
	4090: "Geo++ 专有",
	4091: "Topcon 专有",
	4092: "Leica 专有",
	4093: "NovAtel 专有",
	4094: "Trimble 专有",
	4095: "Ashtech 专有",
}

var msmLevelNames = map[int]string{
	1: "紧凑伪距",
	2: "紧凑相位",
	3: "紧凑伪距+相位",
	4: "伪距+相位+CNR",
	5: "伪距+相位+CNR+多普勒",
	6: "高精度 伪距+相位+CNR",
	7: "高精度 伪距+相位+CNR+多普勒",
}

func MessageName(t int) string {
	if n, ok := names[t]; ok {
		return n
	}
	if sys, level, ok := MSMInfo(t); ok {
		return fmt.Sprintf("%s MSM%d %s", sys, level, msmLevelNames[level])
	}
	switch {
	case t >= 1240 && t <= 1245:
		return "SSR Galileo"
	case t >= 1246 && t <= 1251:
		return "SSR QZSS"
	case t >= 1252 && t <= 1257:
		return "SSR SBAS"
	case t >= 1258 && t <= 1263:
		return "SSR 北斗"
	case t >= 4001 && t <= 4095:
		return "专有消息"
	}
	return "未知消息"
}
