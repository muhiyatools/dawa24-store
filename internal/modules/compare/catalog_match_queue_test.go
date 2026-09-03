package compare

import (
	"sync"
	"testing"
	"time"
)

// A file already queued or running must not be queued again: two matchers over
// the same rows would race each other's writes.
func TestEnqueueCatalogMatchDeduplicates(t *testing.T) {
	s := &Service{repo: stubRepoForQueue{}, catalog: stubCatalogForQueue{}}
	// Claim the queue's machinery without letting workers drain it, so the
	// in-flight bookkeeping is observable.
	s.matchQ.once.Do(func() {
		s.matchQ.jobs = make(chan matchJob, matchQueueDepth)
		s.matchQ.inFlight = make(map[int64]bool)
	})

	if !s.EnqueueCatalogMatch(7, false, nil) {
		t.Fatal("the first enqueue of file 7 was refused")
	}
	if s.EnqueueCatalogMatch(7, false, nil) {
		t.Fatal("file 7 was queued twice; two matchers would write the same rows")
	}
	if !s.EnqueueCatalogMatch(8, false, nil) {
		t.Fatal("a different file was refused while file 7 was queued")
	}
	if got := len(s.matchQ.jobs); got != 2 {
		t.Fatalf("queue holds %d jobs, want 2", got)
	}
}

// A batch upload enqueues from several goroutines at once. Every distinct file
// must land exactly once and none may be lost.
func TestEnqueueCatalogMatchIsConcurrencySafe(t *testing.T) {
	s := &Service{repo: stubRepoForQueue{}, catalog: stubCatalogForQueue{}}
	s.matchQ.once.Do(func() {
		s.matchQ.jobs = make(chan matchJob, matchQueueDepth)
		s.matchQ.inFlight = make(map[int64]bool)
	})

	const files = 40
	var wg sync.WaitGroup
	accepted := make([]bool, files+1)
	for i := 1; i <= files; i++ {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			accepted[id] = s.EnqueueCatalogMatch(id, false, nil)
		}(int64(i))
	}
	wg.Wait()

	for i := 1; i <= files; i++ {
		if !accepted[i] {
			t.Fatalf("file %d was dropped from a %d-file batch", i, files)
		}
	}
	if got := len(s.matchQ.jobs); got != files {
		t.Fatalf("queue holds %d jobs, want %d — a batch upload lost work", got, files)
	}
}

// The queue refuses rather than blocking the uploader's response.
func TestEnqueueCatalogMatchDoesNotBlockWhenFull(t *testing.T) {
	s := &Service{repo: stubRepoForQueue{}, catalog: stubCatalogForQueue{}}
	s.matchQ.once.Do(func() {
		s.matchQ.jobs = make(chan matchJob, 1)
		s.matchQ.inFlight = make(map[int64]bool)
	})

	s.EnqueueCatalogMatch(1, false, nil)

	done := make(chan bool, 1)
	go func() { done <- s.EnqueueCatalogMatch(2, false, nil) }()

	select {
	case ok := <-done:
		if ok {
			t.Fatal("a full queue accepted a job")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("EnqueueCatalogMatch blocked on a full queue; an upload would hang")
	}

	// A refused file is not left claimed, or it could never be matched again.
	s.matchQ.mu.Lock()
	claimed := s.matchQ.inFlight[2]
	s.matchQ.mu.Unlock()
	if claimed {
		t.Fatal("a refused file stayed marked in-flight and can never be retried")
	}
}

// A service with no catalogue source cannot match, and must say so rather than
// silently queueing work nothing will do.
func TestEnqueueCatalogMatchRefusesWithoutCatalog(t *testing.T) {
	s := &Service{repo: stubRepoForQueue{}}
	if s.EnqueueCatalogMatch(1, false, nil) {
		t.Fatal("queued a match with no catalogue source configured")
	}
}

type stubRepoForQueue struct{ Repository }
type stubCatalogForQueue struct{ CatalogSource }
