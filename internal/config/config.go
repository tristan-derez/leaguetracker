package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
    DiscordToken  string
    RiotAPIKey    string
    RiotAPIRegion string
    DBHost        string
    DBPort        string
    DBUsername    string
    DBPassword    string
    DBDatabase    string
    DBSchema      string
}

func Load() (*Config, error) {
    if err := godotenv.Load(); err != nil {
        return nil, fmt.Errorf("error loading .env file: %w", err)
    }

    config := &Config{
        DiscordToken: os.Getenv("DISCORD_TOKEN"),
        RiotAPIKey: os.Getenv("RIOT_API"),
        RiotAPIRegion: os.Getenv("RIOT_REGION"),
        DBHost:       os.Getenv("DB_HOST"),
        DBPort:       os.Getenv("DB_PORT"),
        DBUsername:       os.Getenv("DB_USERNAME"),
        DBPassword:   os.Getenv("DB_PASSWORD"),
        DBDatabase:       os.Getenv("DB_DATABASE"),
        DBSchema:     os.Getenv("DB_SCHEMA"),
    }

    if err := config.validate(); err != nil {
        return nil, err
    }

    return config, nil
}

func (c *Config) validate() error {
    requiredVars := map[string]*string{
        "DISCORD_TOKEN": &c.DiscordToken,
        "RIOT_API":      &c.RiotAPIKey,
        "RIOT_REGION":   &c.RiotAPIRegion,
        "DB_HOST":       &c.DBHost,
        "DB_PORT":       &c.DBPort,
        "DB_USERNAME":   &c.DBUsername,
        "DB_PASSWORD":   &c.DBPassword,
        "DB_DATABASE":   &c.DBDatabase,
        "DB_SCHEMA":     &c.DBSchema,
    }

    var missingVars []string

    for envVar, value := range requiredVars {
        if *value == "" {
            missingVars = append(missingVars, envVar)
        }
    }

    if len(missingVars) > 0 {
        return fmt.Errorf("missing required environment variables: %v", missingVars)
    }

    return nil
}