package scorecard

import "math"

// Level thresholds.  A score at or above the threshold earns that level.
const (
	GoldThreshold   = 85
	SilverThreshold = 60
)

// Evaluate runs every registered check against facts and returns a full
// report: the per-check breakdown, the weighted score, and the level.
func Evaluate(facts Facts) Report {
	results := make([]CheckResult, 0, len(Checks))

	var earned, total int
	for _, c := range Checks {
		passed, detail := c.Eval(facts)

		results = append(results, CheckResult{
			Key:    c.Key,
			Passed: passed,
			Weight: c.Weight,
			Detail: detail,
		})

		total += c.Weight
		if passed {
			earned += c.Weight
		}
	}

	score := scoreFor(earned, total)

	return Report{
		OverallScore: score,
		Level:        LevelFor(score),
		Results:      results,
	}
}

// scoreFor normalizes earned weight against total weight to a 0-100 score,
// rounded to the nearest whole number.
func scoreFor(earned, total int) int {
	if total <= 0 {
		return 0
	}
	return int(math.Round(float64(earned) / float64(total) * 100))
}

// LevelFor maps a 0-100 score to a maturity level.
func LevelFor(score int) Level {
	switch {
	case score >= GoldThreshold:
		return LevelGold
	case score >= SilverThreshold:
		return LevelSilver
	default:
		return LevelBronze
	}
}
