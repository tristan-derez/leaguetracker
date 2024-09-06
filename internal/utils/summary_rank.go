package utils

import "fmt"

// getSummaryRankDisplay returns a string representation of the given rank.
// It uses medal emojis for the top 3 ranks and a numbered format for the rest.
func GetSummaryRankDisplay(rank int) string {
	switch rank {
	case 1:
		return "🥇"
	case 2:
		return "🥈"
	case 3:
		return "🥉"
	default:
		return fmt.Sprintf("#%d", rank)
	}
}
