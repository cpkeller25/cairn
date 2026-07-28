package scorecard

import (
	"fmt"
	"strings"
)

// RecentActivityDays is how recently a repo must have been committed to in
// order to pass the recent_activity check.
const RecentActivityDays = 90

// Check is one scoring rule.  Eval reports whether the facts satisfy the rule,
// along with a human-readable explanation.
type Check struct {
	Key         string
	Description string
	Weight      int
	Eval        func(Facts) (passed bool, detail string)
}

// Checks is the ordered set of rules the engine runs.  Weights need not sum to
// 100; the score is normalised against the total.
var Checks = []Check{
	{
		Key:         "has_readme",
		Description: "Repository has a README",
		Weight:      10,
		Eval: func(f Facts) (bool, string) {
			if f.HasReadme {
				return true, "README found"
			}
			return false, "no README found"
		},
	},
	{
		Key:         "has_ci",
		Description: "Repository has a CI pipeline",
		Weight:      20,
		Eval: func(f Facts) (bool, string) {
			if f.HasCI {
				return true, "CI configuration found"
			}
			return false, "no CI configuration found"
		},
	},
	{
		Key:         "has_tests",
		Description: "Repository contains tests",
		Weight:      20,
		Eval: func(f Facts) (bool, string) {
			if f.HasTests {
				return true, "test files detected"
			}
			return false, "no test files detected"
		},
	},
	{
		Key:         "has_dockerfile",
		Description: "Repository has a Dockerfile",
		Weight:      10,
		Eval: func(f Facts) (bool, string) {
			if f.HasDockerfile {
				return true, "Dockerfile found"
			}
			return false, "no Dockerfile found"
		},
	},
	{
		Key:         "has_license",
		Description: "Repository has a LICENSE",
		Weight:      10,
		Eval: func(f Facts) (bool, string) {
			if f.HasLicense {
				return true, "LICENSE found"
			}
			return false, "no LICENSE found"
		},
	},
	{
		Key:         "has_owner",
		Description: "Catalog entry names an owning team",
		Weight:      15,
		Eval: func(f Facts) (bool, string) {
			if team := strings.TrimSpace(f.OwnerTeam); team != "" {
				return true, "owned by " + team
			}
			return false, "no owning team recorded"
		},
	},
	{
		Key:         "recent_activity",
		Description: fmt.Sprintf("Committed to within %d days", RecentActivityDays),
		Weight:      15,
		Eval: func(f Facts) (bool, string) {
			if f.LastCommitAt.IsZero() {
				return false, "no commit date available"
			}
			days := int(f.FetchedAt.Sub(f.LastCommitAt).Hours() / 24)
			if days < 0 {
				return false, "last commit is in the future"
			}
			if days <= RecentActivityDays {
				return true, fmt.Sprintf("last commit %d days ago", days)
			}
			return false, fmt.Sprintf("last commit %d days ago", days)
		},
	},
}
