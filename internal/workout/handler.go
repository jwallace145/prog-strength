package workout

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jwallace145/progressive-overload-fitness-tracker/internal/auth"
	"github.com/jwallace145/progressive-overload-fitness-tracker/internal/exercise"
	"github.com/jwallace145/progressive-overload-fitness-tracker/internal/httpresp"
)

// Handler exposes HTTP endpoints for workout logging.
//
// The progression endpoint needs to translate a muscle-group filter
// into the set of exercises that target it, so the handler depends on
// the exercise repository as well as the workout one. This is the only
// cross-package coupling in the workout HTTP layer — the underlying
// workout domain types and repository still have no compile-time
// dependency on `exercise` (per CLAUDE.md: "Workout package doesn't
// import exercise package's data"); the join lives at the HTTP edge.
type Handler struct {
	repo         Repository
	exerciseRepo exercise.Repository
}

// NewHandler builds a Handler backed by the given repositories. The
// exercise repo is used by the progression endpoint to resolve a
// muscle-group filter into a list of catalog exercises.
func NewHandler(repo Repository, exerciseRepo exercise.Repository) *Handler {
	return &Handler{repo: repo, exerciseRepo: exerciseRepo}
}

// Mount registers workout routes on the given router. Callers are expected
// to have already wrapped the router in auth.RequireUser middleware — these
// handlers read the user ID from request context and assume it is present.
func (h *Handler) Mount(r chi.Router) {
	r.Route("/workouts", func(r chi.Router) {
		r.Get("/", h.list)
		// Registered before any /{id} routes so chi matches literal
		// path segments instead of trying to interpret them as workout IDs.
		r.Get("/progression", h.progression)
		r.Get("/{id}", h.get)
		r.Post("/", h.create)
		r.Put("/{id}", h.update)
		r.Delete("/{id}", h.delete)
	})
	r.Get("/personal-records", h.personalRecords)
}

// list handles GET /workouts. Returns the authed user's workouts, most
// recent first. The repository caps results at 50; pagination and
// filtering are intentionally not yet exposed (will be added when the
// data volume actually warrants it).
//
// Each workout in the response carries any PR events it produced via
// `personal_records_set`, so the frontend can badge sessions inline
// without a second round trip.
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFrom(r.Context())
	if !ok {
		httpresp.ServerError(w, r.Context(), "missing user in context", errors.New("auth middleware not applied"))
		return
	}

	workouts, err := h.repo.ListByUser(r.Context(), userID, ListOptions{})
	if err != nil {
		httpresp.ServerError(w, r.Context(), "list workouts", err)
		return
	}

	wrapped, err := h.attachPersonalRecordEvents(r.Context(), workouts)
	if err != nil {
		httpresp.ServerError(w, r.Context(), "fetch personal record events", err)
		return
	}
	if wrapped == nil {
		wrapped = []workoutWithEvents{}
	}
	httpresp.OK(w, "listed workouts", wrapped)
}

// get handles GET /workouts/{id}. Returns a single workout owned by
// the authed user, with any PR events it produced embedded as
// `personal_records_set`. Ownership mismatches return 404 (not 403)
// so workout IDs cannot be enumerated.
func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFrom(r.Context())
	if !ok {
		httpresp.ServerError(w, r.Context(), "missing user in context", errors.New("auth middleware not applied"))
		return
	}
	workoutID := chi.URLParam(r, "id")
	if workoutID == "" {
		httpresp.Error(w, http.StatusBadRequest, "workout id is required")
		return
	}

	existing, err := h.repo.GetByID(r.Context(), workoutID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			httpresp.Error(w, http.StatusNotFound, "workout not found")
			return
		}
		httpresp.ServerError(w, r.Context(), "get workout", err)
		return
	}
	if existing.UserID != userID {
		httpresp.Error(w, http.StatusNotFound, "workout not found")
		return
	}

	wrapped, err := h.attachPersonalRecordEvents(r.Context(), []Workout{*existing})
	if err != nil {
		httpresp.ServerError(w, r.Context(), "fetch personal record events", err)
		return
	}
	httpresp.OK(w, "fetched workout", wrapped[0])
}

// progression handles GET /workouts/progression — the muscle-group
// view that powers the Progress page.
//
// Query params:
//   - muscle_group (required): one of the catalog muscle group enum
//     values. The handler resolves it to the set of exercises that
//     target the group and aggregates across all of them.
//   - since (optional, RFC3339): start of the date range; defaults to
//     `until - 90 days` when omitted.
//   - until (optional, RFC3339): end of the date range; defaults to
//     now when omitted.
//
// Additional filter params (exercise_id, equipment, etc.) will be
// added to this endpoint over time. The intent is one progression
// endpoint that dispatches on which filter the caller provided rather
// than a separate URL per filter.
//
// For each exercise that targets the requested muscle group, the
// handler reads that exercise's full 1RM history (the table written
// by every workout CRUD), computes a recency-weighted current
// baseline, normalizes every historical entry against it, and emits
// one point per (workout, exercise) pair. The frontend plots
// everything on a single normalized axis. See
// prog-strength-docs/sows/estimated-one-rep-max-time-series-table.md
// for the full rationale.
func (h *Handler) progression(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFrom(r.Context())
	if !ok {
		httpresp.ServerError(w, r.Context(), "missing user in context", errors.New("auth middleware not applied"))
		return
	}

	q := r.URL.Query()
	muscleGroupRaw := q.Get("muscle_group")
	if muscleGroupRaw == "" {
		httpresp.Error(w, http.StatusBadRequest, "muscle_group is required")
		return
	}
	mg := exercise.MuscleGroup(muscleGroupRaw)
	if !mg.Valid() {
		httpresp.Error(w, http.StatusBadRequest, "invalid muscle_group")
		return
	}

	// `until` defaults to now; `since` defaults to until - 90 days.
	// Compute `until` first so the `since` fallback can use it.
	now := time.Now()
	until := now
	if s := q.Get("until"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			httpresp.Error(w, http.StatusBadRequest, "invalid until: must be RFC3339 format")
			return
		}
		until = t
	}

	since := until.AddDate(0, 0, -90)
	if s := q.Get("since"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			httpresp.Error(w, http.StatusBadRequest, "invalid since: must be RFC3339 format")
			return
		}
		since = t
	}

	if !since.Before(until) {
		httpresp.Error(w, http.StatusBadRequest, "since must be before until")
		return
	}

	// Resolve the muscle group to its set of catalog exercises.
	exercises, err := h.exerciseRepo.List(r.Context(), exercise.ListOptions{MuscleGroup: mg})
	if err != nil {
		httpresp.ServerError(w, r.Context(), "list exercises by muscle group", err)
		return
	}

	// For each exercise, pull a history window broad enough to cover
	// both the baseline computation (always last DefaultBaselineWindow)
	// and the chart points (respects since/until). Query the wider of
	// the two so a single fetch serves both purposes.
	histories := make([]ExerciseHistory, 0, len(exercises))
	historyFloor := now.Add(-DefaultBaselineWindow)
	if since.Before(historyFloor) {
		historyFloor = since
	}
	for _, ex := range exercises {
		entries, err := h.repo.ListOneRepMaxHistory(r.Context(), userID, ex.ID, &historyFloor, nil)
		if err != nil {
			httpresp.ServerError(w, r.Context(), "list one rep max history", err)
			return
		}
		if len(entries) == 0 {
			continue
		}
		histories = append(histories, ExerciseHistory{
			ExerciseID:   ex.ID,
			ExerciseName: ex.Name,
			Entries:      entries,
		})
	}

	result := ComputeMuscleGroupProgression(muscleGroupRaw, histories, since, until, now)
	httpresp.OK(w, "computed progression", result)
}

// create handles POST /workouts.
func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFrom(r.Context())
	if !ok {
		// Reaching this branch means the route was mounted without
		// RequireUser middleware — a wiring bug, not a user-facing error.
		httpresp.ServerError(w, r.Context(), "missing user in context", errors.New("auth middleware not applied"))
		return
	}

	var req createWorkoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpresp.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	name := req.Name
	if name == "" {
		name = fmt.Sprintf("Workout - %s", time.Now().Format("Jan 02, 2006"))
	}

	var performedAt time.Time
	var err error
	if req.PerformedAt == "" {
		performedAt = time.Now()
	} else {
		performedAt, err = time.Parse(time.RFC3339, req.PerformedAt)
		if err != nil {
			httpresp.Error(w, http.StatusBadRequest, "invalid performed_at: must be RFC3339 format")
			return
		}
	}

	endedAt, err := parseOptionalRFC3339(req.EndedAt)
	if err != nil {
		httpresp.Error(w, http.StatusBadRequest, "invalid ended_at: must be RFC3339 format")
		return
	}

	workout := &Workout{
		UserID:      userID,
		Name:        name,
		PerformedAt: performedAt,
		EndedAt:     endedAt,
		Notes:       req.Notes,
		Exercises:   make([]WorkoutExercise, len(req.Exercises)),
	}
	for i, exReq := range req.Exercises {
		workout.Exercises[i] = WorkoutExercise{
			ExerciseID:    exReq.ExerciseID,
			Order:         i,
			SupersetGroup: exReq.SupersetGroup,
			Sets:          exReq.Sets,
			Notes:         exReq.Notes,
		}
	}

	if err := h.repo.Create(r.Context(), workout); err != nil {
		if mapWorkoutValidationError(w, err) {
			return
		}
		httpresp.ServerError(w, r.Context(), "create workout", err)
		return
	}

	httpresp.Created(w, "created workout", workout)
}

// update handles PUT /workouts/{id}. Full-replacement semantics: the body
// contains the complete workout state (same shape as POST), and we replace
// the existing record. ID and UserID are sourced from the URL and the
// authed user respectively — values in the body are ignored.
//
// Unlike create, no convenience defaults are applied. An update means the
// caller is intentionally setting state; silently substituting a date-stamped
// name or "now" for performed_at would let a small omission clobber data
// the user didn't intend to touch. Required fields (performed_at, exercises)
// must be present.
//
// Ownership is enforced by loading the existing workout first and comparing
// UserID. Mismatches return 404 (not 403) so attackers can't enumerate
// workout IDs to discover other users' data.
func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFrom(r.Context())
	if !ok {
		httpresp.ServerError(w, r.Context(), "missing user in context", errors.New("auth middleware not applied"))
		return
	}

	workoutID := chi.URLParam(r, "id")
	if workoutID == "" {
		httpresp.Error(w, http.StatusBadRequest, "workout id is required")
		return
	}

	// Verify ownership before accepting the body. Treat "not yours" as 404.
	existing, err := h.repo.GetByID(r.Context(), workoutID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			httpresp.Error(w, http.StatusNotFound, "workout not found")
			return
		}
		httpresp.ServerError(w, r.Context(), "get workout", err)
		return
	}
	if existing.UserID != userID {
		httpresp.Error(w, http.StatusNotFound, "workout not found")
		return
	}

	var req createWorkoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpresp.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.PerformedAt == "" {
		httpresp.Error(w, http.StatusBadRequest, "performed_at is required")
		return
	}
	performedAt, err := time.Parse(time.RFC3339, req.PerformedAt)
	if err != nil {
		httpresp.Error(w, http.StatusBadRequest, "invalid performed_at: must be RFC3339 format")
		return
	}

	endedAt, err := parseOptionalRFC3339(req.EndedAt)
	if err != nil {
		httpresp.Error(w, http.StatusBadRequest, "invalid ended_at: must be RFC3339 format")
		return
	}

	workout := &Workout{
		ID:          workoutID,
		UserID:      userID,
		Name:        req.Name,
		PerformedAt: performedAt,
		EndedAt:     endedAt,
		Notes:       req.Notes,
		Exercises:   make([]WorkoutExercise, len(req.Exercises)),
	}
	for i, exReq := range req.Exercises {
		workout.Exercises[i] = WorkoutExercise{
			ExerciseID:    exReq.ExerciseID,
			Order:         i,
			SupersetGroup: exReq.SupersetGroup,
			Sets:          exReq.Sets,
			Notes:         exReq.Notes,
		}
	}

	if err := h.repo.Update(r.Context(), workout); err != nil {
		if mapWorkoutValidationError(w, err) {
			return
		}
		if errors.Is(err, ErrNotFound) {
			// Race: deleted between our GetByID and Update.
			httpresp.Error(w, http.StatusNotFound, "workout not found")
			return
		}
		httpresp.ServerError(w, r.Context(), "update workout", err)
		return
	}

	httpresp.OK(w, "updated workout", workout)
}

// delete handles DELETE /workouts/{id}. Soft-deletes the workout via the
// repo (sets DeletedAt); subsequent reads treat the row as gone. Ownership
// is enforced with a GetByID-then-compare pattern that returns 404 on
// mismatch, matching update — never reveal the existence of other users'
// workouts to ID enumeration.
func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFrom(r.Context())
	if !ok {
		httpresp.ServerError(w, r.Context(), "missing user in context", errors.New("auth middleware not applied"))
		return
	}

	workoutID := chi.URLParam(r, "id")
	if workoutID == "" {
		httpresp.Error(w, http.StatusBadRequest, "workout id is required")
		return
	}

	existing, err := h.repo.GetByID(r.Context(), workoutID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			httpresp.Error(w, http.StatusNotFound, "workout not found")
			return
		}
		httpresp.ServerError(w, r.Context(), "get workout", err)
		return
	}
	if existing.UserID != userID {
		httpresp.Error(w, http.StatusNotFound, "workout not found")
		return
	}

	if err := h.repo.Delete(r.Context(), workoutID); err != nil {
		if errors.Is(err, ErrNotFound) {
			// Race: deleted between our GetByID and Delete.
			httpresp.Error(w, http.StatusNotFound, "workout not found")
			return
		}
		httpresp.ServerError(w, r.Context(), "delete workout", err)
		return
	}

	httpresp.OK(w, "deleted workout", nil)
}

// createWorkoutRequest is the request body for POST /workouts (and PUT).
type createWorkoutRequest struct {
	Name        string                  `json:"name"`
	PerformedAt string                  `json:"performed_at"` // RFC3339
	EndedAt     string                  `json:"ended_at"`     // RFC3339, optional
	Notes       string                  `json:"notes"`
	Exercises   []createWorkoutExercise `json:"exercises"`
}

type createWorkoutExercise struct {
	ExerciseID    string `json:"exercise_id"`
	SupersetGroup *int   `json:"superset_group"` // optional; nil = standalone
	Notes         string `json:"notes"`
	Sets          []Set  `json:"sets"`
}

// parseOptionalRFC3339 parses an RFC3339 timestamp string, returning nil for
// an empty string. Used for optional timestamp fields like ended_at where
// "field absent" and "field present but invalid" must be distinguished.
func parseOptionalRFC3339(s string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// mapWorkoutValidationError writes a 400 response if err is a known workout
// validation error and returns true. Returns false for unknown errors so the
// caller can decide (typically: log and respond 500). Centralized so create
// and update don't duplicate the long error chain.
func mapWorkoutValidationError(w http.ResponseWriter, err error) bool {
	var invalidEnumErr *InvalidEnumError
	if errors.As(err, &invalidEnumErr) || errors.Is(err, ErrUserIDRequired) ||
		errors.Is(err, ErrPerformedAtRequired) || errors.Is(err, ErrEndedAtBeforeStart) ||
		errors.Is(err, ErrExercisesRequired) || errors.Is(err, ErrExerciseIDRequired) ||
		errors.Is(err, ErrInvalidOrder) || errors.Is(err, ErrSetsRequired) ||
		errors.Is(err, ErrInvalidReps) || errors.Is(err, ErrInvalidWeight) {
		httpresp.Error(w, http.StatusBadRequest, err.Error())
		return true
	}
	return false
}

// --- Personal Records --------------------------------------------------

// workoutWithEvents is the HTTP-shape wrapper that embeds the PR
// events produced by a workout into its JSON representation. Kept out
// of the Workout domain struct so the repository layer doesn't need
// to know about HTTP-only fields.
type workoutWithEvents struct {
	Workout
	PersonalRecordsSet []personalRecordEventDTO `json:"personal_records_set"`
}

// personalRecordEventDTO is the JSON shape for a PR break event. Defined
// here (rather than as json tags on PersonalRecordEvent) so the
// nullable previous_* fields serialize as proper JSON nulls instead
// of being omitted via omitempty.
type personalRecordEventDTO struct {
	ID             string    `json:"id"`
	ExerciseID     string    `json:"exercise_id"`
	WorkoutID      string    `json:"workout_id"`
	Weight         float64   `json:"weight"`
	Reps           int       `json:"reps"`
	Unit           string    `json:"unit"`
	PreviousWeight *float64  `json:"previous_weight"`
	PreviousReps   *int      `json:"previous_reps"`
	PreviousUnit   *string   `json:"previous_unit"`
	AchievedAt     time.Time `json:"achieved_at"`
}

func eventToDTO(e PersonalRecordEvent) personalRecordEventDTO {
	dto := personalRecordEventDTO{
		ID:             e.ID,
		ExerciseID:     e.ExerciseID,
		WorkoutID:      e.WorkoutID,
		Weight:         e.Weight,
		Reps:           e.Reps,
		Unit:           string(e.Unit),
		PreviousWeight: e.PreviousWeight,
		PreviousReps:   e.PreviousReps,
		AchievedAt:     e.AchievedAt,
	}
	if e.PreviousUnit != nil {
		u := string(*e.PreviousUnit)
		dto.PreviousUnit = &u
	}
	return dto
}

// attachPersonalRecordEvents wraps a slice of workouts with their PR
// events in a single bulk fetch. Used by the workout list and detail
// handlers so the frontend can badge PR-breaking sessions inline
// without making per-workout queries.
func (h *Handler) attachPersonalRecordEvents(
	ctx context.Context,
	workouts []Workout,
) ([]workoutWithEvents, error) {
	if len(workouts) == 0 {
		return []workoutWithEvents{}, nil
	}
	ids := make([]string, len(workouts))
	for i, w := range workouts {
		ids[i] = w.ID
	}
	events, err := h.repo.ListPersonalRecordEventsByWorkouts(ctx, ids)
	if err != nil {
		return nil, err
	}
	byWorkout := make(map[string][]personalRecordEventDTO)
	for _, e := range events {
		byWorkout[e.WorkoutID] = append(byWorkout[e.WorkoutID], eventToDTO(e))
	}
	out := make([]workoutWithEvents, len(workouts))
	for i, w := range workouts {
		out[i] = workoutWithEvents{
			Workout:            w,
			PersonalRecordsSet: byWorkout[w.ID],
		}
		if out[i].PersonalRecordsSet == nil {
			out[i].PersonalRecordsSet = []personalRecordEventDTO{}
		}
	}
	return out, nil
}

// personalRecordDTO is the JSON shape for one row of the
// /personal-records response. Carries the PR row's fields plus the
// computed current_estimated_1rm for the same exercise. Nullable
// fields use pointers so JSON renders null rather than zero values.
type personalRecordDTO struct {
	ExerciseID          string     `json:"exercise_id"`
	ExerciseName        string     `json:"exercise_name"`
	WorkoutID           *string    `json:"workout_id"`
	Weight              *float64   `json:"weight"`
	Reps                *int       `json:"reps"`
	Unit                *string    `json:"unit"`
	AchievedAt          *time.Time `json:"achieved_at"`
	CurrentEstimated1RM *float64   `json:"current_estimated_1rm"`
	Estimated1RMUnit    *string    `json:"estimated_1rm_unit"`
}

// personalRecords handles GET /personal-records.
//
// Returns one row per headline lift in HeadlineLifts order. Headline
// lifts the user has never trained still appear, with PR fields set
// to null — the frontend renders empty-state cards from these rows.
// The current_estimated_1rm field is computed per request from the
// 1RM history table; it is not stored.
func (h *Handler) personalRecords(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFrom(r.Context())
	if !ok {
		httpresp.ServerError(w, r.Context(), "missing user in context", errors.New("auth middleware not applied"))
		return
	}

	// Build a quick lookup of the user's existing PRs.
	prs, err := h.repo.ListPersonalRecords(r.Context(), userID)
	if err != nil {
		httpresp.ServerError(w, r.Context(), "list personal records", err)
		return
	}
	byExercise := make(map[string]PersonalRecord, len(prs))
	for _, pr := range prs {
		byExercise[pr.ExerciseID] = pr
	}

	// Resolve exercise display names from the catalog.
	exerciseNames := make(map[string]string, len(HeadlineLifts))
	for _, slug := range HeadlineLifts {
		ex, err := h.exerciseRepo.GetByID(r.Context(), slug)
		if err == nil {
			exerciseNames[slug] = ex.Name
		} else {
			// Catalog mismatch — the unit test guards against this, but
			// fall back to the slug so the row still renders something
			// rather than crashing the request.
			exerciseNames[slug] = slug
		}
	}

	now := time.Now()
	until := now
	since := until.Add(-DefaultBaselineWindow)

	out := make([]personalRecordDTO, 0, len(HeadlineLifts))
	for _, slug := range HeadlineLifts {
		dto := personalRecordDTO{
			ExerciseID:   slug,
			ExerciseName: exerciseNames[slug],
		}

		if pr, ok := byExercise[slug]; ok {
			weight := pr.Weight
			reps := pr.Reps
			unit := string(pr.Unit)
			workoutID := pr.WorkoutID
			achievedAt := pr.AchievedAt
			dto.Weight = &weight
			dto.Reps = &reps
			dto.Unit = &unit
			dto.WorkoutID = &workoutID
			dto.AchievedAt = &achievedAt
		}

		// Compute the current recency-weighted estimated 1RM from the
		// 1RM history table. Not stored; cheap to compute on demand.
		entries, err := h.repo.ListOneRepMaxHistory(r.Context(), userID, slug, &since, &until)
		if err != nil {
			httpresp.ServerError(w, r.Context(), "list one rep max history", err)
			return
		}
		if baseline, ok := RecencyWeightedBaseline(entries, now, DefaultBaselineWindow, DefaultBaselineTau); ok {
			rounded := round1(baseline)
			dto.CurrentEstimated1RM = &rounded
			// Unit of the baseline mirrors the most-recent in-window
			// entry's unit, which is the same convention used in the
			// muscle-group progression handler.
			for _, e := range entries {
				if !e.PerformedAt.Before(now.Add(-DefaultBaselineWindow)) {
					u := string(e.Unit)
					dto.Estimated1RMUnit = &u
					break
				}
			}
		}

		out = append(out, dto)
	}

	httpresp.OK(w, "listed personal records", out)
}
