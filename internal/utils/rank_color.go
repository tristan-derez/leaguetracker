package utils

import (
	"strings"
)

// GetRankColor returns an appropriate color code (in hexadecimal format) based on the rank of a summoner.
func GetRankColor(summonerRanking string) int {
	var color int
	rank := strings.ToUpper(summonerRanking)

	switch rank {
	case "UNRANKED":
		color = 0xCCCCCC // Light Grey
	case "IRON":
		color = 0x3C3C3C // Dark Grey
	case "BRONZE":
		color = 0xCD7F32 // Bronze
	case "SILVER":
		color = 0xC0C0C0 // Silver
	case "GOLD":
		color = 0xFFD700 // Gold
	case "PLATINUM":
		color = 0x00FFCC // Teal
	case "EMERALD":
		color = 0x50C878 // Emerald Green
	case "DIAMOND":
		color = 0x00BFFF // Light Blue
	case "MASTER":
		color = 0x800080 // Purple
	case "GRANDMASTER":
		color = 0xFF4500 // Red
	case "CHALLENGER":
		color = 0x1E90FF // Blue
	default:
		color = 0xFFFFFF // White for any undefined rank
	}
	return color
}
