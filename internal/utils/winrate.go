package utils

func CalculateWinRate(wins, losses int) float64 {
	totalGames := wins + losses
	var winRate float64

	if totalGames > 0 {
		winRate = float64(wins) / float64(totalGames) * 100
	}

	return winRate
}
