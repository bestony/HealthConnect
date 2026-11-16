package domain

import (
	"crypto/sha256"
	"encoding/binary"
)

// NaturalKey is a SHA-256 over length-prefixed parts.
// The first part is the user id, so two users can store the same Apple row.
func NaturalKey(parts ...string) []byte {
	h := sha256.New()
	var n [4]byte
	for _, part := range parts {
		binary.BigEndian.PutUint32(n[:], uint32(len(part)))
		h.Write(n[:])
		h.Write([]byte(part))
	}
	return h.Sum(nil)
}

// QuantityKey identifies one numeric sample for one user.
func QuantityKey(userID string, s QuantitySample) []byte {
	return NaturalKey(userID, "quantity", s.Type, s.StartRaw, s.EndRaw, s.ValueText, s.Unit, s.SourceName, s.Device)
}

// CategoryKey identifies one category sample for one user.
func CategoryKey(userID string, s CategorySample) []byte {
	return NaturalKey(userID, "category", s.Type, s.StartRaw, s.EndRaw, s.ValueText, s.SourceName, s.Device)
}

// CorrelationKey identifies one correlation for one user.
func CorrelationKey(userID string, c Correlation) []byte {
	return NaturalKey(userID, "correlation", c.Type, c.StartRaw, c.EndRaw, c.SourceName, c.Device)
}

// WorkoutKey identifies one workout for one user.
func WorkoutKey(userID string, w Workout) []byte {
	distance := ""
	if w.Distance != nil {
		distance = formatCanon(*w.Distance)
	}
	energy := ""
	if w.Energy != nil {
		energy = formatCanon(*w.Energy)
	}
	duration := ""
	if w.DurationSec != nil {
		duration = formatCanon(*w.DurationSec)
	}
	return NaturalKey(userID, "workout", w.ActivityType, w.StartRaw, w.EndRaw, duration, distance, w.DistanceUnit, energy, w.EnergyUnit, w.SourceName, w.Device)
}
