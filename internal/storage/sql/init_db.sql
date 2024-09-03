-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS guilds (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    guild_id TEXT UNIQUE NOT NULL,
    guild_name TEXT,
    channel_id TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS summoners (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name TEXT UNIQUE NOT NULL,
    riot_account_id TEXT,
    riot_summoner_id TEXT UNIQUE NOT NULL,
    riot_summoner_puuid TEXT,
    summoner_level BIGINT,
    profile_icon_id INTEGER,
    revision_date BIGINT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS league_entries (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    summoner_id UUID REFERENCES summoners(id),
    queue_type TEXT NOT NULL,
    tier TEXT,
    rank TEXT,
    league_points INTEGER,
    wins INTEGER,
    losses INTEGER,
    hot_streak BOOLEAN,
    veteran BOOLEAN,
    fresh_blood BOOLEAN,
    inactive BOOLEAN,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(summoner_id, queue_type)
);

CREATE TABLE IF NOT EXISTS matches (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    summoner_id UUID REFERENCES summoners(id),
    match_id TEXT NOT NULL,
    champion_name TEXT NOT NULL,
    game_creation BIGINT NOT NULL,
    game_duration INTEGER NOT NULL,
    game_end_timestamp BIGINT NOT NULL,
    game_id BIGINT NOT NULL,
    queue_id INT NOT NULL,
    game_mode TEXT NOT NULL,
    game_type TEXT NOT NULL,
    kills INTEGER NOT NULL,
    deaths INTEGER NOT NULL,
    assists INTEGER NOT NULL,
    result TEXT NOT NULL,
    pentakills INTEGER NOT NULL,
    team_position TEXT NOT NULL,
    team_damage_percentage DOUBLE PRECISION NOT NULL,
    kill_participation DOUBLE PRECISION NOT NULL,
    total_damage_dealt_to_champions INTEGER NOT NULL,
    total_minions_killed INTEGER NOT NULL,
    neutral_minions_killed INTEGER NOT NULL,
    wards_killed INTEGER NOT NULL,
    wards_placed INTEGER NOT NULL,
    win BOOLEAN NOT NULL,
    total_minions_and_neutral_minions_killed INTEGER NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(summoner_id, match_id)
);

CREATE TABLE IF NOT EXISTS lp_history (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    summoner_id UUID REFERENCES summoners(id),
    match_id TEXT NOT NULL,
    lp_change INTEGER NOT NULL,
    new_lp INTEGER NOT NULL,
    tier TEXT NOT NULL,
    rank TEXT NOT NULL,
    timestamp TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS guild_summoner_associations (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    guild_id UUID REFERENCES guilds(id),
    summoner_id UUID REFERENCES summoners(id),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(guild_id, summoner_id)
);

CREATE TABLE IF NOT EXISTS placement_games (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    summoner_id UUID REFERENCES summoners(id),
    season_year INTEGER NOT NULL,
    total_games INTEGER NOT NULL DEFAULT 0,
    wins INTEGER NOT NULL DEFAULT 0,
    losses INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(summoner_id, season_year),
    CONSTRAINT max_games CHECK (total_games <= 5)
);