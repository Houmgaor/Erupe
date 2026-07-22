-- Reject tournament rows with non-positive timestamps. Such rows used to be
-- returned by GetActive, producing an EnumerateRanking response with state=3
-- and zero start/end times that crashed every ZZ quest counter
-- (see Mezeporta/Erupe#193).

-- Clean up any pre-existing bad rows so the CHECK constraints can be applied.
-- Cascading FKs on tournament_cups, tournament_entries, tournament_results
-- (ON DELETE CASCADE in 0021_tournament.sql) take care of dependent rows.
DELETE FROM tournaments
WHERE start_time  <= 0
   OR entry_end   <= 0
   OR ranking_end <= 0
   OR reward_end  <= 0;

-- Guarded so this migration can be re-applied against a database that
-- already has these constraints (e.g. the pre-schema_version baseline
-- detection path re-runs every migration after 0001).
DO $$ BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints
        WHERE table_name = 'tournaments' AND constraint_name = 'tournaments_start_time_positive'
    ) THEN
        ALTER TABLE tournaments ADD CONSTRAINT tournaments_start_time_positive CHECK (start_time > 0);
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints
        WHERE table_name = 'tournaments' AND constraint_name = 'tournaments_entry_end_positive'
    ) THEN
        ALTER TABLE tournaments ADD CONSTRAINT tournaments_entry_end_positive CHECK (entry_end > 0);
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints
        WHERE table_name = 'tournaments' AND constraint_name = 'tournaments_ranking_end_positive'
    ) THEN
        ALTER TABLE tournaments ADD CONSTRAINT tournaments_ranking_end_positive CHECK (ranking_end > 0);
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints
        WHERE table_name = 'tournaments' AND constraint_name = 'tournaments_reward_end_positive'
    ) THEN
        ALTER TABLE tournaments ADD CONSTRAINT tournaments_reward_end_positive CHECK (reward_end > 0);
    END IF;
END $$;
