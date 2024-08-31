package storage

type SQLQuery string

const (
	// insert or update a new guild into guilds table
	insertNewGuildSQL SQLQuery = `
        INSERT INTO guilds (guild_id, guild_name, created_at, updated_at)
        VALUES ($1, $2, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
        ON CONFLICT (guild_id) 
        DO UPDATE SET guild_name = $2, updated_at = CURRENT_TIMESTAMP
    `

	// insert or update a summoner into summoners table
	insertSummonerSQL SQLQuery = `
    INSERT INTO summoners (
        name, riot_account_id, riot_summoner_id, riot_summoner_puuid, summoner_level, profile_icon_id, 
        revision_date, created_at, updated_at
    ) 
    VALUES ($1, $2, $3, $4, $5, $6, $7::BIGINT, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP) 
    ON CONFLICT (riot_summoner_id) DO UPDATE SET 
        name = EXCLUDED.name,
        summoner_level = EXCLUDED.summoner_level,
        profile_icon_id = EXCLUDED.profile_icon_id,
        revision_date = EXCLUDED.revision_date,
        updated_at = CURRENT_TIMESTAMP 
    RETURNING id
    `

	// insert a league entry for a summoner
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

	// associate a summoner to a guild
	insertGuildSummonerAssociationSQL SQLQuery = `
    INSERT INTO guild_summoner_associations (guild_id, summoner_id, created_at, updated_at)
    VALUES ($1, $2, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
    ON CONFLICT (guild_id, summoner_id) DO NOTHING
    `

	// delete an association of a summoner from a guild
	deleteSummonerSQL SQLQuery = `
        DELETE FROM guild_summoner_associations
        WHERE guild_id = $1 AND summoner_id = (SELECT id FROM summoners WHERE name = $2)
    `

	// insert match data into matches table
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

	// gives the rank from a summoner by joining summoners and league entries
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

	// get the channel id from guilds table
	selectChannelIdFromGuildIdSQL SQLQuery = `
    SELECT channel_id
	FROM guilds
	WHERE guild_id = $1
    `

	// remove the attributed channel for updates from guild
	removeChannelFromGuildSQL SQLQuery = `
    UPDATE guilds
    SET channel_id = NULL, updated_at = CURRENT_TIMESTAMP
    WHERE guild_id = $1 AND channel_id = $2
    `

	// select every summoners associated to guild
	selectAllSummonersForAGuildSQL SQLQuery = `
    SELECT s.riot_summoner_puuid, s.name, s.riot_summoner_id
    FROM summoners s
    JOIN guild_summoner_associations gsa ON s.id = gsa.summoner_id
    WHERE gsa.guild_id = $1
    `

	// update the channel id of a guild
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

	// update LP, rank and tier in league entries
	updateLeagueEntriesSQL SQLQuery = `
    UPDATE league_entries
    SET 
        league_points = $1, 
        tier = $2, 
        rank = $3, 
        updated_at = CURRENT_TIMESTAMP
    WHERE 
        summoner_id = $4 
        AND queue_type = 'RANKED_SOLO_5x5'
    `

	// create a new row for lp, tier and rank in lp_history with a summoner id
	// we store the tier and rank for a better tracking of progress
	insertLDataInLPHistorySQL SQLQuery = `
    INSERT INTO lp_history (summoner_id, match_id, lp_change, new_lp, tier, rank)
    VALUES ($1, $2, $3, $4, $5, $6)
    `

	// get rank from league entries
	selectRankInLeagueEntriesSQL SQLQuery = `
    SELECT tier, rank, league_points
    FROM league_entries
    WHERE summoner_id = $1
    `

	// remove all summoners associated to a guild
	removeAllSummonersFromGuildSQL SQLQuery = `
    DELETE FROM guild_summoner_associations
    WHERE guild_id = $1
	`

	// Get daily progress for summoners in a guild for the previous day
	getDailySummonerProgressSQL SQLQuery = `
    WITH target_date AS (
        SELECT (CURRENT_DATE AT TIME ZONE 'UTC' - INTERVAL '1 day')::DATE AS date
    ),
    daily_progress AS (
        SELECT 
            s.id AS summoner_id,
            s.name,
            lh.tier,
            lh.rank,
            lh.new_lp,
            lh.lp_change,
            lh.timestamp,
            ROW_NUMBER() OVER (PARTITION BY s.id ORDER BY lh.timestamp DESC) AS rn_latest,
            ROW_NUMBER() OVER (PARTITION BY s.id ORDER BY lh.timestamp ASC) AS rn_earliest
        FROM 
            summoners s
        JOIN 
            guild_summoner_associations gsa ON s.id = gsa.summoner_id
        JOIN 
            lp_history lh ON s.id = lh.summoner_id
        CROSS JOIN
            target_date td
        WHERE 
            gsa.guild_id = $1
            AND lh.timestamp >= td.date
            AND lh.timestamp < td.date + INTERVAL '1 day'
    ),
    summoner_stats AS (
        SELECT
            summoner_id,
            SUM(CASE WHEN lp_change > 0 THEN 1 ELSE 0 END) AS wins,
            SUM(CASE WHEN lp_change < 0 THEN 1 ELSE 0 END) AS losses,
            SUM(lp_change) AS total_lp_change
        FROM
            daily_progress
        GROUP BY
            summoner_id
    )
    SELECT 
        dp_latest.name,
        dp_latest.tier AS current_tier,
        dp_latest.rank AS current_rank,
        dp_latest.new_lp AS current_lp,
        dp_earliest.tier AS previous_tier,
        dp_earliest.rank AS previous_rank,
        dp_earliest.new_lp AS previous_lp,
        COALESCE(ss.wins, 0) AS wins,
        COALESCE(ss.losses, 0) AS losses,
        COALESCE(ss.total_lp_change, 0) AS lp_change
    FROM 
        daily_progress dp_latest
    JOIN 
        daily_progress dp_earliest ON dp_latest.summoner_id = dp_earliest.summoner_id
    LEFT JOIN
        summoner_stats ss ON dp_latest.summoner_id = ss.summoner_id
    WHERE 
        dp_latest.rn_latest = 1
        AND dp_earliest.rn_earliest = 1
        AND (dp_latest.new_lp != dp_earliest.new_lp OR dp_latest.tier != dp_earliest.tier OR dp_latest.rank != dp_earliest.rank)
    ORDER BY 
        ss.total_lp_change DESC NULLS LAST
    `

	selectSummonerInGuildSQL SQLQuery = `
    SELECT s.riot_summoner_id, s.riot_account_id, s.riot_summoner_puuid, 
            s.profile_icon_id, s.revision_date, s.summoner_level, s.name,
            array_agg(gsa.guild_id) as guild_ids
    FROM summoners s
    JOIN guild_summoner_associations gsa ON s.id = gsa.summoner_id
    GROUP BY s.id
    `
)
