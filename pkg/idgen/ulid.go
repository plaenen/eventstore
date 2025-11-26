package idgen

import (
	"math/rand"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
)

var (
	entropy     = ulid.Monotonic(rand.New(rand.NewSource(time.Now().UnixNano())), 0)
	entropyLock sync.Mutex
)

func MustGenerateSortableID() string {
	entropyLock.Lock()
	defer entropyLock.Unlock()

	ms := ulid.Timestamp(time.Now())
	id, err := ulid.New(ms, entropy)
	if err != nil {
		panic(err)
	}
	return id.String()
}
