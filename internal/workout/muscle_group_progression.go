package workout

import (
	"sort"
	"time"
)

// MuscleGroupProgression is the response body for GET
// /workouts/muscle-group-progression. It normalizes per-(workout,
// exercise) estimated 1RM history against each exercise's current
// recency-weighted baseline so disparate exercises within a muscle
// group can be plotted on a single comparable Y-axis.
//
// See prog-strength-docs/sows/estimated-one-rep-max.md
// for the full design rationale. The short version: a lifter's chest
// strength is a single thing, but the absolute 1RM on barbell bench
// vs dumbbell bench vs cable fly lives on different scales. Dividing
// each session's 1RM by that exercise's current baseline turns it
// into a fraction of the lifter's current capability on that exercise
// — a number that means the same thing across every exercise.
type MuscleGroupProgression struct {
	MuscleGroup string    `json:"muscle_group"`
	Since       time.Time `json:"since"`
	Until       time.Time `json:"until"`

	// ExerciseBaselines lists, for each exercise that contributed at
	// least one point, the current recency-weighted 1RM baseline used
	// for normalization. The frontend uses these for tooltip context
	// and chart legends. Sorted by exercise_name for stable rendering.
	ExerciseBaselines []ExerciseBaseline `json:"exercise_baselines"`

	// Points is one entry per (workout, exercise) pair where the
	// exercise targets this muscle group and a baseline could be
	// computed. Sorted by performed_at ascending so charts render
	// left-to-right without re-sorting client-side.
	Points []MuscleGroupProgressionPoint `json:"points"`

	// Trendline is the single least-squares regression through every
	// normalized point. Nil when fewer than 2 points exist or when
	// all points share the same X (regression is undefined). The
	// endpoints are evaluated at since and until so the frontend can
	// plot the line with two coordinates without re-deriving the math.
	Trendline *Trendline `json:"trendline,omitempty"`
}

// ExerciseBaseline is the per-exercise context the frontend needs to
// explain normalized values to the user. Surfaced separately from
// Points so the tooltip can show "your set was at 92% of your current
// barbell bench press capability (~245 lb)" without the chart having
// to carry the baseline on every point.
type ExerciseBaseline struct {
	ExerciseID   string  `json:"exercise_id"`
	ExerciseName string  `json:"exercise_name"`
	Baseline     float64 `json:"baseline"`
	Unit         string  `json:"unit"`
}

// MuscleGroupProgressionPoint is one (workout, exercise) contribution
// to the chart. NormalizedMax is the load-bearing field (it's what the
// chart's Y-axis represents); the raw fields are carried for tooltip
// rendering so the frontend can show absolute numbers alongside the
// normalized percentage.
type MuscleGroupProgressionPoint struct {
	WorkoutID    string    `json:"workout_id"`
	ExerciseID   string    `json:"exercise_id"`
	ExerciseName string    `json:"exercise_name"`
	PerformedAt  time.Time `json:"performed_at"`

	// NormalizedMax = max_estimated_1rm / current baseline. 1.0 means
	// the lifter's heaviest set today matched their current baseline
	// capability on this exercise; >1.0 above, <1.0 below. Using max
	// (rather than the per-workout avg) keeps warmup sets from
	// deflating the signal — see RecencyWeightedBaseline for the
	// full rationale. This is what gets plotted.
	NormalizedMax float64 `json:"normalized_max"`

	// Raw per-set aggregates carried for the tooltip.
	AvgEstimated1RM float64 `json:"avg_estimated_1rm"`
	MaxEstimated1RM float64 `json:"max_estimated_1rm"`
	MinEstimated1RM float64 `json:"min_estimated_1rm"`
	SetCount        int     `json:"set_count"`
	Unit            string  `json:"unit"`
}

// ExerciseHistory pairs the read-side entries for one exercise with
// that exercise's display name. The handler builds this up from the
// catalog + repo queries; ComputeMuscleGroupProgression takes it as
// an opaque slice so the pure-math part has no IO dependency.
type ExerciseHistory struct {
	ExerciseID   string
	ExerciseName string
	// Entries spans a wide enough range to compute both the baseline
	// (which always looks at the last DefaultBaselineWindow) and the
	// chart points (which respect since/until). Filtering by
	// performed_at happens inside ComputeMuscleGroupProgression.
	Entries []OneRepMaxEntry
}

// ComputeMuscleGroupProgression turns per-exercise 1RM history into
// the normalized, charted progression for a muscle group.
//
// Algorithm:
//
//  1. For each exercise's entries, compute the recency-weighted
//     baseline as of `now` (not `until`) using DefaultBaselineWindow
//     and DefaultBaselineTau. The "current capability" anchor is the
//     present, not the end of the query window — a lifter querying
//     "last 90 days" expects to see how their past sat relative to
//     where they are *now*, not where they were three months ago.
//
//  2. Skip exercises without a computable baseline (no entries in the
//     last DefaultBaselineWindow). Without a baseline there is no
//     way to normalize, and showing raw 1RMs in a chart that's
//     supposed to mean "fraction of current capability" would mislead.
//
//  3. For each entry inside [since, until], emit one point with
//     normalized_max = max_estimated_1rm / baseline.
//
//  4. Sort points by performed_at ascending and fit one trendline
//     through them all. Single trendline is the right answer: every
//     point is on the same normalized axis, so the regression is
//     valid across exercises.
//
// Pure function: no IO, no time.Now lookups. Caller supplies `now`
// so tests can pin the baseline calculation deterministically.
func ComputeMuscleGroupProgression(
	muscleGroup string,
	histories []ExerciseHistory,
	since, until, now time.Time,
) MuscleGroupProgression {
	result := MuscleGroupProgression{
		MuscleGroup:       muscleGroup,
		Since:             since,
		Until:             until,
		ExerciseBaselines: []ExerciseBaseline{},
		Points:            []MuscleGroupProgressionPoint{},
	}

	for _, h := range histories {
		baseline, ok := RecencyWeightedBaseline(
			h.Entries, now, DefaultBaselineWindow, DefaultBaselineTau,
		)
		if !ok || baseline <= 0 {
			continue
		}

		// Unit of the baseline is the most-recent in-window entry's
		// unit. In normal training that's stable per exercise; if a
		// user has mixed units within the window the value is still
		// meaningful since baselines and entries share the same unit.
		var baselineUnit string
		for _, e := range h.Entries {
			if !e.PerformedAt.Before(now.Add(-DefaultBaselineWindow)) {
				baselineUnit = string(e.Unit)
				break
			}
		}

		contributed := false
		for _, e := range h.Entries {
			if e.PerformedAt.Before(since) || e.PerformedAt.After(until) {
				continue
			}
			result.Points = append(result.Points, MuscleGroupProgressionPoint{
				WorkoutID:       e.WorkoutID,
				ExerciseID:      e.ExerciseID,
				ExerciseName:    h.ExerciseName,
				PerformedAt:     e.PerformedAt,
				NormalizedMax:   round3(e.MaxEstimated1RM / baseline),
				AvgEstimated1RM: round1(e.AvgEstimated1RM),
				MaxEstimated1RM: round1(e.MaxEstimated1RM),
				MinEstimated1RM: round1(e.MinEstimated1RM),
				SetCount:        e.SetCount,
				Unit:            string(e.Unit),
			})
			contributed = true
		}

		if contributed {
			result.ExerciseBaselines = append(result.ExerciseBaselines, ExerciseBaseline{
				ExerciseID:   h.ExerciseID,
				ExerciseName: h.ExerciseName,
				Baseline:     round1(baseline),
				Unit:         baselineUnit,
			})
		}
	}

	sort.Slice(result.Points, func(i, j int) bool {
		return result.Points[i].PerformedAt.Before(result.Points[j].PerformedAt)
	})
	sort.Slice(result.ExerciseBaselines, func(i, j int) bool {
		return result.ExerciseBaselines[i].ExerciseName < result.ExerciseBaselines[j].ExerciseName
	})

	if len(result.Points) >= 2 {
		result.Trendline = normalizedRegressionLine(result.Points, since, until)
	}

	return result
}

// normalizedRegressionLine fits a least-squares line through the
// NormalizedMax field across every point, regardless of which exercise
// the point came from. Returns nil when the X-variance is zero so we
// don't render a degenerate line.
//
// Mirrors the math in progression.go's regressionLine but doesn't
// share code — duplicating ~20 lines is cheaper than introducing a
// generic over ProgressionPoint vs MuscleGroupProgressionPoint, and
// keeps each function readable in isolation.
func normalizedRegressionLine(
	points []MuscleGroupProgressionPoint,
	since, until time.Time,
) *Trendline {
	n := len(points)
	if n < 2 {
		return nil
	}

	firstX := points[0].PerformedAt.UnixMilli()
	allSameX := true
	for i := 1; i < n; i++ {
		if points[i].PerformedAt.UnixMilli() != firstX {
			allSameX = false
			break
		}
	}
	if allSameX {
		return nil
	}

	var sumX, sumY, sumXY, sumXX float64
	for _, p := range points {
		x := float64(p.PerformedAt.UnixMilli())
		y := p.NormalizedMax
		sumX += x
		sumY += y
		sumXY += x * y
		sumXX += x * x
	}
	denom := float64(n)*sumXX - sumX*sumX
	if denom <= 0 {
		return nil
	}
	slope := (float64(n)*sumXY - sumX*sumY) / denom
	intercept := (sumY - slope*sumX) / float64(n)

	startX := float64(since.UnixMilli())
	endX := float64(until.UnixMilli())
	return &Trendline{
		StartAt:    since,
		StartValue: round3(slope*startX + intercept),
		EndAt:      until,
		EndValue:   round3(slope*endX + intercept),
	}
}

// round3 keeps normalized values readable in JSON ("0.927") without
// losing meaningful precision. Three decimals matches the granularity
// the chart actually uses on the Y-axis.
func round3(v float64) float64 {
	return float64(int(v*1000+0.5)) / 1000
}
