package storage

import (
	"database/sql"
	_ "embed"
	"fmt"
	"log"
	"time"

	_ "github.com/lib/pq"
	"github.com/tristan-derez/league-tracker/internal/config"
	riotapi "github.com/tristan-derez/league-tracker/internal/riot-api"
)

//go:embed sql/init_db.sql
var initDBSQL string

type Storage struct {
	db *sql.DB
}

// New creates and initializes a new Storage instance connected to the specified PostgreSQL database
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
	_, err := s.db.Exec(`
        INSERT INTO guilds (guild_id, guild_name, created_at, updated_at)
        VALUES ($1, $2, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
        ON CONFLICT (guild_id) 
        DO UPDATE SET guild_name = $2, updated_at = CURRENT_TIMESTAMP
    `, guildID, guildName)
	return err
}

// AddSummoner adds or updates a summoner's information, their league entry if available, and associates them with a guild in the database.
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

// RemoveSummoner removes a summoner from the database, but only the summoner associated to a guild.
func (s *Storage) RemoveSummoner(guildID, summonerName string) error {
	_, err := s.db.Exec(string(deleteSummonerSQL), guildID, summonerName)
	return err
}

// AddMatch adds a new match record to the database for a given summoner, using their riot_summoner_id.
func (s *Storage) AddMatch(riotSummonerID string, matchData *riotapi.MatchData, newLP int, newRank string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	summonerID, err := s.GetSummonerIDFromRiotID(riotSummonerID)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("summoner with Riot ID %s not found", riotSummonerID)
		}
		return fmt.Errorf("error fetching summoner ID: %w", err)
	}

	_, err = s.db.Exec(string(insertMatchDataSQL), summonerID, matchData.MatchID, matchData.ChampionName, matchData.GameCreation,
		matchData.GameDuration, matchData.GameEndTimestamp, matchData.GameID, matchData.QueueID,
		matchData.GameMode, matchData.GameType, matchData.Kills, matchData.Deaths, matchData.Assists,
		matchData.Result, matchData.Pentakills, matchData.TeamPosition, matchData.TeamDamagePercentage, matchData.KillParticipation,
		matchData.TotalDamageDealtToChampions, matchData.TotalMinionsKilled,
		matchData.NeutralMinionsKilled, matchData.WardsKilled, matchData.WardsPlaced,
		matchData.Win, matchData.TotalMinionsKilled+matchData.NeutralMinionsKilled)
	if err != nil {
		return fmt.Errorf("error inserting match data: %w", err)
	}

	previousRank, err := s.GetPreviousRank(summonerID)
	if err != nil {
		return fmt.Errorf("error fetching previous rank: %w", err)
	}

	lpChange := s.CalculateLPChange(previousRank.PrevTier, newRank, previousRank.PrevLP, newLP)

	_, err = tx.Exec(string(insertLPChangeIntoLPHistorySQL), summonerID, matchData.MatchID, lpChange, newLP)
	if err != nil {
		return fmt.Errorf("error insterting LP history: %w", err)
	}

	_, err = tx.Exec(string(updateLPinLeagueEntriesSQL), newLP, summonerID)
	if err != nil {
		return fmt.Errorf("error updating league entry: %w", err)
	}

	return tx.Commit()
}

func (s *Storage) CalculateLPChange(oldTier, newTier string, oldLP, newLP int) int {
	if oldTier == "DIAMOND" && newTier == "MASTER" {
		return (100 - oldLP) + newLP
	}

	return newLP - oldLP
}

// ListSummoners retrieves and returns a list of summoners with their ranks for a given guild ID.
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

func (s *Storage) Close() error {
	return s.db.Close()
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

// GetAllSummonersForGuild retrieve and returns summoners in a guild list in the database.
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

// UpdateLPHistory updates the LP for a summoner in the league_entries table
func (s *Storage) UpdateLPHistory(riotSummonerID, matchID string, lpChange, newLP int) error {
	_, err := s.db.Exec(string(insertLPHistorySQL), riotSummonerID, matchID, lpChange, newLP)
	if err != nil {
		return fmt.Errorf("error updating LP history: %w", err)
	}
	return nil
}

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

func (s *Storage) GetSummonerIDFromRiotID(riotSummonerID string) (int, error) {
	var summonerID int
	err := s.db.QueryRow("SELECT id FROM summoners WHERE riot_summoner_id = $1", riotSummonerID).Scan(&summonerID)
	if err != nil {
		return 0, fmt.Errorf("error fetching summoner ID: %w", err)
	}
	return summonerID, nil
}

func (s *Storage) GetLastLPChange(summonerID int) (int, error) {
	var lpChange int
	err := s.db.QueryRow("SELECT lp_change FROM lp_history WHERE summoner_id = $1 ORDER BY timestamp DESC LIMIT 1", summonerID).Scan(&lpChange)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}
	return lpChange, nil
}

// Retrieves PreviousRank from LeagueEntries using summonerID, to be used before updating entries
func (s *Storage) GetPreviousRank(summonerID int) (*PreviousRank, error) {
	var prevRank PreviousRank
	err := s.db.QueryRow(string(selectPreviousRankInLeagueEntriesSQL), summonerID).Scan(&prevRank.PrevTier, &prevRank.PrevRank, &prevRank.PrevLP)
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

func (s *Storage) GetLPChangesForLast24Hours(riotSummonerID string) ([]LPChange, error) {
	rows, err := s.db.Query(string(selectLPChangesOfSummonersLast24HoursSQL), riotSummonerID)
	if err != nil {
		return nil, fmt.Errorf("error querying LP changes: %w", err)
	}
	defer rows.Close()

	var lpChanges []LPChange
	for rows.Next() {
		var change LPChange
		if err := rows.Scan(&change.Timestamp, &change.NewLP); err != nil {
			return nil, fmt.Errorf("error scanning LP change data: %w", err)
		}
		lpChanges = append(lpChanges, change)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating LP change data rows: %w", err)
	}

	return lpChanges, nil
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
