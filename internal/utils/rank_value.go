package utils

import "strings"

//  GetRankValue returns an int that represent the level of the rank.
//  	- "I" returns 4 as its the best rank possible
//  	- "IV" returns 1 as its the weakest rank possible
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
