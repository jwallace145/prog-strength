package exercise

// Catalog is the canonical list of supported exercises.
// This is the source of truth until exercises are persisted in a database.
//
// To add an exercise: append to this slice. IDs are stable, human-readable
// slugs — never renumber or rename them, because workout logs will reference
// them. To remove an exercise, set DeletedAt rather than deleting the entry.
var Catalog = []Exercise{
	{
		ID:           "leg-extension",
		Name:         "Leg Extension",
		Description:  "Seated on a machine, extend the knees against a padded lever to isolate the quadriceps.",
		MuscleGroups: []MuscleGroup{MuscleQuads},
		Equipment:    []Equipment{EquipmentMachine},
	},
	{
		ID:           "machine-lying-leg-curl",
		Name:         "Machine Lying Leg Curl",
		Description:  "Lying face-down on a machine, curl the heels toward the glutes to isolate the hamstrings.",
		MuscleGroups: []MuscleGroup{MuscleHamstrings},
		Equipment:    []Equipment{EquipmentMachine},
	},
	{
		ID:           "barbell-high-bar-back-squat",
		Name:         "Barbell High Bar Back Squat",
		Description:  "Barbell held high on the traps; squat to depth with a more upright torso than the low-bar variant.",
		MuscleGroups: []MuscleGroup{MuscleQuads, MuscleGlutes, MuscleHamstrings, MuscleCore},
		Equipment:    []Equipment{EquipmentBarbell, EquipmentRack},
	},
	{
		ID:           "barbell-calf-raise",
		Name:         "Barbell Calf Raise",
		Description:  "Standing with a barbell across the upper back, raise onto the balls of the feet to target the calves.",
		MuscleGroups: []MuscleGroup{MuscleCalves},
		Equipment:    []Equipment{EquipmentBarbell},
	},
	{
		ID:           "bodyweight-squat",
		Name:         "Bodyweight Squat",
		Description:  "Squat to depth using only bodyweight; foundational movement for lower body mobility and conditioning.",
		MuscleGroups: []MuscleGroup{MuscleQuads, MuscleGlutes, MuscleHamstrings},
		Equipment:    []Equipment{EquipmentNone},
	},
	{
		ID:           "hanging-leg-raise",
		Name:         "Hanging Leg Raise",
		Description:  "Hanging from a pull-up bar, raise the legs to engage the lower abdominals and hip flexors.",
		MuscleGroups: []MuscleGroup{MuscleCore},
		Equipment:    []Equipment{EquipmentPullupBar},
	},
	{
		ID:           "decline-bench-sit-up",
		Name:         "Decline Bench Sit Up",
		Description:  "Anchored on a decline bench, perform sit-ups against gravity to target the abdominals.",
		MuscleGroups: []MuscleGroup{MuscleCore},
		Equipment:    []Equipment{EquipmentDeclineBench},
	},
	{
		ID:           "seated-cable-row",
		Name:         "Seated Cable Row",
		Description:  "Seated at a low-pulley cable station, row the handle to the abdomen to target the mid-back and lats.",
		MuscleGroups: []MuscleGroup{MuscleBack},
		Equipment:    []Equipment{EquipmentCable},
	},
	{
		ID:           "dumbbell-bench-press",
		Name:         "Dumbbell Bench Press",
		Description:  "Lying on a flat bench, press a pair of dumbbells from chest level to lockout to target the chest.",
		MuscleGroups: []MuscleGroup{MuscleChest},
		Equipment:    []Equipment{EquipmentDumbbell, EquipmentFlatBench},
	},
	{
		ID:           "barbell-bent-over-row",
		Name:         "Barbell Bent Over Row",
		Description:  "Hinged at the hips with a flat back, row a barbell to the lower chest/upper abdomen to target the mid-back and lats.",
		MuscleGroups: []MuscleGroup{MuscleBack},
		Equipment:    []Equipment{EquipmentBarbell},
	},
	{
		ID:           "incline-dumbbell-bench-press",
		Name:         "Incline Dumbbell Bench Press",
		Description:  "On an incline bench, press a pair of dumbbells from upper-chest level to lockout; the angle shifts load to the upper chest and front delts.",
		MuscleGroups: []MuscleGroup{MuscleChest, MuscleShoulders},
		Equipment:    []Equipment{EquipmentDumbbell, EquipmentInclineBench},
	},
	{
		ID:           "incline-dumbbell-fly",
		Name:         "Incline Dumbbell Fly",
		Description:  "On an incline bench with a slight elbow bend, lower dumbbells out in an arc and squeeze them back together to isolate the upper chest.",
		MuscleGroups: []MuscleGroup{MuscleChest},
		Equipment:    []Equipment{EquipmentDumbbell, EquipmentInclineBench},
	},
	{
		ID:           "pull-up",
		Name:         "Pull Up",
		Description:  "Hanging from an overhand grip on a pull-up bar, pull the chest toward the bar to target the lats and mid-back.",
		MuscleGroups: []MuscleGroup{MuscleBack},
		Equipment:    []Equipment{EquipmentPullupBar},
	},
	{
		ID:           "hanging-knee-raise",
		Name:         "Hanging Knee Raise",
		Description:  "Hanging from a pull-up bar, raise the knees toward the chest to engage the lower abdominals. Easier regression of the hanging leg raise.",
		MuscleGroups: []MuscleGroup{MuscleCore},
		Equipment:    []Equipment{EquipmentPullupBar},
	},
	{
		ID:           "cable-crunch",
		Name:         "Cable Crunch",
		Description:  "Kneeling at a high-pulley cable station with a rope attachment, crunch the torso toward the floor to target the abdominals under cable tension.",
		MuscleGroups: []MuscleGroup{MuscleCore},
		Equipment:    []Equipment{EquipmentCable},
	},
}
