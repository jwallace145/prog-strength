-- migrations/003_exercise_one_rep_max_history.sql
-- One row per (workout, exercise) pair recording the per-set Epley
-- estimated 1RM rolled up to min/avg/max for that exercise within that
-- workout. The table is fully derived from `workouts` — every workout
-- write produces a corresponding write here. See
-- prog-strength-docs/sows/estimated-one-rep-max.md
-- for the full rationale.
--
-- The table is created empty here; an in-process backfill runs on the
-- first startup after this migration ships and populates rows from the
-- existing workouts. Backfill lives in Go (not SQL) so that the live
-- write path and the backfill share a single aggregation function.
--
-- ON DELETE CASCADE on workout_id is defensive — workouts are soft-
-- deleted in practice, so the cascade rarely fires; the repository
-- explicitly hard-deletes history rows when a workout is soft-deleted.

CREATE TABLE IF NOT EXISTS exercise_one_rep_max_history (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    workout_id TEXT NOT NULL REFERENCES workouts(id) ON DELETE CASCADE,
    exercise_id TEXT NOT NULL REFERENCES exercises(id),
    performed_at DATETIME NOT NULL,
    min_estimated_1rm REAL NOT NULL,
    avg_estimated_1rm REAL NOT NULL,
    max_estimated_1rm REAL NOT NULL,
    set_count INTEGER NOT NULL CHECK(set_count > 0),
    unit TEXT NOT NULL CHECK(unit IN ('lb', 'kg')),
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);

-- Dominant access pattern: "this user's recent entries for this
-- exercise" (the baseline computation). DESC matches the natural sort.
CREATE INDEX idx_orm_history_user_exercise_time
    ON exercise_one_rep_max_history(user_id, exercise_id, performed_at DESC);

-- Workout-keyed cascade lookups during workout update/delete.
CREATE INDEX idx_orm_history_workout
    ON exercise_one_rep_max_history(workout_id);
