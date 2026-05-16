package exercise

import (
	"regexp"
	"testing"
)

// slugPattern enforces lowercase kebab-case IDs: segments of [a-z0-9]
// joined by single dashes, no leading/trailing/double dashes, no other
// punctuation. Catches typos like "Pull Up", "pull--up", or "Pull-Up"
// before they ship — workout logs reference these IDs forever, so
// noticing a malformed slug at test time beats noticing it after rows
// have already been written.
var slugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// TestCatalog enforces the invariants the catalog is responsible for:
// IDs are unique, IDs are valid slugs, and every entry passes the same
// Validate() that the API would run on input from a write endpoint
// (covers missing name, missing muscle groups, unknown enum values).
//
// Sub-tests are used so failures from one concern don't mask the others
// — adding a malformed entry typically trips multiple checks at once,
// and seeing all of them in a single test run speeds up fixes.
func TestCatalog(t *testing.T) {
	t.Run("no duplicate IDs", func(t *testing.T) {
		seen := make(map[string]int, len(Catalog))
		for i, e := range Catalog {
			if prev, ok := seen[e.ID]; ok {
				t.Errorf("duplicate ID %q at index %d (first seen at index %d)", e.ID, i, prev)
				continue
			}
			seen[e.ID] = i
		}
	})

	t.Run("IDs are valid slugs", func(t *testing.T) {
		for _, e := range Catalog {
			if !slugPattern.MatchString(e.ID) {
				t.Errorf("invalid slug %q: must be lowercase kebab-case (e.g. 'barbell-bench-press')", e.ID)
			}
		}
	})

	t.Run("entries validate", func(t *testing.T) {
		for _, e := range Catalog {
			// Range-loop variable is value-copied; take its address
			// explicitly since Validate is defined on *Exercise.
			ex := e
			if err := ex.Validate(); err != nil {
				t.Errorf("%s: %v", ex.ID, err)
			}
		}
	})
}
