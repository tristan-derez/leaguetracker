package storage

type SQLQuery string

const (
	insertSummonerSQL SQLQuery = `
    INSERT INTO summoners (
        name, riot_account_id, riot_summoner_id, riot_summoner_puuid, summoner_level, profile_icon_id, 
        revision_date, created_at, updated_at
    ) 
    VALUES ($1, $2, $3, $4, $5, $6, $7::BIGINT, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP) 
    ON CONFLICT (name) DO UPDATE SET 
        summoner_level = EXCLUDED.summoner_level,
        profile_icon_id = EXCLUDED.profile_icon_id,
        revision_date = EXCLUDED.revision_date,
        updated_at = CURRENT_TIMESTAMP 
    RETURNING id
    `

	insertLeagueEntrySQL SQLQuery = `
    INSERT INTO league_entries (
        summoner_id, queue_type, tier, rank, league_points,
        wins, losses, hot_streak, veteran, fresh_blood, inactive,
        created_at, updated_at
    )
    VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
    ON CONFLICT (summoner_id, queue_type) DO UPDATE SET
        tier = EXCLUDED.tier,
        rank = EXCLUDED.rank,
        league_points = EXCLUDED.league_points,
        wins = EXCLUDED.wins,
        losses = EXCLUDED.losses,
        hot_streak = EXCLUDED.hot_streak,
        veteran = EXCLUDED.veteran,
        fresh_blood = EXCLUDED.fresh_blood,
        inactive = EXCLUDED.inactive,
        updated_at = CURRENT_TIMESTAMP
    `

	insertGuildSummonerAssociationSQL SQLQuery = `
    INSERT INTO guild_summoner_associations (guild_id, summoner_id, created_at, updated_at)
    VALUES ($1, $2, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
    ON CONFLICT (guild_id, summoner_id) DO NOTHING
    `

	deleteSummonerSQL SQLQuery = `
        DELETE FROM guild_summoner_associations
        WHERE guild_id = $1 AND summoner_id = (SELECT id FROM summoners WHERE name = $2)
    `

	insertMatchDataSQL SQLQuery = `
    INSERT INTO matches (
            summoner_id, match_id, champion_name, game_creation, game_duration,
            game_end_timestamp, game_id, queue_id, game_mode, game_type, kills, deaths, assists,
            result, pentakills, team_position, team_damage_percentage, kill_participation, total_damage_dealt_to_champions,
            total_minions_killed, neutral_minions_killed, wards_killed,
            wards_placed, win, total_minions_and_neutral_minions_killed
    ) VALUES (
            $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15,
            $16, $17, $18, $19, $20, $21, $22, $23, $24, $25
    ) ON CONFLICT (summoner_id, match_id) DO NOTHING
    `

	selectSummonerWithRankSQL SQLQuery = `
    SELECT 
        s.riot_summoner_id, 
        s.riot_account_id, 
        s.riot_summoner_puuid, 
        s.profile_icon_id, 
        s.revision_date, 
        s.summoner_level, 
        s.name,
        CASE
            WHEN le.tier = 'UNRANKED' OR le.tier IS NULL THEN 'UNRANKED'
            ELSE le.tier || ' ' || le.rank
        END as rank,
        COALESCE(le.league_points, 0) as league_points
    FROM 
        summoners s
    LEFT JOIN 
        league_entries le ON s.id = le.summoner_id AND le.queue_type = 'RANKED_SOLO_5x5'
    JOIN 
        guild_summoner_associations gsa ON s.id = gsa.summoner_id
    WHERE 
        gsa.guild_id = $1
    `

	selectChannelIdFromGuildIdSQL SQLQuery = `
    SELECT channel_id
	FROM guilds
	WHERE guild_id = $1
    `

	removeChannelFromGuildSQL SQLQuery = `
    UPDATE guild_summoner_associations
    SET channel_id = NULL, updated_at = CURRENT_TIMESTAMP
    WHERE guild_id = $1 AND channel_id = $2
    `

	selectAllSummonersForAGuildSQL SQLQuery = `
    SELECT s.riot_summoner_puuid, s.name, s.riot_summoner_id
    FROM summoners s
    JOIN guild_summoner_associations gsa ON s.id = gsa.summoner_id
    WHERE gsa.guild_id = $1
    `

	updateGuildWithChannelIDSQL SQLQuery = `
    UPDATE guilds
    SET channel_id = $2, updated_at = CURRENT_TIMESTAMP
    WHERE guild_id = $1
    `

	// get last match id from db
	selectLastMatchIDSQL SQLQuery = `
    SELECT match_id
    FROM matches
    WHERE summoner_id = (SELECT id FROM summoners WHERE riot_summoner_puuid = $1)
    ORDER BY game_end_timestamp DESC
    LIMIT 1
	`

	// get previous lp from league entries in database
	selectLeaguePointsFromLeagueEntriesSQL SQLQuery = `
    SELECT league_points FROM league_entries WHERE summoner_id = $1 AND queue_type = 'RANKED_SOLO_5x5'
    `

	// insert LP change into lp_history
	insertLPChangeIntoLPHistorySQL SQLQuery = `
    INSERT INTO lp_history (summoner_id, match_id, lp_change, new_lp)
    VALUES ($1, $2, $3, $4)
    `
	// update LP in league entries
	updateLPinLeagueEntriesSQL SQLQuery = `
    UPDATE league_entries
    SET league_points = $1, updated_at = CURRENT_TIMESTAMP
    WHERE summoner_id = $2 AND queue_type = 'RANKED_SOLO_5x5'
    `
	selectLastKnownLPSQL SQLQuery = `
    SELECT new_lp
    FROM lp_history
    WHERE summoner_id = (SELECT id FROM summoners WHERE riot_summoner_id = $1)
    ORDER BY timestamp DESC
    LIMIT 1
    `

	insertLPHistorySQL SQLQuery = `
    INSERT INTO lp_history (summoner_id, match_id, lp_change, new_lp)
    VALUES ((SELECT id FROM summoners WHERE riot_summoner_id = $1), $2, $3, $4)
    `

	selectPreviousRankInLeagueEntriesSQL SQLQuery = `
    SELECT tier, rank, league_points
    FROM league_entries
    WHERE summoner_id = $1
    `

	selectLPChangesOfSummonersLast24HoursSQL SQLQuery = `
    SELECT lh.timestamp, lh.new_lp
    FROM lp_history lh
    JOIN summoners s ON lh.summoner_id = s.id
    WHERE s.riot_summoner_id = $1 AND lh.timestamp > NOW() - INTERVAL '24 hours'
    ORDER BY lh.timestamp ASC
	`
)
