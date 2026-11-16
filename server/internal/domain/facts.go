// Package domain holds Apple Health facts before they are stored.
package domain

import "time"

// QuantitySample is one numeric HealthKit record.
type QuantitySample struct {
	HKUUID        string
	Type          string
	Value         float64
	ValueText     string
	Unit          string
	SourceName    string
	SourceVersion string
	Device        string
	Start         time.Time
	End           time.Time
	StartRaw      string
	EndRaw        string
	TZOffsetMin   int
	Creation      *time.Time
	Metadata      map[string]string
	Attrs         map[string]string
}

// CategorySample is one categorical HealthKit record, such as a sleep segment.
type CategorySample struct {
	HKUUID        string
	Type          string
	ValueText     string
	SourceName    string
	SourceVersion string
	Device        string
	Start         time.Time
	End           time.Time
	StartRaw      string
	EndRaw        string
	TZOffsetMin   int
	Creation      *time.Time
	Metadata      map[string]string
	Attrs         map[string]string
}

// Correlation groups child samples, such as a blood-pressure reading.
type Correlation struct {
	ID            string
	Type          string
	SourceName    string
	SourceVersion string
	Device        string
	Start         time.Time
	End           time.Time
	StartRaw      string
	EndRaw        string
	TZOffsetMin   int
	Creation      *time.Time
	Metadata      map[string]string
	Attrs         map[string]string
	Quantities    []QuantitySample
	Categories    []CategorySample
}

// WorkoutEvent is one event inside a workout.
type WorkoutEvent struct {
	Type        string
	At          time.Time
	AtRaw       string
	TZOffsetMin int
	DurationSec *float64
}

// WorkoutStatistic is one aggregate attached to a workout.
type WorkoutStatistic struct {
	Type    string
	Sum     *float64
	Minimum *float64
	Maximum *float64
	Average *float64
	Unit    string
}

// Workout is one HKWorkout.
type Workout struct {
	ID            string
	ActivityType  string
	DurationSec   *float64
	Distance      *float64
	DistanceUnit  string
	Energy        *float64
	EnergyUnit    string
	SourceName    string
	SourceVersion string
	Device        string
	Start         time.Time
	End           time.Time
	StartRaw      string
	EndRaw        string
	TZOffsetMin   int
	Creation      *time.Time
	Metadata      map[string]string
	Attrs         map[string]string
	Events        []WorkoutEvent
	Statistics    []WorkoutStatistic
}

// ActivitySummary is one daily activity ring row.
type ActivitySummary struct {
	Day              time.Time
	ActiveEnergy     *float64
	ActiveEnergyGoal *float64
	ActiveEnergyUnit string
	ExerciseMinutes  *float64
	ExerciseGoal     *float64
	StandHours       *float64
	StandGoal        *float64
	MoveMinutes      *float64
	MoveGoal         *float64
	Attrs            map[string]string
}
