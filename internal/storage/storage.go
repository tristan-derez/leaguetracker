package storage

import (
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	_ "github.com/lib/pq"
	"github.com/tristan-derez/league-tracker/internal/config"
	riotapi "github.com/tristan-derez/league-tracker/internal/riot-api"
	"github.com/tristan-derez/league-tracker/internal/utils"
)

//go:embed sql/init_db.sql
var initDBSQL string

// Storage represents a database connection and provides methods for data operations.
type Storage struct {
	db *sql.DB
}

// New creates and initializes a new Storage instance connected to the specified PostgreSQL database.
func New(config *config.Config) (*Storage, error) {
	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		config.DBHost, config.DBPort, config.DBUsername, config.DBPassword, config.DBDatabase)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, err
	}

	storage := &Storage{db: db}
	if err := storage.initDB(); err != nil {
		return nil, fmt.Errorf("error initializing database: %w", err)
	}

	return storage, nil
}

// initDB initializes the database by executing the SQL query in initDBSQL.
func (s *Storage) initDB() error {
	_, err := s.db.Exec(initDBSQL)
	if err != nil {
		return fmt.Errorf("error executing init_db.sql: %w", err)
	}

	log.Println("Database initialized successfully")
	return nil
}

// AddGuild adds or update a guild row to the database
func (s *Storage) AddGuild(guildID, guildName string) error {
	_, err := s.db.Exec(string(insertNewGuildSQL), guildID, guildName)
	return err
}

// Close closes the database connection.
func (s *Storage) Close() error {
	return s.db.Close()
}

// AddSummoner adds or updates a summoner's information, their league entry if available,
// and associates them with a guild in the database.
func (s *Storage) AddSummoner(guildID, channelID, summonerName string, summoner riotapi.Summoner, leagueEntry *riotapi.LeagueEntry) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	var summonerID int
	err = tx.QueryRow(string(insertSummonerSQL),
		summonerName, summoner.RiotAccountID, summoner.RiotSummonerID, summoner.SummonerPUUID,
		summoner.SummonerLevel, summoner.ProfileIconID, summoner.RevisionDate).Scan(&summonerID)
	if err != nil {
		return fmt.Errorf("insert/update summoner: %w", err)
	}

	if leagueEntry != nil {
		_, err = tx.Exec(string(insertLeagueEntrySQL),
			summonerID, leagueEntry.QueueType, leagueEntry.Tier, leagueEntry.Rank,
			leagueEntry.LeaguePoints, leagueEntry.Wins, leagueEntry.Losses,
			leagueEntry.HotStreak, leagueEntry.Veteran, leagueEntry.FreshBlood,
			leagueEntry.Inactive)
		if err != nil {
			return fmt.Errorf("insert/update league entry: %w", err)
		}
	}

	_, err = tx.Exec(string(insertGuildSummonerAssociationSQL), guildID, summonerID)
	if err != nil {
		return fmt.Errorf("insert guild-summoner association: %w", err)
	}

	_, err = tx.Exec(string(updateGuildWithChannelIDSQL), guildID, channelID)
	if err != nil {
		return fmt.Errorf("update guild channel: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

// ErrSummonerNotFound is returned when a summoner is not found in the database
var ErrSummonerNotFound = errors.New("summoner not found")

// RemoveSummoner removes one or more summoner(s) associated with a guild from the database.
func (s *Storage) RemoveSummoner(guildID, summonerName string) error {
	result, err := s.db.Exec(string(deleteSummonerSQL), guildID, summonerName)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return ErrSummonerNotFound
	}

	return nil
}

// RemoveAllSummoners removes all summoners associated with a guild from the database.
func (s *Storage) RemoveAllSummoners(guildID string) error {
	_, err := s.db.Exec(string(removeAllSummonersFromGuildSQL), guildID)
	if err != nil {
		return fmt.Errorf("error removing all summoners from guild: %w", err)
	}
	return nil
}

// AddMatchAndGetLPChange adds a new match record to the database for a given summoner,
// updates lp_history and league_entry, and returns the LP change.
func (s *Storage) AddMatchAndGetLPChange(riotSummonerID string, matchData *riotapi.MatchData, newLP int, newRank, newTier string) (int, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	summonerID, err := s.GetSummonerIDFromRiotID(riotSummonerID)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, fmt.Errorf("summoner with Riot ID %s not found", riotSummonerID)
		}
		return 0, fmt.Errorf("error fetching summoner ID: %w", err)
	}

	err = s.insertMatchData(summonerID, matchData)
	if err != nil {
		return 0, err
	}

	previousRank, err := s.GetPreviousRank(summonerID)
	if err != nil {
		return 0, fmt.Errorf("error fetching previous rank: %w", err)
	}

	lpChange := s.CalculateLPChange(previousRank.PrevTier, newTier, previousRank.PrevRank, newRank, previousRank.PrevLP, newLP)

	err = s.updateLPHistory(summonerID, matchData.MatchID, lpChange, newLP)
	if err != nil {
		return 0, err
	}

	err = s.updateLeagueEntry(summonerID, newLP, newTier, newRank)
	if err != nil {
		return 0, err
	}

	err = tx.Commit()
	if err != nil {
		return 0, fmt.Errorf("commit transaction: %w", err)
	}

	return lpChange, nil
}

// insertMatchData inserts match data for a summoner into the database.
func (s *Storage) insertMatchData(summonerID int, matchData *riotapi.MatchData) error {
	_, err := s.db.Exec(string(insertMatchDataSQL), summonerID, matchData.MatchID, matchData.ChampionName, matchData.GameCreation,
		matchData.GameDuration, matchData.GameEndTimestamp, matchData.GameID, matchData.QueueID,
		matchData.GameMode, matchData.GameType, matchData.Kills, matchData.Deaths, matchData.Assists,
		matchData.Result, matchData.Pentakills, matchData.TeamPosition, matchData.TeamDamagePercentage, matchData.KillParticipation,
		matchData.TotalDamageDealtToChampions, matchData.TotalMinionsKilled,
		matchData.NeutralMinionsKilled, matchData.WardsKilled, matchData.WardsPlaced,
		matchData.Win, matchData.TotalMinionsKilled+matchData.NeutralMinionsKilled)
	if err != nil {
		return fmt.Errorf("error inserting match data: %w", err)
	}
	return nil
}

// updateLPHistory updates the LP history for a summoner in the database.
func (s *Storage) updateLPHistory(summonerID int, matchID string, lpChange, newLP int) error {
	_, err := s.db.Exec(string(insertLPInLPHistorySQL), summonerID, matchID, lpChange, newLP)
	if err != nil {
		return fmt.Errorf("error inserting LP history: %w", err)
	}
	return nil
}

// updateLeagueEntry updates lp, tier and rank in league_entries for a summoner in the database.
func (s *Storage) updateLeagueEntry(summonerID int, newLP int, newTier, newRank string) error {
	_, err := s.db.Exec(string(updateLeagueEntriesSQL), newLP, newTier, newRank, summonerID)
	if err != nil {
		return fmt.Errorf("error updating league entry: %w", err)
	}
	return nil
}

// returns the lp change based on previous and new tier, rank and lp
func (s *Storage) CalculateLPChange(oldTier, newTier, oldRank, newRank string, oldLP, newLP int) int {
	oldTier = strings.ToUpper(oldTier)
	newTier = strings.ToUpper(newTier)

	// Special handling for Master, Grandmaster, and Challenger tiers
	highTiers := map[string]bool{"MASTER": true, "GRANDMASTER": true, "CHALLENGER": true}
	if highTiers[oldTier] && highTiers[newTier] {
		return newLP - oldLP
	}

	oldDivision := utils.GetRankValue(oldRank)
	newDivision := utils.GetRankValue(newRank)

	var lpChange int
	if oldTier != newTier {
		if tierOrder[newTier] > tierOrder[oldTier] {
			// Promotion to a new tier
			lpChange = (100 - oldLP) + newLP
		} else {
			// Demotion to a lower tier
			lpChange = -(oldLP) - (100 - newLP)
		}
	} else if oldDivision != newDivision {
		if newDivision > oldDivision {
			// Promotion within the same tier
			lpChange = (100 - oldLP) + newLP
		} else {
			// Demotion within the same tier
			lpChange = -(oldLP) - (100 - newLP)
		}
	} else {
		// Same division, normal LP change
		lpChange = newLP - oldLP
	}

	return lpChange
}

// ListSummoners retrieves and returns a list of summoners with their ranks for a given guild id.
func (s *Storage) ListSummoners(guildID string) ([]riotapi.Summoner, error) {
	rows, err := s.db.Query(string(selectSummonerWithRankSQL), guildID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var summoners []riotapi.Summoner
	for rows.Next() {
		var s riotapi.Summoner
		if err := rows.Scan(
			&s.RiotSummonerID, &s.RiotAccountID, &s.SummonerPUUID,
			&s.ProfileIconID, &s.RevisionDate, &s.SummonerLevel, &s.Name,
			&s.Rank, &s.LeaguePoints,
		); err != nil {
			return nil, err
		}
		summoners = append(summoners, s)
	}

	return summoners, rows.Err()
}

// GetGuildChannelID retrieves the channel ID associated with a given guild ID.
func (s *Storage) GetGuildChannelID(guildID string) (string, error) {
	var channelID string
	err := s.db.QueryRow(string(selectChannelIdFromGuildIdSQL), guildID).Scan(&channelID)

	return channelID, err
}

// GetLastMatchID retrieves the most recent match ID for a given summoner PUUID.
func (s *Storage) GetLastMatchID(puuid string) (string, error) {
	var matchID string
	err := s.db.QueryRow(string(selectLastMatchIDSQL), puuid).Scan(&matchID)

	if err == sql.ErrNoRows {
		return "", nil
	}

	return matchID, err
}

// RemoveChannelFromGuild removes the association between a channel and a guild in the database.
func (s *Storage) RemoveChannelFromGuild(guildID, channelID string) error {
	result, err := s.db.Exec(string(removeChannelFromGuildSQL), guildID, channelID)
	if err != nil {
		return fmt.Errorf("error removing channel from guild association: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("error getting rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("no association found for this guild and channel")
	}

	return nil
}

// GetAllSummonersForGuild retrieves and returns summoners in a guild list in the database.
func (s *Storage) GetAllSummonersForGuild(guildID string) ([]riotapi.Summoner, error) {
	rows, err := s.db.Query(string(selectAllSummonersForAGuildSQL), guildID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var summoners []riotapi.Summoner
	for rows.Next() {
		var s riotapi.Summoner
		if err := rows.Scan(&s.SummonerPUUID, &s.Name, &s.RiotSummonerID); err != nil {
			return nil, err
		}
		summoners = append(summoners, s)
	}

	return summoners, rows.Err()
}

// UpdateLPHistory updates the LP for a summoner in the lp_history table.
func (s *Storage) UpdateLPHistory(riotSummonerID, matchID string, lpChange, newLP int) error {
	_, err := s.db.Exec(string(insertLPInLPHistorySQL), riotSummonerID, matchID, lpChange, newLP)
	if err != nil {
		return fmt.Errorf("error updating LP history: %w", err)
	}
	return nil
}

// GetLastKnownLP retrieves last lp stored in league entries.
func (s *Storage) GetLastKnownLP(summonerID int) (int, error) {
	var previousLP int
	err := s.db.QueryRow(string(selectLeaguePointsFromLeagueEntriesSQL), summonerID).Scan(&previousLP)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, fmt.Errorf("error fetching previous LP: %w", err)
	}

	return previousLP, nil
}

// GetSummonerIDFromRiotID retrives id of a summoner from riot_summoner_id.
func (s *Storage) GetSummonerIDFromRiotID(riotSummonerID string) (int, error) {
	var summonerID int
	err := s.db.QueryRow("SELECT id FROM summoners WHERE riot_summoner_id = $1", riotSummonerID).Scan(&summonerID)
	if err != nil {
		return 0, fmt.Errorf("error fetching summoner ID: %w", err)
	}
	return summonerID, nil
}

// GetRankBySummonerId retrieves the rank from leagueEntries using the summoner ID.
// This should be called before updating entries.
func (s *Storage) GetPreviousRank(summonerID int) (*PreviousRank, error) {
	var prevRank PreviousRank
	err := s.db.QueryRow(string(selectRankInLeagueEntriesSQL), summonerID).Scan(&prevRank.PrevTier, &prevRank.PrevRank, &prevRank.PrevLP)
	if err != nil {
		if err == sql.ErrNoRows {
			// No previous rank found, return nil with no error
			return nil, nil
		}
		// If there's any other error, return it
		return nil, fmt.Errorf("error querying previous rank: %w", err)
	}

	return &prevRank, nil
}

type PreviousRank struct {
	PrevTier string
	PrevRank string
	PrevLP   int
}

type LPChange struct {
	Timestamp time.Time
	NewLP     int
}

var tierOrder = map[string]int{
	"IRON":        0,
	"BRONZE":      1,
	"SILVER":      2,
	"GOLD":        3,
	"PLATINUM":    4,
	"EMERALD":     5,
	"DIAMOND":     6,
	"MASTER":      7,
	"GRANDMASTER": 8,
	"CHALLENGER":  9,
}
