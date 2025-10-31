package domain

import (
	"encoding/json"
	"time"
)

// EventStats tracks application statistics for a single event type.
type EventStats struct {
	// EventType is the type of event being tracked
	EventType string `json:"event_type"`

	// Count is the number of times this event type was applied
	Count int64 `json:"count"`

	// FirstApplied is the timestamp of the first time this event was applied
	FirstApplied time.Time `json:"first_applied"`

	// LastApplied is the timestamp of the most recent application
	LastApplied time.Time `json:"last_applied"`
}

// EventAnalytics tracks which events have been applied to an aggregate.
// This provides valuable debugging and analytical information about the
// aggregate's lifecycle and event distribution.
type EventAnalytics struct {
	// Stats maps event types to their application statistics
	Stats map[string]*EventStats `json:"stats"`

	// TotalEvents is the total number of events applied
	TotalEvents int64 `json:"total_events"`

	// LastUpdated is when the analytics were last updated
	LastUpdated time.Time `json:"last_updated"`
}

// NewEventAnalytics creates a new EventAnalytics instance.
func NewEventAnalytics() *EventAnalytics {
	return &EventAnalytics{
		Stats:       make(map[string]*EventStats),
		TotalEvents: 0,
		LastUpdated: Now(),
	}
}

// RecordEvent records that an event of the given type was applied.
func (a *EventAnalytics) RecordEvent(eventType string, timestamp time.Time) {
	if a.Stats == nil {
		a.Stats = make(map[string]*EventStats)
	}

	stats, exists := a.Stats[eventType]
	if !exists {
		// First time seeing this event type
		stats = &EventStats{
			EventType:    eventType,
			Count:        0,
			FirstApplied: timestamp,
			LastApplied:  timestamp,
		}
		a.Stats[eventType] = stats
	}

	// Update stats
	stats.Count++
	stats.LastApplied = timestamp

	// Update totals
	a.TotalEvents++
	a.LastUpdated = Now()
}

// GetEventTypes returns a list of all event types that have been applied.
func (a *EventAnalytics) GetEventTypes() []string {
	types := make([]string, 0, len(a.Stats))
	for eventType := range a.Stats {
		types = append(types, eventType)
	}
	return types
}

// GetStats returns statistics for a specific event type.
// Returns nil if the event type has never been applied.
func (a *EventAnalytics) GetStats(eventType string) *EventStats {
	return a.Stats[eventType]
}

// GetCount returns the number of times an event type was applied.
func (a *EventAnalytics) GetCount(eventType string) int64 {
	if stats := a.Stats[eventType]; stats != nil {
		return stats.Count
	}
	return 0
}

// GetMostFrequent returns the event type that was applied most frequently.
// Returns empty string if no events have been applied.
func (a *EventAnalytics) GetMostFrequent() string {
	var mostFrequent string
	var maxCount int64

	for eventType, stats := range a.Stats {
		if stats.Count > maxCount {
			maxCount = stats.Count
			mostFrequent = eventType
		}
	}

	return mostFrequent
}

// GetLeastFrequent returns the event type that was applied least frequently.
// Returns empty string if no events have been applied.
func (a *EventAnalytics) GetLeastFrequent() string {
	var leastFrequent string
	var minCount int64 = -1

	for eventType, stats := range a.Stats {
		if minCount == -1 || stats.Count < minCount {
			minCount = stats.Count
			leastFrequent = eventType
		}
	}

	return leastFrequent
}

// GetDistribution returns a map of event types to their percentage of total events.
func (a *EventAnalytics) GetDistribution() map[string]float64 {
	if a.TotalEvents == 0 {
		return make(map[string]float64)
	}

	distribution := make(map[string]float64, len(a.Stats))
	for eventType, stats := range a.Stats {
		distribution[eventType] = float64(stats.Count) / float64(a.TotalEvents) * 100.0
	}

	return distribution
}

// Clone creates a deep copy of the analytics.
func (a *EventAnalytics) Clone() *EventAnalytics {
	if a == nil {
		return nil
	}

	clone := &EventAnalytics{
		Stats:       make(map[string]*EventStats, len(a.Stats)),
		TotalEvents: a.TotalEvents,
		LastUpdated: a.LastUpdated,
	}

	for eventType, stats := range a.Stats {
		clone.Stats[eventType] = &EventStats{
			EventType:    stats.EventType,
			Count:        stats.Count,
			FirstApplied: stats.FirstApplied,
			LastApplied:  stats.LastApplied,
		}
	}

	return clone
}

// MarshalJSON serializes the analytics to JSON.
func (a *EventAnalytics) MarshalJSON() ([]byte, error) {
	if a == nil {
		return []byte("null"), nil
	}

	type Alias EventAnalytics
	return json.Marshal((*Alias)(a))
}

// UnmarshalJSON deserializes the analytics from JSON.
func (a *EventAnalytics) UnmarshalJSON(data []byte) error {
	type Alias EventAnalytics
	aux := (*Alias)(a)
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	// Ensure maps are initialized
	if a.Stats == nil {
		a.Stats = make(map[string]*EventStats)
	}

	return nil
}

// Reset clears all analytics data.
func (a *EventAnalytics) Reset() {
	a.Stats = make(map[string]*EventStats)
	a.TotalEvents = 0
	a.LastUpdated = Now()
}

// Merge combines analytics from another EventAnalytics instance.
// This is useful when combining analytics from multiple sources.
func (a *EventAnalytics) Merge(other *EventAnalytics) {
	if other == nil {
		return
	}

	for eventType, otherStats := range other.Stats {
		existingStats, exists := a.Stats[eventType]
		if !exists {
			// Copy the stats
			a.Stats[eventType] = &EventStats{
				EventType:    otherStats.EventType,
				Count:        otherStats.Count,
				FirstApplied: otherStats.FirstApplied,
				LastApplied:  otherStats.LastApplied,
			}
		} else {
			// Merge counts
			existingStats.Count += otherStats.Count

			// Keep earliest FirstApplied
			if otherStats.FirstApplied.Before(existingStats.FirstApplied) {
				existingStats.FirstApplied = otherStats.FirstApplied
			}

			// Keep latest LastApplied
			if otherStats.LastApplied.After(existingStats.LastApplied) {
				existingStats.LastApplied = otherStats.LastApplied
			}
		}
	}

	a.TotalEvents += other.TotalEvents
	a.LastUpdated = Now()
}
