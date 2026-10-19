package phenotype

// Parameters are engineering defaults, not medical cutoffs. Changing any value
// requires a new SpecVersion and new reproducibility/calibration fixtures.
const (
	defaultFrequency      = .5
	backgroundFloor       = 1e-4
	backgroundCeiling     = .99
	falsePositiveFloor    = .01
	falseNegativeFloor    = .01
	exclusionPenalty      = .001
	exclusionSupport      = 1000.
	minimumLR             = .001
	maximumLR             = 1000.
	minTotalLR            = 10.
	minMargin             = 2.
	minSpecificTerms      = 2
	minInformationContent = 3.
)

func frequencyMapping() map[string]float64 {
	return map[string]float64{"HP:0040280": 1, "HP:0040281": .8, "HP:0040282": .5, "HP:0040283": .1, "HP:0040284": .01, "HP:0040285": 0}
}
