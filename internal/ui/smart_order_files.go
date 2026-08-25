package ui

import (
	"sync"
	"time"
)

// Holding the uploaded file between step 1 and step 2.
//
// The buyer uploads, then confirms a mapping, and only then are rows staged.
// The file has to survive that gap. It is held in memory rather than written to
// object storage because the gap is normally under a minute and the file is
// discarded the moment the rows are staged — persisting it would mean a retention
// policy and a cleanup job for something with a one-minute life.
//
// The trade-off is deliberate and bounded: entries expire, the map is capped,
// and an expired entry produces "re-upload the file", not a broken run. On a
// multi-instance deployment a buyer could land on a different instance and be
// asked to upload again; the chunked-upload path in ingest is the answer when
// that becomes common enough to matter.

const (
	smartOrderFileTTL   = 30 * time.Minute
	smartOrderFileLimit = 64
)

type smartOrderFile struct {
	content  []byte
	filename string
	stored   time.Time
}

type smartOrderFileStore struct {
	mu    sync.Mutex
	files map[string]smartOrderFile
}

func newSmartOrderFileStore() *smartOrderFileStore {
	return &smartOrderFileStore{files: make(map[string]smartOrderFile)}
}

func (s *smartOrderFileStore) put(runID string, content []byte, filename string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evictLocked()
	if len(s.files) >= smartOrderFileLimit {
		// Drop the oldest rather than refuse the upload: the buyer in front of
		// us matters more than one who walked away twenty minutes ago.
		var oldestKey string
		oldest := time.Now()
		for k, f := range s.files {
			if f.stored.Before(oldest) {
				oldest, oldestKey = f.stored, k
			}
		}
		delete(s.files, oldestKey)
	}
	s.files[runID] = smartOrderFile{content: content, filename: filename, stored: time.Now()}
}

func (s *smartOrderFileStore) get(runID string) ([]byte, string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evictLocked()
	f, ok := s.files[runID]
	if !ok {
		return nil, "", false
	}
	return f.content, f.filename, true
}

func (s *smartOrderFileStore) drop(runID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.files, runID)
}

func (s *smartOrderFileStore) evictLocked() {
	cutoff := time.Now().Add(-smartOrderFileTTL)
	for k, f := range s.files {
		if f.stored.Before(cutoff) {
			delete(s.files, k)
		}
	}
}
