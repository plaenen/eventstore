package eventsourcing

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

// TimeFunc is a function that returns the current time.
// This can be overridden for testing.
var TimeFunc = time.Now

// Now returns the current time using the configured TimeFunc.
func Now() time.Time {
	return TimeFunc()
}

// generateRandomEventID generates a random unique event ID.
// This is used as a fallback when deterministic IDs are not needed.
func generateRandomEventID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err) // Should never happen
	}
	return hex.EncodeToString(b)
}

// GenerateID generates a unique identifier.
func GenerateID() string {
	return generateRandomEventID()
}
