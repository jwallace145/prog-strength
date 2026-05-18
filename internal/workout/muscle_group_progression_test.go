package workout

import (
	"math"
	"testing"
	"time"

	"github.com/jwallace145/progressive-overload-fitness-tracker/internal/user"
)

// helper: build an OneRepMaxEntry with sensible defaults. All three
// per-set aggregates are set to the same `value` so the tests are
// agnostic about which one the baseline math reads — the actual
// production code uses MaxEstimated1RM.
func entry(performedAt time.Time, value float64) OneRepMaxEntry {
	return OneRepMaxEntry{
		WorkoutID:       "w-" + performedAt.Format("20060102"),
		ExerciseID:      "x",
		PerformedAt:     performedAt,
		MinEstimated1RM: value,
		AvgEstimated1RM: value,
		MaxEstimated1RM: value,
		SetCount:        3,
		Unit:            user.WeightUnitPounds,
	}
}

func TestComputeMuscleGroupProgression_Empty(t *testing.T) {
	since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	until := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	now := until

	result := ComputeMuscleGroupProgression("chest", nil, since, until, now)

	if result.MuscleGroup != "chest" {
		t.Errorf("muscle_group: got %q, want chest", result.MuscleGroup)
	}
	if len(result.Points) != 0 {
		t.Errorf("expected 0 points, got %d", len(result.Points))
	}
	if len(result.ExerciseBaselines) != 0 {
		t.Errorf("expected 0 baselines, got %d", len(result.ExerciseBaselines))
	}
	if result.Trendline != nil {
		t.Error("expected nil trendline for empty input")
	}
}

func TestComputeMuscleGroupProgression_SkipsExerciseWithoutBaseline(t *testing.T) {
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	since := now.Add(-90 * 24 * time.Hour)
	until := now

	// Only entry is 6 months old → outside the 90-day baseline window
	// → no baseline → exercise is skipped entirely.
	histories := []ExerciseHistory{{
		ExerciseID:   "stale-exercise",
		ExerciseName: "Stale Exercise",
		Entries:      []OneRepMaxEntry{entry(now.Add(-180*24*time.Hour), 200)},
	}}

	result := ComputeMuscleGroupProgression("chest", histories, since, until, now)
	if len(result.Points) != 0 {
		t.Errorf("expected exercise with no baseline to be skipped; got %d points", len(result.Points))
	}
	if len(result.ExerciseBaselines) != 0 {
		t.Errorf("expected no baselines emitted; got %d", len(result.ExerciseBaselines))
	}
}

func TestComputeMuscleGroupProgression_SingleExerciseNormalized(t *testing.T) {
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	since := now.Add(-90 * 24 * time.Hour)
	until := now

	// Three weekly entries all at the same value — the recency-weighted
	// baseline should equal that value, and the normalized point for
	// each entry should be exactly 1.0.
	histories := []ExerciseHistory{{
		ExerciseID:   "barbell-bench-press",
		ExerciseName: "Barbell Bench Press",
		Entries: []OneRepMaxEntry{
			entry(now.Add(-21*24*time.Hour), 200),
			entry(now.Add(-14*24*time.Hour), 200),
			entry(now.Add(-7*24*time.Hour), 200),
		},
	}}

	result := ComputeMuscleGroupProgression("chest", histories, since, until, now)
	if len(result.Points) != 3 {
		t.Fatalf("expected 3 points, got %d", len(result.Points))
	}
	for _, p := range result.Points {
		if math.Abs(p.NormalizedMax-1.0) > 0.001 {
			t.Errorf("constant series should normalize to 1.0; got %v for %s", p.NormalizedMax, p.PerformedAt)
		}
	}
	if len(result.ExerciseBaselines) != 1 {
		t.Fatalf("expected 1 baseline, got %d", len(result.ExerciseBaselines))
	}
	if math.Abs(result.ExerciseBaselines[0].Baseline-200) > 0.5 {
		t.Errorf("baseline should equal the constant value; got %v", result.ExerciseBaselines[0].Baseline)
	}
}

func TestComputeMuscleGroupProgression_MultipleExercisesShareAxis(t *testing.T) {
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	since := now.Add(-90 * 24 * time.Hour)
	until := now

	// Two different exercises with very different absolute strength
	// scales (barbell vs dumbbell). After normalization each entry
	// should sit at ~1.0 because each exercise's value is constant.
	histories := []ExerciseHistory{
		{
			ExerciseID:   "barbell-bench-press",
			ExerciseName: "Barbell Bench Press",
			Entries: []OneRepMaxEntry{
				entry(now.Add(-21*24*time.Hour), 250),
				entry(now.Add(-7*24*time.Hour), 250),
			},
		},
		{
			ExerciseID:   "dumbbell-bench-press",
			ExerciseName: "Dumbbell Bench Press",
			Entries: []OneRepMaxEntry{
				entry(now.Add(-14*24*time.Hour), 90),
				entry(now.Add(-3*24*time.Hour), 90),
			},
		},
	}

	result := ComputeMuscleGroupProgression("chest", histories, since, until, now)
	if len(result.Points) != 4 {
		t.Fatalf("expected 4 points across both exercises, got %d", len(result.Points))
	}
	for _, p := range result.Points {
		if math.Abs(p.NormalizedMax-1.0) > 0.01 {
			t.Errorf("normalized value should be ~1.0 regardless of exercise scale; got %v for %s", p.NormalizedMax, p.ExerciseName)
		}
	}
}

func TestComputeMuscleGroupProgression_PointsSortedByTime(t *testing.T) {
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	since := now.Add(-90 * 24 * time.Hour)
	until := now

	// Two exercises with entries interleaved in time; the final point
	// list should be globally sorted ascending so the chart renders
	// left-to-right without further sorting on the client.
	histories := []ExerciseHistory{
		{
			ExerciseID:   "a",
			ExerciseName: "A",
			Entries: []OneRepMaxEntry{
				entry(now.Add(-30*24*time.Hour), 100),
				entry(now.Add(-10*24*time.Hour), 105),
			},
		},
		{
			ExerciseID:   "b",
			ExerciseName: "B",
			Entries: []OneRepMaxEntry{
				entry(now.Add(-20*24*time.Hour), 100),
				entry(now.Add(-5*24*time.Hour), 100),
			},
		},
	}

	result := ComputeMuscleGroupProgression("chest", histories, since, until, now)
	for i := 1; i < len(result.Points); i++ {
		if result.Points[i].PerformedAt.Before(result.Points[i-1].PerformedAt) {
			t.Errorf("points not sorted by performed_at ascending at index %d", i)
		}
	}
}

func TestComputeMuscleGroupProgression_AscendingTrend(t *testing.T) {
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	since := now.Add(-90 * 24 * time.Hour)
	until := now

	// Strictly increasing avg over time — the trendline through the
	// normalized values should also slope up (end > start).
	histories := []ExerciseHistory{{
		ExerciseID:   "barbell-bench-press",
		ExerciseName: "Barbell Bench Press",
		Entries: []OneRepMaxEntry{
			entry(now.Add(-60*24*time.Hour), 200),
			entry(now.Add(-40*24*time.Hour), 210),
			entry(now.Add(-20*24*time.Hour), 220),
			entry(now.Add(-5*24*time.Hour), 230),
		},
	}}

	result := ComputeMuscleGroupProgression("chest", histories, since, until, now)
	if result.Trendline == nil {
		t.Fatal("expected non-nil trendline for 4 points across distinct dates")
	}
	if result.Trendline.EndValue <= result.Trendline.StartValue {
		t.Errorf("expected positive slope: start=%v end=%v",
			result.Trendline.StartValue, result.Trendline.EndValue)
	}
}

func TestComputeMuscleGroupProgression_EntriesOutsideWindowNotPlotted(t *testing.T) {
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	// Narrow chart window — only the last 14 days.
	since := now.Add(-14 * 24 * time.Hour)
	until := now

	// Older entries (still within the 90-day baseline) should
	// contribute to the baseline but not as plotted points.
	histories := []ExerciseHistory{{
		ExerciseID:   "barbell-bench-press",
		ExerciseName: "Barbell Bench Press",
		Entries: []OneRepMaxEntry{
			entry(now.Add(-60*24*time.Hour), 200), // baseline only
			entry(now.Add(-30*24*time.Hour), 200), // baseline only
			entry(now.Add(-10*24*time.Hour), 200), // baseline + plot
			entry(now.Add(-3*24*time.Hour), 200),  // baseline + plot
		},
	}}

	result := ComputeMuscleGroupProgression("chest", histories, since, until, now)
	if len(result.Points) != 2 {
		t.Errorf("expected only the 2 in-window entries as points, got %d", len(result.Points))
	}
}

func TestComputeMuscleGroupProgression_SinglePointNoTrendline(t *testing.T) {
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	since := now.Add(-90 * 24 * time.Hour)
	until := now

	histories := []ExerciseHistory{{
		ExerciseID:   "x",
		ExerciseName: "X",
		Entries:      []OneRepMaxEntry{entry(now.Add(-5*24*time.Hour), 200)},
	}}

	result := ComputeMuscleGroupProgression("chest", histories, since, until, now)
	if len(result.Points) != 1 {
		t.Fatalf("expected 1 point, got %d", len(result.Points))
	}
	if result.Trendline != nil {
		t.Error("expected nil trendline for a single point")
	}
}

func TestComputeMuscleGroupProgression_BaselinesSortedByName(t *testing.T) {
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	since := now.Add(-90 * 24 * time.Hour)
	until := now

	histories := []ExerciseHistory{
		{ExerciseID: "z", ExerciseName: "Z Exercise",
			Entries: []OneRepMaxEntry{entry(now.Add(-5*24*time.Hour), 100)}},
		{ExerciseID: "a", ExerciseName: "A Exercise",
			Entries: []OneRepMaxEntry{entry(now.Add(-5*24*time.Hour), 100)}},
		{ExerciseID: "m", ExerciseName: "M Exercise",
			Entries: []OneRepMaxEntry{entry(now.Add(-5*24*time.Hour), 100)}},
	}

	result := ComputeMuscleGroupProgression("chest", histories, since, until, now)
	want := []string{"A Exercise", "M Exercise", "Z Exercise"}
	if len(result.ExerciseBaselines) != len(want) {
		t.Fatalf("expected %d baselines, got %d", len(want), len(result.ExerciseBaselines))
	}
	for i, w := range want {
		if result.ExerciseBaselines[i].ExerciseName != w {
			t.Errorf("baseline[%d].name: got %q, want %q", i, result.ExerciseBaselines[i].ExerciseName, w)
		}
	}
}
