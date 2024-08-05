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
	apiKey     string
	httpClient *http.Client
	region     string
}

func NewClient(apiKey, region string) *Client {
	return &Client{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: time.Second *10,
		},
		region: region,
	}
}

func (c *Client) GetAccountPUUIDBySummonerName(gameName, tagLine string)(*Account, error) {
	encodedName := url.PathEscape(gameName)
    encodedTag := url.PathEscape(tagLine)
	baseURL := "https://europe.api.riotgames.com/riot/account/v1/accounts/by-riot-id"
	fullURL := fmt.Sprintf("%s/%s/%s", baseURL, encodedName, encodedTag)

	req, err := http.NewRequest("GET", fullURL , nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Set("X-Riot-Token", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status code: %d", resp.StatusCode)
	}

	var account Account
	if err := json.NewDecoder(resp.Body).Decode(&account); err != nil {
		return nil, fmt.Errorf("error decoding response: %w", err)
	}

	return &account, nil
}

func (c *Client) GetSummonerByPUUID(puuid string)(*Summoner, error) {
	url := fmt.Sprintf("https://%s.api.riotgames.com/lol/summoner/v4/summoners/by-puuid/%s", c.region, puuid)

	req, err := http.NewRequest("GET", url , nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Set("X-Riot-Token", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status code: %d", resp.StatusCode)
	}

	var summoner Summoner
	if err := json.NewDecoder(resp.Body).Decode(&summoner); err != nil {
		return nil, fmt.Errorf("error decoding response: %w", err)
	}

	return &summoner, nil
}

func (c *Client) GetSummonerRank(summonerID string) (*LeagueEntry, error) {
    url := fmt.Sprintf("https://%s.api.riotgames.com/lol/league/v4/entries/by-summoner/%s", c.region, summonerID)

    req, err := http.NewRequest("GET", url, nil)
    if err != nil {
        return nil, fmt.Errorf("error creating request: %w", err)
    }
    req.Header.Set("X-Riot-Token", c.apiKey)

    resp, err := c.httpClient.Do(req)
    if err != nil {
        return nil, fmt.Errorf("error sending request: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        body, _ := io.ReadAll(resp.Body)
        return nil, fmt.Errorf("API request failed with status code %d: %s", resp.StatusCode, string(body))
    }

    var leagueEntries []LeagueEntry
    if err := json.NewDecoder(resp.Body).Decode(&leagueEntries); err != nil {
        return nil, fmt.Errorf("error decoding response: %w", err)
    }

    if len(leagueEntries) == 0 {
        return nil, fmt.Errorf("no ranked entries found for summoner ID %s", summonerID)
    }

    for _, entry := range leagueEntries {
        if entry.QueueType == "RANKED_SOLO_5x5" {
            return &entry, nil
        }
    }

    return nil, fmt.Errorf("no solo queue entry found for summoner ID %s", summonerID)
}

type Account struct {
	SummonerPUUID    string `json:"puuid"`
	SummonerName     string `json:"summonerName"`
	SummonerTagLine  string `json:"summonerTagLine"`
}

type Summoner struct {
	RiotSummonerID string `json:"id"`
	RiotAccountID  string `json:"accountId"`
    SummonerPUUID  string `json:"puuid"`
    ProfileIconID  int    `json:"profileIconId"`
    RevisionDate   int64  `json:"revisionDate"`
    SummonerLevel  int    `json:"summonerLevel"`
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