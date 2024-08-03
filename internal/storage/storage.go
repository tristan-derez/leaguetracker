package storage

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/lib/pq"
	"github.com/tristan-derez/league-tracker/internal/config"
)

type Storage struct {
    db *sql.DB
}

type Summoner struct {
    ID           int
    Name         string
    Rank         *string
    LeaguePoints *int
    CreatedAt    time.Time
    UpdatedAt    time.Time
}


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
    statements := []string{
        `CREATE TABLE IF NOT EXISTS guilds (
            guild_id TEXT PRIMARY KEY,
            guild_name TEXT,
            created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
            updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
        )`,
        `CREATE TABLE IF NOT EXISTS summoners (
            id SERIAL PRIMARY KEY,
            name TEXT UNIQUE NOT NULL,
            rank TEXT,
            league_points INTEGER,
            created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
            updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
        )`,
        `CREATE TABLE IF NOT EXISTS guild_summoner_associations (
            guild_id TEXT REFERENCES guilds(guild_id),
            summoner_id INTEGER REFERENCES summoners(id),
            created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
            updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
            PRIMARY KEY (guild_id, summoner_id)
        )`,
    }

    for i, stmt := range statements {
        log.Printf("Executing statement %d:\n%s", i+1, stmt)
        _, err := s.db.Exec(stmt)
        if err != nil {
            log.Printf("Error executing statement %d: %v", i+1, err)
            return fmt.Errorf("error executing statement %d: %w", i+1, err)
        }
        log.Printf("Statement %d executed successfully", i+1)
    }

    log.Println("All statements executed successfully")
    return nil
}

func (s *Storage) AddGuild(guildID, guildName string) error {
    _, err := s.db.Exec(`
        INSERT INTO guilds (guild_id, guild_name, created_at, updated_at)
        VALUES ($1, $2, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
        ON CONFLICT (guild_id) 
        DO UPDATE SET guild_name = $2, updated_at = CURRENT_TIMESTAMP
    `, guildID, guildName)
    return err
}

func (s *Storage) AddSummoner(guildID, summonerName string) error {
    tx, err := s.db.Begin()
    if err != nil {
        return err
    }
    defer tx.Rollback()

    var summonerID int
    err = tx.QueryRow(`
        INSERT INTO summoners (name, created_at, updated_at) 
        VALUES ($1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP) 
        ON CONFLICT (name) DO UPDATE SET updated_at = CURRENT_TIMESTAMP 
        RETURNING id
    `, summonerName).Scan(&summonerID)
    if err != nil {
        return err
    }

    _, err = tx.Exec(`
        INSERT INTO guild_summoner_associations (guild_id, summoner_id, created_at, updated_at)
        VALUES ($1, $2, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
        ON CONFLICT (guild_id, summoner_id) 
        DO UPDATE SET updated_at = CURRENT_TIMESTAMP
    `, guildID, summonerID)
    if err != nil {
        return err
    }

    return tx.Commit()
}

func (s *Storage) RemoveSummoner(guildID, summonerName string) error {
    _, err := s.db.Exec(`
        DELETE FROM guild_summoner_associations
        WHERE guild_id = $1 AND summoner_id = (SELECT id FROM summoners WHERE name = $2)
    `, guildID, summonerName)
    return err
}

func (s *Storage) ListSummoners(guildID string) ([]Summoner, error) {
    rows, err := s.db.Query(`
        SELECT s.id, s.name, COALESCE(s.rank, ''), COALESCE(s.league_points, 0), s.updated_at
        FROM summoners s
        JOIN guild_summoner_associations gsa ON s.id = gsa.summoner_id
        WHERE gsa.guild_id = $1
    `, guildID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var summoners []Summoner
    for rows.Next() {
        var s Summoner
        var rank string
        var leaguePoints int
        if err := rows.Scan(&s.ID, &s.Name, &rank, &leaguePoints, &s.UpdatedAt); err != nil {
            return nil, err
        }
        s.Rank = &rank
        s.LeaguePoints = &leaguePoints
        summoners = append(summoners, s)
    }

    return summoners, rows.Err()
}

func (s *Storage) Close() error {
    return s.db.Close()
}