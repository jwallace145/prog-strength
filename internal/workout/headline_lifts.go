package workout

// HeadlineLifts is the curated list of exercise slugs surfaced on the
// Personal Records view. The list lives in backend Go (rather than
// the frontend) so future mobile and web clients see the same set
// without having to be kept in sync. See the personal-records SOW.
//
// Slugs must exist in the exercise catalog; the unit test
// (TestHeadlineLifts_AllInCatalog) verifies this so a typo doesn't
// ship silently.
var HeadlineLifts = []string{
	"barbell-bench-press",
	"barbell-high-bar-back-squat",
	"barbell-deadlift",
}
