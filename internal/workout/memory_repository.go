package workout

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/jwallace145/progressive-overload-fitness-tracker/internal/id"
)

// Compile-time check that *MemoryRepository satisfies Repository.
var _ Repository = (*MemoryRepository)(nil)

// MemoryRepository is an in-memory implementation of Repository.
// It's safe for concurrent use. Data is lost when the process exits —
// intended for development, testing, and early prototyping.
type MemoryRepository struct {
	mu       sync.RWMutex
	workouts map[string]*Workout
	// history keyed by workout_id so cascade on Update/Delete is O(1).
	// The in-memory repo is dev/test only, so the per-list filter for
	// reads is fine — beats maintaining a second index.
	history map[string][]OneRepMaxEntry
	now     func() time.Time // injectable for tests
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		workouts: make(map[string]*Workout),
		history:  make(map[string][]OneRepMaxEntry),
		now:      time.Now,
	}
}

func (r *MemoryRepository) Create(ctx context.Context, w *Workout) error {
	if err := w.Validate(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	now := r.now().UTC()
	w.ID = id.New()
	w.CreatedAt = now
	w.UpdatedAt = now

	// Store a copy so external mutation doesn't affect our state.
	stored := *w
	r.workouts[w.ID] = &stored
	r.writeHistoryLocked(*w, now)
	return nil
}

func (r *MemoryRepository) GetByID(ctx context.Context, id string) (*Workout, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	w, ok := r.workouts[id]
	if !ok || w.DeletedAt != nil {
		return nil, ErrNotFound
	}
	out := *w
	return &out, nil
}

func (r *MemoryRepository) ListByUser(ctx context.Context, userID string, opts ListOptions) ([]Workout, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []Workout
	for _, w := range r.workouts {
		if w.UserID != userID || w.DeletedAt != nil {
			continue
		}
		if opts.Since != nil && w.PerformedAt.Before(*opts.Since) {
			continue
		}
		if opts.Until != nil && w.PerformedAt.After(*opts.Until) {
			continue
		}
		results = append(results, *w)
	}

	// Most recent first.
	sort.Slice(results, func(i, j int) bool {
		return results[i].PerformedAt.After(results[j].PerformedAt)
	})

	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}
	if opts.Offset >= len(results) {
		return []Workout{}, nil
	}
	end := opts.Offset + limit
	if end > len(results) {
		end = len(results)
	}
	return results[opts.Offset:end], nil
}

func (r *MemoryRepository) Update(ctx context.Context, w *Workout) error {
	if err := w.Validate(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	existing, ok := r.workouts[w.ID]
	if !ok || existing.DeletedAt != nil {
		return ErrNotFound
	}

	w.CreatedAt = existing.CreatedAt
	w.UpdatedAt = r.now().UTC()
	stored := *w
	r.workouts[w.ID] = &stored
	r.writeHistoryLocked(*w, w.UpdatedAt)
	return nil
}

func (r *MemoryRepository) Delete(ctx context.Context, workoutID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	w, ok := r.workouts[workoutID]
	if !ok || w.DeletedAt != nil {
		return ErrNotFound
	}
	now := r.now().UTC()
	w.DeletedAt = &now
	w.UpdatedAt = now
	delete(r.history, workoutID)
	return nil
}

func (r *MemoryRepository) ListOneRepMaxHistory(
	ctx context.Context,
	userID, exerciseID string,
	since, until *time.Time,
) ([]OneRepMaxEntry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var out []OneRepMaxEntry
	for _, entries := range r.history {
		for _, e := range entries {
			if e.UserID != userID || e.ExerciseID != exerciseID {
				continue
			}
			if since != nil && e.PerformedAt.Before(*since) {
				continue
			}
			if until != nil && e.PerformedAt.After(*until) {
				continue
			}
			out = append(out, e)
		}
	}
	// Most recent first, matching the SQLite implementation.
	sort.Slice(out, func(i, j int) bool {
		return out[i].PerformedAt.After(out[j].PerformedAt)
	})
	return out, nil
}

// writeHistoryLocked replaces the history rows for w with a freshly
// aggregated set. Caller must hold r.mu in write mode. Used by Create
// and Update — same aggregation function the SQLite repository uses.
func (r *MemoryRepository) writeHistoryLocked(w Workout, now time.Time) {
	delete(r.history, w.ID)
	entries := AggregateOneRepMax(w)
	if len(entries) == 0 {
		return
	}
	for i := range entries {
		entries[i].ID = id.New()
		entries[i].CreatedAt = now
		entries[i].UpdatedAt = now
	}
	r.history[w.ID] = entries
}
