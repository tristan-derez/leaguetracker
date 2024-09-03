package storage

import (
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
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

	var summonerUUID uuid.UUID
	err = tx.QueryRow(string(insertSummonerSQL),
		summonerName, summoner.RiotAccountID, summoner.RiotSummonerID, summoner.SummonerPUUID,
		summoner.SummonerLevel, summoner.ProfileIconID, summoner.RevisionDate).Scan(&summonerUUID)
	if err != nil {
		return fmt.Errorf("insert/update summoner: %w", err)
	}

	if leagueEntry != nil {
		_, err = tx.Exec(string(insertLeagueEntrySQL),
			summonerUUID, leagueEntry.QueueType, leagueEntry.Tier, leagueEntry.Rank,
			leagueEntry.LeaguePoints, leagueEntry.Wins, leagueEntry.Losses,
			leagueEntry.HotStreak, leagueEntry.Veteran, leagueEntry.FreshBlood,
			leagueEntry.Inactive)
		if err != nil {
			return fmt.Errorf("insert/update league entry: %w", err)
		}
	} else {
		currentSeason := s.GetCurrentSeason()
		err = s.InitializePlacementGames(summonerUUID, currentSeason)
		if err != nil {
			return fmt.Errorf("initialize placement games: %w", err)
		}
	}

	_, err = tx.Exec(string(insertGuildSummonerAssociationSQL), guildID, summonerUUID)
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

	summonerUUID, err := s.GetSummonerUUIDFromRiotID(riotSummonerID)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, fmt.Errorf("summoner with Riot ID %s not found", riotSummonerID)
		}
		return 0, fmt.Errorf("error fetching summoner UUID: %w", err)
	}

	err = s.insertMatchData(summonerUUID, matchData)
	if err != nil {
		return 0, fmt.Errorf("error inserting match data: %w", err)
	}

	previousRank, err := s.GetPreviousRank(summonerUUID)
	if err != nil {
		return 0, fmt.Errorf("error fetching previous rank: %w", err)
	}

	lpChange := s.CalculateLPChange(previousRank.PrevTier, newTier, previousRank.PrevRank, newRank, previousRank.PrevLP, newLP)

	err = s.createNewRowInLPHistory(summonerUUID, matchData.MatchID, lpChange, newLP, newTier, newRank)
	if err != nil {
		return 0, err
	}

	err = s.updateLeagueEntry(summonerUUID, newLP, newTier, newRank)
	if err != nil {
		return 0, err
	}

	err = tx.Commit()
	if err != nil {
		return 0, fmt.Errorf("commit transaction: %w", err)
	}

	return lpChange, nil
}

// InitializePlacementGames initializes or updates the placement games record for a summoner
func (s *Storage) InitializePlacementGames(summonerUUID uuid.UUID, season Season) error {
	seasonStr := SeasonToString(season)
	_, err := s.db.Exec(`
		INSERT INTO placement_games (summoner_id, season, total_games, wins, losses)
		VALUES ($1, $2, 0, 0, 0)
		ON CONFLICT (summoner_id, season) DO UPDATE
		SET total_games = 0, wins = 0, losses = 0
	`, summonerUUID, seasonStr)
	return err
}

// UpdatePlacementGames updates the placement games record for a summoner
func (s *Storage) UpdatePlacementGames(summonerUUID uuid.UUID, season Season, status *riotapi.PlacementStatus) error {
	_, err := s.db.Exec(`
        INSERT INTO placement_games (summoner_id, season, total_games, wins, losses)
        VALUES ($1, $2, $3, $4, $5)
        ON CONFLICT (summoner_id, season) DO UPDATE
        SET total_games = $3, wins = $4, losses = $5
    `, summonerUUID, SeasonToString(season), status.GamesPlayed, status.Wins, status.Losses)
	return err
}

// GetCurrentPlacementGames retrieves the current placement games status for a summoner
func (s *Storage) GetCurrentPlacementGames(summonerUUID uuid.UUID) (*PlacementGames, error) {
	currentSeason := s.GetCurrentSeason()
	var pg PlacementGames
	err := s.db.QueryRow(`
        SELECT total_games, wins, losses
        FROM placement_games
        WHERE summoner_id = $1 AND season = $2
    `, summonerUUID, SeasonToString(currentSeason)).Scan(&pg.TotalGames, &pg.Wins, &pg.Losses)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &pg, nil
}

// GetCurrentSeason returns the current season based on the current date
func (s *Storage) GetCurrentSeason() Season {
	now := time.Now()
	year := now.Year()
	month := now.Month()
	day := now.Day()

	// split 1 starts the 10th of january
	// split 2 starts the 25th of may
	// split 3 start the 25th of september

	if month == time.January && day < 10 {
		return Season{Year: year - 1, Split: 3}
	} else if month < time.May || (month == time.May && day <= 24) {
		return Season{Year: year, Split: 1}
	} else if month < time.September || (month == time.September && day < 25) {
		return Season{Year: year, Split: 2}
	} else {
		return Season{Year: year, Split: 3}
	}
}

// SeasonToString converts a Season struct to a string representation
func SeasonToString(s Season) string {
	return fmt.Sprintf("S%d S%d", s.Year, s.Split)
}

// AddPlacementMatch adds a new match record to the database for a given summoner that is in placement games.
// no lp calculated because riot doesnt give informations about lp in placement.
func (s *Storage) AddPlacementMatch(riotSummonerID string, matchData *riotapi.MatchData) error {
	summonerUUID, err := s.GetSummonerUUIDFromRiotID(riotSummonerID)
	if err != nil {
		return fmt.Errorf("error fetching summoner ID: %w", err)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	err = s.insertMatchData(summonerUUID, matchData)
	if err != nil {
		return fmt.Errorf("error inserting match data: %w", err)
	}

	err = s.createNewRowInLPHistory(summonerUUID, matchData.MatchID, 0, 0, "UNRANKED", "")
	if err != nil {
		return fmt.Errorf("error creating new row in lp history: %w", err)
	}

	err = s.updateLeagueEntry(summonerUUID, 0, "UNRANKED", "")
	if err != nil {
		return fmt.Errorf("error updating league entry: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

// insertMatchData inserts match data for a summoner into the database.
func (s *Storage) insertMatchData(summonerUUID uuid.UUID, matchData *riotapi.MatchData) error {
	_, err := s.db.Exec(string(insertMatchDataSQL), summonerUUID, matchData.MatchID, matchData.ChampionName, matchData.GameCreation,
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

// createNewRowInLPHistory inserts a new record into the lp_history table for a summoner.
// It captures the LP change, new LP total, tier, and rank for a specific match,
// enabling detailed tracking of a summoner's rank progression over time.
func (s *Storage) createNewRowInLPHistory(summonerUUID uuid.UUID, matchID string, lpChange, newLP int, tier, rank string) error {
	_, err := s.db.Exec(string(insertLDataInLPHistorySQL), summonerUUID, matchID, lpChange, newLP, tier, rank)
	if err != nil {
		return fmt.Errorf("error inserting LP history: %w", err)
	}
	return nil
}

// updateLeagueEntry updates lp, tier and rank in league_entries for a summoner in the database.
func (s *Storage) updateLeagueEntry(summonerUUID uuid.UUID, newLP int, newTier, newRank string) error {
	_, err := s.db.Exec(string(updateLeagueEntriesSQL), newLP, newTier, newRank, summonerUUID)
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

// GetGuildName retrieves the name of a guild with a guildID
func (s *Storage) GetGuildName(guildID string) (string, error) {
	var guildName string
	err := s.db.QueryRow("SELECT guild_name FROM guilds WHERE guild_id = $1", guildID).Scan(&guildName)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("no guild found with ID %s", guildID)
		}
		return "", fmt.Errorf("error fetching guild name: %w", err)
	}
	return guildName, nil
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

// GetSummonerUUIDFromRiotID retrieves the UUID of a summoner from riot_summoner_id.
func (s *Storage) GetSummonerUUIDFromRiotID(riotSummonerID string) (uuid.UUID, error) {
	var summonerUUID uuid.UUID
	err := s.db.QueryRow("SELECT id FROM summoners WHERE riot_summoner_id = $1", riotSummonerID).Scan(&summonerUUID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("error fetching summoner UUID: %w", err)
	}
	return summonerUUID, nil
}

// GetRankBySummonerUUID retrieves the rank from leagueEntries using the summoner UUID.
// This should be called before updating entries.
func (s *Storage) GetPreviousRank(summonerUUID uuid.UUID) (*PreviousRank, error) {
	var prevRank PreviousRank
	err := s.db.QueryRow(string(selectRankInLeagueEntriesSQL), summonerUUID).Scan(&prevRank.PrevTier, &prevRank.PrevRank, &prevRank.PrevLP)
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

// GetDailySummonerProgress fetches the daily summoner progress for a guild from the database.
func (s *Storage) GetDailySummonerProgress(guildID string) ([]DailySummonerProgress, error) {
	rows, err := s.db.Query(string(getDailySummonerProgressSQL), guildID)
	if err != nil {
		return nil, fmt.Errorf("error querying daily summoner progress: %w", err)
	}
	defer rows.Close()

	var progress []DailySummonerProgress
	for rows.Next() {
		var p DailySummonerProgress
		err := rows.Scan(
			&p.Name, &p.CurrentTier, &p.CurrentRank, &p.CurrentLP,
			&p.PreviousTier, &p.PreviousRank, &p.PreviousLP,
			&p.Wins, &p.Losses, &p.LPChange,
		)
		if err != nil {
			return nil, fmt.Errorf("error scanning daily summoner progress: %w", err)
		}
		progress = append(progress, p)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating daily summoner progress rows: %w", err)
	}

	return progress, nil
}

// GetAllGuilds retrieves all guilds from the database.
func (s *Storage) GetAllGuilds() ([]Guild, error) {
	query := `
        SELECT guild_id, guild_name, channel_id
        FROM guilds
    `

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("error querying guilds: %w", err)
	}
	defer rows.Close()

	var guilds []Guild
	for rows.Next() {
		var g Guild
		if err := rows.Scan(&g.ID, &g.Name, &g.ChannelID); err != nil {
			return nil, fmt.Errorf("error scanning guild row: %w", err)
		}
		guilds = append(guilds, g)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating guild rows: %w", err)
	}

	return guilds, nil
}

func (s *Storage) GetAllSummonersWithGuilds() ([]SummonerWithGuilds, error) {
	rows, err := s.db.Query(string(selectSummonerInGuildSQL))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var summoners []SummonerWithGuilds
	for rows.Next() {
		var s SummonerWithGuilds
		var guildIDs string
		err := rows.Scan(
			&s.Summoner.RiotSummonerID, &s.Summoner.RiotAccountID, &s.Summoner.SummonerPUUID,
			&s.Summoner.ProfileIconID, &s.Summoner.RevisionDate, &s.Summoner.SummonerLevel, &s.Summoner.Name,
			&guildIDs,
		)
		if err != nil {
			return nil, err
		}
		s.GuildIDs = strings.Split(strings.Trim(guildIDs, "{}"), ",")
		summoners = append(summoners, s)
	}

	return summoners, rows.Err()
}

type Guild struct {
	ID        string
	Name      string
	ChannelID string
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

type DailySummonerProgress struct {
	Name         string
	CurrentTier  string
	CurrentRank  string
	CurrentLP    int
	PreviousTier string
	PreviousRank string
	PreviousLP   int
	Wins         int
	Losses       int
	LPChange     int
}

type SummonerWithGuilds struct {
	Summoner riotapi.Summoner
	GuildIDs []string
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

type Season struct {
	Year  int
	Split int
}

type PlacementGames struct {
	Season     string
	TotalGames int
	Wins       int
	Losses     int
}
