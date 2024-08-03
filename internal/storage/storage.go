package storage

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"
	"github.com/tristan-derez/league-tracker/internal/config"
)

type Storage struct {
    db *sql.DB
}

func New(config *config.Config) (*Storage, error) {
    connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
        config.DBHost, config.DBPort, config.DBUsername, config.DBPassword, config.DBDatabase)


    var db *sql.DB
    var err error

    for i := 0; i < 5; i++ {
        db, err = sql.Open("postgres", connStr)
        if err == nil {
            err = db.Ping()
            if err == nil {
                break
            }
        }
        fmt.Printf("Failed to connect to database, retrying in 5 seconds... (attempt %d/5)\n", i+1)
        time.Sleep(5 * time.Second)
    }

    if err != nil {
        return nil, fmt.Errorf("error connecting to database after 5 attempts: %w", err)
    }

    storage := &Storage{db: db}
    if err := storage.initDB(); err != nil {
        return nil, fmt.Errorf("error initializing database: %w", err)
    }

    return storage, nil
}

func (s *Storage) initDB() error {
    _, err := s.db.Exec(`
        CREATE TABLE IF NOT EXISTS summoners (
            name TEXT PRIMARY KEY
        )
    `)
    return err
}

func (s *Storage) AddSummoner(name string) error {
    _, err := s.db.Exec("INSERT INTO summoners (name) VALUES ($1) ON CONFLICT (name) DO NOTHING", name)
    return err
}

func (s *Storage) RemoveSummoner(name string) error {
    // todo: check if summoner is in db
    _, err := s.db.Exec("DELETE FROM summoners WHERE name = $1", name)
    return err
}

func (s *Storage) ListSummoners() ([]string, error) {
    rows, err := s.db.Query("SELECT name FROM summoners")
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var summoners []string
    for rows.Next() {
        var name string
        if err := rows.Scan(&name); err != nil {
            return nil, err
        }
        summoners = append(summoners, name)
    }

    return summoners, rows.Err()
}

func (s *Storage) Close() error {
    return s.db.Close()
}