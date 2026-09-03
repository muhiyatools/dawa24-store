package compare

import (
	"context"
	"sync"
	"time"
)

// Automatic catalogue matching.
//
// Matching a file against the catalogue is the step that makes an uploaded
// spreadsheet useful: until it runs, every row is a string with a price and
// nothing ties it to a product. It used to be a button the user pressed once
// per file, which meant a ten-file batch needed ten clicks and, in practice,
// went unmatched.
//
// It now runs by itself for every file that finishes uploading. That turns one
// deliberate action into an automatic one, so the two things that were fine at
// one-at-a-time have to be handled:
//
//   - Concurrency. Each match walks the catalogue and writes back every row.
//     Ten of those at once exhausts the connection pool and takes the site down
//     with it. The queue below runs a fixed two at a time, whatever arrives.
//   - Duplication. An upload auto-queues a file and the user may still press
//     the button. `inFlight` keeps the same file from being matched twice at
//     once, which would have two writers racing over the same rows.
//
// The queue is deliberately in-process and unpersisted: a match is derived
// data, cheap to redo, and the button is still there. Losing a queued match to
// a restart costs a click, not correctness.

const (
	// matchQueueWorkers is how many catalogue matches may run at once. Two is
	// chosen against the connection pool, not against throughput: a match holds
	// a connection for its whole run.
	matchQueueWorkers = 2

	// matchQueueDepth is how many files may be waiting. A batch upload is the
	// realistic producer, and the compare tool caps a user at far fewer files
	// than this; a full queue drops the request rather than blocking the
	// uploader's response.
	matchQueueDepth = 256

	// matchJobTimeout bounds one file's match.
	matchJobTimeout = 10 * time.Minute
)

type matchJob struct {
	fileID int64
	useAI  bool
	orgID  *int64
}

type matchQueue struct {
	once     sync.Once
	jobs     chan matchJob
	mu       sync.Mutex
	inFlight map[int64]bool
}

// EnqueueCatalogMatch schedules a catalogue match for one file and returns
// immediately. It reports whether the file was accepted: false means the same
// file is already queued or running, or the queue is full.
func (s *Service) EnqueueCatalogMatch(fileID int64, useAI bool, orgID *int64) bool {
	if s == nil || s.repo == nil || s.catalog == nil || fileID <= 0 {
		return false
	}
	q := &s.matchQ
	q.once.Do(func() {
		q.jobs = make(chan matchJob, matchQueueDepth)
		q.inFlight = make(map[int64]bool)
		for i := 0; i < matchQueueWorkers; i++ {
			go s.runMatchWorker()
		}
	})

	q.mu.Lock()
	if q.inFlight[fileID] {
		q.mu.Unlock()
		return false
	}
	q.inFlight[fileID] = true
	q.mu.Unlock()

	select {
	case q.jobs <- matchJob{fileID: fileID, useAI: useAI, orgID: orgID}:
		return true
	default:
		// Full: release the claim so a later attempt can take it.
		q.mu.Lock()
		delete(q.inFlight, fileID)
		q.mu.Unlock()
		if s.log != nil {
			s.log.Warn("catalog match queue full; file not auto-matched", "file_id", fileID)
		}
		return false
	}
}

func (s *Service) runMatchWorker() {
	for job := range s.matchQ.jobs {
		s.runMatchJob(job)
	}
}

// runMatchJob executes one match, always releasing the file's claim.
func (s *Service) runMatchJob(job matchJob) {
	defer func() {
		s.matchQ.mu.Lock()
		delete(s.matchQ.inFlight, job.fileID)
		s.matchQ.mu.Unlock()
		if r := recover(); r != nil && s.log != nil {
			s.log.Error("panic recovered during catalog match", "panic", r, "file_id", job.fileID)
		}
	}()

	// Its own context: the request that queued this has long since been
	// answered, and the worker must not inherit that request's cancellation.
	ctx, cancel := context.WithTimeout(context.Background(), matchJobTimeout)
	defer cancel()

	stats, err := s.MatchFileRows(ctx, job.fileID, job.useAI, job.orgID)
	if err != nil {
		if s.log != nil {
			s.log.ErrorContext(ctx, "catalog match failed", "error", err, "file_id", job.fileID)
		}
		return
	}
	if s.log != nil {
		s.log.InfoContext(ctx, "catalog match completed",
			"file_id", job.fileID, "matched", stats.Matched(), "total", stats.Rows)
	}
}
