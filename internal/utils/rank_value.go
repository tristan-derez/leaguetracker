package utils

import "strings"

func GetRankValue(rank string) int {
	switch strings.ToUpper(rank) {
	case "I":
		return 4
	case "II":
		return 3
	case "III":
		return 2
	case "IV":
		return 1
	default:
		return 0
	}
}
