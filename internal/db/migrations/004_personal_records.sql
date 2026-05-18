-- migrations/004_personal_records.sql
-- Personal records and their break events. See
-- prog-strength-docs/sows/personal-records.md for the full design.
--
-- Two related tables:
--   personal_records        — one row per (user, exercise) with the
--                             user's current heaviest set on that
--                             exercise, plus the workout that holds it.
--   personal_record_events  — append-only log, one row per time a PR
--                             was broken. Used to badge PR-breaking
--                             workouts and feed the chat agent.
--
-- Both tables are fully derived from `workouts`. An in-process backfill
-- on the first startup after this migration populates them; subsequent
-- writes are maintained by the workout repository inside its existing
-- Create/Update/Delete transactions.

CREATE TABLE IF NOT EXISTS personal_records (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    exercise_id TEXT NOT NULL REFERENCES exercises(id),
    workout_id TEXT NOT NULL REFERENCES workouts(id) ON DELETE CASCADE,
    weight REAL NOT NULL CHECK(weight > 0),
    reps INTEGER NOT NULL CHECK(reps >= 1),
    unit TEXT NOT NULL CHECK(unit IN ('lb', 'kg')),
    achieved_at DATETIME NOT NULL,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    UNIQUE(user_id, exercise_id)
);

CREATE INDEX idx_pr_user_achieved
    ON personal_records(user_id, achieved_at DESC);

-- Append-only event log. Rows are derived from `workouts`; the repo's
-- Update/Delete paths rebuild them for affected (user, exercise) pairs
-- rather than mutating in place.
CREATE TABLE IF NOT EXISTS personal_record_events (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    exercise_id TEXT NOT NULL REFERENCES exercises(id),
    workout_id TEXT NOT NULL REFERENCES workouts(id) ON DELETE CASCADE,
    weight REAL NOT NULL,
    reps INTEGER NOT NULL,
    unit TEXT NOT NULL CHECK(unit IN ('lb', 'kg')),
    previous_weight REAL,
    previous_reps INTEGER,
    previous_unit TEXT,
    achieved_at DATETIME NOT NULL,
    created_at DATETIME NOT NULL
);

-- Workout-keyed lookups for the "did this workout break a PR?" join
-- driving the 🏆 badge on workout list/detail responses.
CREATE INDEX idx_pre_workout
    ON personal_record_events(workout_id);

-- Per-user recent-break feeds (agent congratulations, future feed UI).
CREATE INDEX idx_pre_user_achieved
    ON personal_record_events(user_id, achieved_at DESC);
