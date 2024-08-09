package riotapi

import (
	"encoding/json"
	"fmt"
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

func (c *Client) GetSummonerRank(summonerID string) (*LeagueEntry, error) {
	url := fmt.Sprintf("https://%s.api.riotgames.com/lol/league/v4/entries/by-summoner/%s", c.region, summonerID)

	resp, err := c.makeRequest(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var leagueEntries []LeagueEntry
	if err := json.NewDecoder(resp.Body).Decode(&leagueEntries); err != nil {
		return nil, fmt.Errorf("error decoding response: %w", err)
	}

	if len(leagueEntries) == 0 {
		return &LeagueEntry{
			QueueType: "RANKED_SOLO_5x5",
			Tier:      "UNRANKED",
			Rank:      "",
		}, nil
	}

	for _, entry := range leagueEntries {
		if entry.QueueType == "RANKED_SOLO_5x5" {
			return &entry, nil
		}
	}

	return nil, fmt.Errorf("no solo queue entry found for summoner ID %s", summonerID)
}

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

func findParticipant(participants []participant, summonerPUUID string) (*participant, error) {
	for _, p := range participants {
		if p.Puuid == summonerPUUID {
			return &p, nil
		}
	}
	return nil, fmt.Errorf("summoner not found in match data")
}

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
		GameMode:                    info.GameMode,
		GameType:                    info.GameType,
		Kills:                       participant.Kills,
		Deaths:                      participant.Deaths,
		Result:                      result,
		Pentakills:                  participant.PentaKills,
		TeamPosition:                participant.TeamPosition,
		TotalDamageDealtToChampions: participant.TotalDamageDealtToChampions,
		TotalMinionsKilled:          participant.TotalMinionsKilled,
		NeutralMinionsKilled:        participant.NeutralMinionsKilled,
		WardsKilled:                 participant.WardsKilled,
		WardsPlaced:                 participant.WardsPlaced,
		Win:                         participant.Win,
	}
}

func (c *Client) GetLastMatchID(puuid string) (string, error) {
	url := fmt.Sprintf("https://europe.api.riotgames.com/lol/match/v5/matches/by-puuid/%s/ids?count=1", puuid)

	resp, err := c.makeRequest(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var matchIDs []string
	if err := json.NewDecoder(resp.Body).Decode(&matchIDs); err != nil {
		return "", fmt.Errorf("error decoding response: %w", err)
	}

	if len(matchIDs) == 0 {
		return "", fmt.Errorf("no matches found for the given PUUID")
	}

	return matchIDs[0], nil
}

// GetLastMatchForNewSummoner gets the last match for a new summoner being added for tracking.
// It returns the match data even if there are no matches (returns nil, nil in this case).
func (c *Client) GetLastMatchDataForNewSummoner(summonerPUUID string) (*MatchData, error) {
	matchID, err := c.GetLastMatchID(summonerPUUID)
	if err != nil {
		return nil, fmt.Errorf("error getting last match ID: %w", err)
	}

	matchData, err := c.GetMatchData(matchID, summonerPUUID)
	if err != nil {
		return nil, fmt.Errorf("error getting match data: %w", err)
	}

	return matchData, nil
}

// GetNewMatchForSummoner checks if there's a new match for a summoner we're already tracking.
// It returns a boolean indicating if there's a new match, and the new match data if there is one and if its a ranked solo/duo game.
func (c *Client) GetNewMatchForSummoner(summonerPUUID string, lastKnownMatchID string) (bool, *MatchData, error) {
	matchID, err := c.GetLastMatchID(summonerPUUID)
	if err != nil {
		return false, nil, fmt.Errorf("error getting last match ID: %w", err)
	}

	if matchID == "" || matchID == lastKnownMatchID {
		return false, nil, nil // No new matches
	}

	matchData, err := c.GetMatchData(matchID, summonerPUUID)
	if err != nil {
		return false, nil, fmt.Errorf("error getting match data: %w", err)
	}

	// queue_id for ranked 5x5 solo/duo games is 4
	if matchData.QueueID == 4 {
		return true, matchData, nil
	}

	return true, matchData, nil
}

type Account struct {
	SummonerPUUID   string `json:"puuid"`
	SummonerName    string `json:"summonerName"`
	SummonerTagLine string `json:"summonerTagLine"`
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
	GameMode         string        `json:"gameMode"`
	GameType         string        `json:"gameType"`
	Participants     []participant `json:"participants"`
}

type participant struct {
	ChampionName                string `json:"championName"`
	Kills                       int    `json:"kills"`
	Deaths                      int    `json:"deaths"`
	Assists                     int    `json:"assists"`
	PentaKills                  int    `json:"pentaKills"`
	TeamPosition                string `json:"teamPosition"`
	TotalDamageDealtToChampions int    `json:"totalDamageDealtToChampions"`
	TotalMinionsKilled          int    `json:"totalMinionsKilled"`
	NeutralMinionsKilled        int    `json:"neutralMinionsKilled"`
	WardsKilled                 int    `json:"wardsKilled"`
	WardsPlaced                 int    `json:"wardsPlaced"`
	Win                         bool   `json:"win"`
	Puuid                       string `json:"puuid"`
}
