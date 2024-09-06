package utils

type ChampionNames struct {
	DisplayName string
	ImageName   string
}

var specialCases = map[string]ChampionNames{
	"FiddleSticks": {DisplayName: "Fiddlesticks", ImageName: "Fiddlesticks"},
	"MonkeyKing":   {DisplayName: "Wukong", ImageName: "MonkeyKing"},
}

func ChampionNameMapper(apiName string, forImage bool) string {
	if specialCase, exists := specialCases[apiName]; exists {
		if forImage {
			return specialCase.ImageName
		}
		return specialCase.DisplayName
	}
	return apiName
}
