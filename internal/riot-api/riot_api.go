package riotapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

type Client struct {
	apiKey      string
	httpClient  *http.Client
	region      string
	rateLimiter *RateLimiter
}

// NewClient creates and returns a new Client instance for interacting with the Riot API.
// It initializes the client with the provided API key and region, and sets up a rate limiter.
func NewClient(apiKey, region string) *Client {
	return &Client{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: time.Second * 10,
		},
		region:      region,
		rateLimiter: NewRateLimiter(20, 100), // 20 requests per second, burst of 100
	}
}

// GetAccountPUUIDBySummonerName fetch the puuid of a summoner with the gameName and tagLine.
//   - gameName#tagLine
func (c *Client) GetAccountPUUIDBySummonerName(gameName, tagLine string) (*Account, error) {
	encodedName := url.PathEscape(gameName)
	encodedTag := url.PathEscape(tagLine)
	url := fmt.Sprintf("https://europe.api.riotgames.com/riot/account/v1/accounts/by-riot-id/%s/%s", encodedName, encodedTag)

	resp, err := c.makeRequest(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var account Account
	if err := json.NewDecoder(resp.Body).Decode(&account); err != nil {
		return nil, fmt.Errorf("error decoding response: %w", err)
	}

	return &account, nil
}

// GetSummonerByPUUID fetch summoner data by their puuid.
func (c *Client) GetSummonerByPUUID(puuid string) (*Summoner, error) {
	url := fmt.Sprintf("https://%s.api.riotgames.com/lol/summoner/v4/summoners/by-puuid/%s", c.region, puuid)

	resp, err := c.makeRequest(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var summoner Summoner
	if err := json.NewDecoder(resp.Body).Decode(&summoner); err != nil {
		return nil, fmt.Errorf("error decoding response: %w", err)
	}

	return &summoner, nil
}

// GetSummonerRank fetch summoner current tier and rank from Riot API.
func (c *Client) GetSummonerRank(RiotSummonerID string) (*LeagueEntry, error) {
	url := fmt.Sprintf("https://%s.api.riotgames.com/lol/league/v4/entries/by-summoner/%s", c.region, RiotSummonerID)

	resp, err := c.makeRequest(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var leagueEntries []LeagueEntry
	if err := json.NewDecoder(resp.Body).Decode(&leagueEntries); err != nil {
		return nil, fmt.Errorf("error decoding response: %w", err)
	}

	// Look for solo queue entry
	for _, entry := range leagueEntries {
		if entry.QueueType == "RANKED_SOLO_5x5" {
			return &entry, nil
		}
	}

	return &LeagueEntry{
		QueueType:    "RANKED_SOLO_5x5",
		Tier:         "UNRANKED",
		Rank:         "",
		LeaguePoints: 0,
	}, nil
}

// GetMatchData fetch summoner match data using the matchID, summonerPUUID is used to find participant.
func (c *Client) GetMatchData(matchID string, summonerPUUID string) (*MatchData, error) {
	url := fmt.Sprintf("https://europe.api.riotgames.com/lol/match/v5/matches/%s", matchID)

	resp, err := c.makeRequest(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var matchResp matchResponse
	if err := json.NewDecoder(resp.Body).Decode(&matchResp); err != nil {
		return nil, fmt.Errorf("error decoding response: %w", err)
	}

	participant, err := findParticipant(matchResp.Info.Participants, summonerPUUID)
	if err != nil {
		return nil, err
	}

	return createMatchData(matchID, matchResp.Info, *participant), nil
}

// findParticipant searches for a participant in a match by their PUUID.
// It returns a pointer to the participant if found, or an error if not found.
func findParticipant(participants []participant, summonerPUUID string) (*participant, error) {
	for _, p := range participants {
		if p.Puuid == summonerPUUID {
			return &p, nil
		}
	}
	return nil, fmt.Errorf("summoner not found in match data")
}

func (c *Client) GetPlacementStatus(puuid string) (*PlacementStatus, error) {
	matchIDs, err := c.GetRankedSoloMatchIDs(puuid, 5)
	if err != nil {
		return nil, fmt.Errorf("error fetching match IDs: %w", err)
	}

	placementStatus := &PlacementStatus{
		IsInPlacements: true,
		TotalGames:     0,
		Wins:           0,
		Losses:         0,
	}

	currentSplit := getCurrentSplit()

	for _, matchID := range matchIDs {
		match, err := c.GetMatchData(matchID, puuid)
		if err != nil {
			return nil, fmt.Errorf("error fetching match data: %w", err)
		}

		if match.QueueID != 420 || match.GameDuration <= 300 {
			continue
		}

		matchSplit := getSplitFromTimestamp(match.GameEndTimestamp)
		if matchSplit != currentSplit {
			continue // Skip matches from previous splits
		}

		placementStatus.TotalGames++
		if match.Win {
			placementStatus.Wins++
		} else {
			placementStatus.Losses++
		}

		if placementStatus.TotalGames >= 5 {
			placementStatus.IsInPlacements = false
			break
		}
	}

	return placementStatus, nil
}

func getCurrentSplit() int {
	now := time.Now()
	year := now.Year()
	splitStartDates := []time.Time{
		time.Date(year, time.January, 10, 0, 0, 0, 0, time.UTC),
		time.Date(year, time.May, 25, 0, 0, 0, 0, time.UTC),
		time.Date(year, time.September, 25, 0, 0, 0, 0, time.UTC),
	}

	for i, startDate := range splitStartDates {
		if now.Before(startDate) {
			if i == 0 {
				return 3 // Last split of previous year
			}
			return i
		}
	}
	return 3 // Last split of the year
}

func getSplitFromTimestamp(timestamp int64) int {
	matchTime := time.Unix(timestamp/1000, 0)
	year := matchTime.Year()
	splitStartDates := []time.Time{
		time.Date(year, time.January, 10, 0, 0, 0, 0, time.UTC),
		time.Date(year, time.May, 25, 0, 0, 0, 0, time.UTC),
		time.Date(year, time.September, 25, 0, 0, 0, 0, time.UTC),
	}

	for i, startDate := range splitStartDates {
		if matchTime.Before(startDate) {
			if i == 0 {
				return 3 // Last split of previous year
			}
			return i
		}
	}
	return 3 // Last split of the year
}

// createMatchData constructs a MatchData struct from the given match information and participant data.
// It takes the matchID, general match info, and specific participant data as input.
func createMatchData(matchID string, info matchInfo, participant participant) *MatchData {
	result := "Loss"
	if participant.Win {
		result = "Win"
	}

	return &MatchData{
		MatchID:                     matchID,
		ChampionName:                participant.ChampionName,
		GameCreation:                info.GameCreation,
		GameDuration:                info.GameDuration,
		GameEndTimestamp:            info.GameEndTimestamp,
		GameID:                      info.GameId,
		QueueID:                     info.QueueId,
		GameMode:                    info.GameMode,
		GameType:                    info.GameType,
		Kills:                       participant.Kills,
		Deaths:                      participant.Deaths,
		Assists:                     participant.Assists,
		Result:                      result,
		Pentakills:                  participant.PentaKills,
		TeamPosition:                participant.TeamPosition,
		TeamDamagePercentage:        participant.Challenges.TeamDamagePercentage,
		KillParticipation:           participant.Challenges.KillParticipation,
		TotalDamageDealtToChampions: participant.TotalDamageDealtToChampions,
		TotalMinionsKilled:          participant.TotalMinionsKilled,
		NeutralMinionsKilled:        participant.NeutralMinionsKilled,
		WardsKilled:                 participant.WardsKilled,
		WardsPlaced:                 participant.WardsPlaced,
		Win:                         participant.Win,
	}
}

// GetRankedSoloMatchIDs retrieves last game(s) id(s) from a summoner.
func (c *Client) GetRankedSoloMatchIDs(puuid string, count int) ([]string, error) {
	url := fmt.Sprintf("https://europe.api.riotgames.com/lol/match/v5/matches/by-puuid/%s/ids?queue=420&type=ranked&count=%d", puuid, count)

	resp, err := c.makeRequest(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var matchIDs []string
	if err := json.NewDecoder(resp.Body).Decode(&matchIDs); err != nil {
		return nil, fmt.Errorf("error decoding response: %w", err)
	}

	if len(matchIDs) == 0 {
		return nil, nil
	}

	return matchIDs, nil
}

// GetLastRankedSoloMatchData fetch the last match of a summoner
// by fetching the last ranked game with summonerPUUID,
// and returns the match data.
func (c *Client) GetLastRankedSoloMatchData(summonerPUUID string) (*MatchData, error) {
	matchIDs, err := c.GetRankedSoloMatchIDs(summonerPUUID, 1)
	if err != nil {
		return nil, fmt.Errorf("error getting match IDs: %w", err)
	}

	if len(matchIDs) == 0 {
		return nil, nil
	}

	matchData, err := c.GetMatchData(matchIDs[0], summonerPUUID)
	if err != nil {
		return nil, fmt.Errorf("error getting match data: %w", err)
	}

	return matchData, nil
}

// GetNewMatchForSummoner checks if there's a new ranked solo/duo match for a summoner we're already tracking.
// It returns a boolean indicating if there's a new match, and the new match data if there is one.
func (c *Client) GetNewMatchForSummoner(summonerPUUID string, lastKnownMatchID string) (bool, *MatchData, error) {
	matchIDs, err := c.GetRankedSoloMatchIDs(summonerPUUID, 1)
	if err != nil {
		return false, nil, fmt.Errorf("error getting last match ID: %w", err)
	}

	if len(matchIDs) == 0 {
		return false, nil, nil // No new matches
	}

	latestMatchID := matchIDs[0]

	if latestMatchID == lastKnownMatchID {
		return false, nil, nil // No new matches
	}

	matchData, err := c.GetMatchData(latestMatchID, summonerPUUID)
	if err != nil {
		return false, nil, fmt.Errorf("error getting match data: %w", err)
	}

	return true, matchData, nil
}

// GetCurrentDDragonVersion fetch the current version of DDragon Version.
func (c *Client) GetCurrentDDragonVersion() (string, error) {
	const defaultVersion string = "14.15.1"
	url := "https://ddragon.leagueoflegends.com/api/versions.json"

	resp, err := c.makeRequest(url)
	if err != nil {
		return defaultVersion, fmt.Errorf("error making request: %w. using default version", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return defaultVersion, fmt.Errorf("error reading response body: %w. using default version", err)
	}

	var versions []string
	err = json.Unmarshal(body, &versions)
	if err != nil {
		return defaultVersion, fmt.Errorf("error unmarshaling JSON: %w. using default version", err)
	}

	if len(versions) == 0 {
		return defaultVersion, fmt.Errorf("no versions found in the response. using default version")
	}

	return versions[0], nil
}

type Account struct {
	SummonerPUUID   string `json:"puuid"`
	SummonerName    string `json:"gameName"`
	SummonerTagLine string `json:"tagLine"`
}

type Summoner struct {
	RiotSummonerID string `json:"id"`
	RiotAccountID  string `json:"accountId"`
	SummonerPUUID  string `json:"puuid"`
	ProfileIconID  int    `json:"profileIconId"`
	RevisionDate   int64  `json:"revisionDate"`
	SummonerLevel  int    `json:"summonerLevel"`
	Name           string
	Rank           string
	LeaguePoints   int
}

type LeagueEntry struct {
	LeagueID     string `json:"leagueId"`
	SummonerID   string `json:"summonerId"`
	SummonerName string `json:"summonerName"`
	QueueType    string `json:"queueType"`
	Tier         string `json:"tier"`
	Rank         string `json:"rank"`
	LeaguePoints int    `json:"leaguePoints"`
	Wins         int    `json:"wins"`
	Losses       int    `json:"losses"`
	HotStreak    bool   `json:"hotStreak"`
	Veteran      bool   `json:"veteran"`
	FreshBlood   bool   `json:"freshBlood"`
	Inactive     bool   `json:"inactive"`
}

type MatchData struct {
	MatchID                     string
	ChampionName                string
	GameCreation                int64
	GameDuration                int
	GameEndTimestamp            int64
	GameID                      int64
	QueueID                     int
	GameMode                    string
	GameType                    string
	Kills                       int
	Deaths                      int
	Assists                     int
	Result                      string
	Pentakills                  int
	TeamPosition                string
	TotalDamageDealtToChampions int
	TeamDamagePercentage        float64
	KillParticipation           float64
	TotalMinionsKilled          int
	NeutralMinionsKilled        int
	WardsKilled                 int
	WardsPlaced                 int
	Win                         bool
}

type matchResponse struct {
	Info matchInfo `json:"info"`
}

type matchInfo struct {
	GameCreation     int64         `json:"gameCreation"`
	GameDuration     int           `json:"gameDuration"`
	GameEndTimestamp int64         `json:"gameEndTimestamp"`
	GameId           int64         `json:"gameId"`
	QueueId          int           `json:"queueId"`
	GameMode         string        `json:"gameMode"`
	GameType         string        `json:"gameType"`
	Participants     []participant `json:"participants"`
}

type participant struct {
	ChampionName                string    `json:"championName"`
	Kills                       int       `json:"kills"`
	Deaths                      int       `json:"deaths"`
	Assists                     int       `json:"assists"`
	PentaKills                  int       `json:"pentaKills"`
	TeamPosition                string    `json:"teamPosition"`
	TotalDamageDealtToChampions int       `json:"totalDamageDealtToChampions"`
	TotalMinionsKilled          int       `json:"totalMinionsKilled"`
	NeutralMinionsKilled        int       `json:"neutralMinionsKilled"`
	WardsKilled                 int       `json:"wardsKilled"`
	WardsPlaced                 int       `json:"wardsPlaced"`
	Win                         bool      `json:"win"`
	Puuid                       string    `json:"puuid"`
	Challenges                  challenge `json:"challenges"`
}

type challenge struct {
	TeamDamagePercentage float64 `json:"teamDamagePercentage"`
	KillParticipation    float64 `json:"killParticipation"`
}

type PlacementStatus struct {
	IsInPlacements bool
	TotalGames     int
	Wins           int
	Losses         int
}
