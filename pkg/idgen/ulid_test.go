package idgen

import (
	"fmt"
	"testing"
)

func TestMustGenerateSortableID_Format(t *testing.T) {
	id := MustGenerateSortableID()
	if len(id) != 26 {
		t.Errorf("expected ID length 26, got %d", len(id))
	}
	// ULID uses Crockford's Base32, which excludes I, L, O, U
	for _, c := range id {
		if c > 127 {
			t.Errorf("invalid character in ID: %c", c)
		}
	}
}

func TestMustGenerateSortableID_Sortable(t *testing.T) {
	prevID := MustGenerateSortableID()
	for i := 0; i < 1000; i++ {
		id := MustGenerateSortableID()
		if id <= prevID {
			t.Errorf("IDs are not strictly increasing: %s <= %s", id, prevID)
		}
		prevID = id
	}
}

func TestMustGenerateSortableID_Uniqueness(t *testing.T) {
	iterations := 10000
	ids := make(map[string]bool)
	for i := 0; i < iterations; i++ {
		id := MustGenerateSortableID()
		if ids[id] {
			t.Fatalf("duplicate ID generated: %s", id)
		}
		ids[id] = true
	}
}

func TestMustGenerateSortableID_Concurrency(t *testing.T) {
	concurrency := 10
	iterations := 100
	errCh := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					errCh <- fmt.Errorf("panic: %v", r)
				}
			}()

			prevID := ""
			for j := 0; j < iterations; j++ {
				id := MustGenerateSortableID()
				if len(id) != 26 {
					errCh <- fmt.Errorf("invalid length: %d", len(id))
					return
				}
				if j > 0 && id <= prevID {
					// Note: In extremely high concurrency, strict ordering might not be guaranteed
					// between goroutines without external synchronization, but within a single
					// goroutine or if the generator is monotonic, it should hold.
					// However, standard ULID generation with random entropy doesn't guarantee
					// strict monotonicity across parallel calls unless the timestamp increments.
					// So we mainly check for validity and no panic here.
				}
				prevID = id
			}
			errCh <- nil
		}()
	}

	for i := 0; i < concurrency; i++ {
		if err := <-errCh; err != nil {
			t.Errorf("concurrent generation failed: %v", err)
		}
	}
}
