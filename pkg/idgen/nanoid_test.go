package idgen

import (
	"strings"
	"testing"
)

func TestNanoID(t *testing.T) {
	t.Run("Default", func(t *testing.T) {
		id, err := NanoID()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(id) != 21 {
			t.Errorf("expected length 21, got %d", len(id))
		}
	})

	t.Run("WithPrefix", func(t *testing.T) {
		prefix := "user"
		id, err := NanoID(WithPrefix(prefix))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.HasPrefix(id, prefix+"_") {
			t.Errorf("expected prefix %s_, got %s", prefix, id)
		}
		if len(id) != 21+len(prefix)+1 {
			t.Errorf("expected length %d, got %d", 21+len(prefix)+1, len(id))
		}
	})

	t.Run("WithLength", func(t *testing.T) {
		length := 10
		id, err := NanoID(WithLength(length))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(id) != length {
			t.Errorf("expected length %d, got %d", length, len(id))
		}
	})

	t.Run("WithAlphabet", func(t *testing.T) {
		alphabet := "abc"
		id, err := NanoID(WithAlphabet(alphabet), WithLength(10))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for _, c := range id {
			if !strings.ContainsRune(alphabet, c) {
				t.Errorf("invalid character %c in ID %s", c, id)
			}
		}
	})

	t.Run("WithPrefixAndLength", func(t *testing.T) {
		prefix := "test"
		length := 15
		id, err := NanoID(WithPrefix(prefix), WithLength(length))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.HasPrefix(id, prefix+"_") {
			t.Errorf("expected prefix %s_, got %s", prefix, id)
		}
		if len(id) != length+len(prefix)+1 {
			t.Errorf("expected length %d, got %d", length+len(prefix)+1, len(id))
		}
	})
}

func TestMustNanoID(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("MustNanoID panicked: %v", r)
		}
	}()

	id := MustNanoID()
	if len(id) != 21 {
		t.Errorf("expected length 21, got %d", len(id))
	}

	prefix := "must"
	id = MustNanoID(WithPrefix(prefix))
	if !strings.HasPrefix(id, prefix+"_") {
		t.Errorf("expected prefix %s_, got %s", prefix, id)
	}
}
