package catalog

import (
	"errors"
	"sync"
	"time"
)

// Progress for a long-running import.
//
// Preparing a nine-thousand-row file with AI enrichment is minutes of work, not
// milliseconds. Run inside the POST that started it, the admin gets a browser
// spinning on a request that may outlive its own timeout, with no way to tell a
// working import from a hung one. So preparation runs in the background and
// reports where it has reached, and the review screen polls for that.

// ImportPhase is the stage a preparation run has reached.
type ImportPhase string

const (
	// PhaseQueued means the run has not started yet.
	PhaseQueued ImportPhase = "queued"
	// ImportPhaseReading is decoding the workbook and finding its blocks.
	ImportPhaseReading ImportPhase = "reading"
	// ImportPhaseParsing is turning rows into products.
	ImportPhaseParsing ImportPhase = "parsing"
	// ImportPhaseEnriching is asking the model about the rows with gaps.
	ImportPhaseEnriching ImportPhase = "enriching"
	// ImportPhaseMatching is resolving rows against the existing catalogue.
	ImportPhaseMatching ImportPhase = "matching"
	// ImportPhaseStaging is writing the staged rows for review.
	ImportPhaseStaging ImportPhase = "staging"
	// ImportPhaseDone means the review screen can be shown.
	ImportPhaseDone ImportPhase = "done"
	// ImportPhaseFailed means preparation stopped; nothing was written.
	ImportPhaseFailed ImportPhase = "failed"
)

// Label renders a phase for the progress panel.
func (p ImportPhase) Label() string {
	switch p {
	case ImportPhaseReading:
		return "قراءة الملف وتحليل بنيته"
	case ImportPhaseParsing:
		return "استخراج الأصناف من الصفوف"
	case ImportPhaseEnriching:
		return "استكمال البيانات بالذكاء الاصطناعي"
	case ImportPhaseMatching:
		return "مطابقة الأصناف مع الكتالوج الحالي"
	case ImportPhaseStaging:
		return "تجهيز النتائج للمراجعة"
	case ImportPhaseDone:
		return "اكتملت المعالجة"
	case ImportPhaseFailed:
		return "تعذرت المعالجة"
	default:
		return "في انتظار البدء"
	}
}

// Terminal reports whether nothing further will happen.
func (p ImportPhase) Terminal() bool {
	return p == ImportPhaseDone || p == ImportPhaseFailed
}

// ImportProgress is one snapshot of a preparation run.
type ImportProgress struct {
	Phase ImportPhase `json:"phase"`
	// Current and Total describe the work inside the phase. Total of zero means
	// the phase has no meaningful count, so the bar shows as indeterminate
	// rather than pretending to a precision it does not have.
	Current int `json:"current"`
	Total   int `json:"total"`
	// Message is the phase label, or the reason it failed.
	Message   string    `json:"message"`
	StartedAt time.Time `json:"started_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Percent is how far the current phase has got, 0-100. It returns -1 when the
// phase carries no count.
func (p ImportProgress) Percent() int {
	if p.Phase == ImportPhaseDone {
		return 100
	}
	if p.Total <= 0 {
		return -1
	}
	return min(p.Current*100/p.Total, 100)
}

// Elapsed is how long the run has been going.
func (p ImportProgress) Elapsed() time.Duration {
	if p.StartedAt.IsZero() {
		return 0
	}
	return p.UpdatedAt.Sub(p.StartedAt)
}

// ProgressFunc receives progress updates. A nil ProgressFunc is valid and
// ignores them, so the synchronous path costs nothing.
type ProgressFunc func(phase ImportPhase, current, total int)

func (f ProgressFunc) report(phase ImportPhase, current, total int) {
	if f != nil {
		f(phase, current, total)
	}
}

// ProgressTracker holds the live state of the preparation runs in flight.
//
// It is memory-only and per-process on purpose. Progress is a view of work
// happening in this process right now; persisting it would add a write per
// batch to say something that stops being true the moment the process ends. The
// durable record is the session row, which reaches "ready" when the work lands.
type ProgressTracker struct {
	mu   sync.RWMutex
	runs map[string]ImportProgress
}

// NewProgressTracker creates an empty tracker.
func NewProgressTracker() *ProgressTracker {
	return &ProgressTracker{runs: map[string]ImportProgress{}}
}

// Begin registers a run and returns the function that reports its progress.
func (t *ProgressTracker) Begin(sessionID string) ProgressFunc {
	if t == nil {
		return nil
	}
	now := time.Now()

	t.mu.Lock()
	t.runs[sessionID] = ImportProgress{
		Phase: ImportPhaseReading, Message: ImportPhaseReading.Label(),
		StartedAt: now, UpdatedAt: now,
	}
	t.mu.Unlock()

	return func(phase ImportPhase, current, total int) {
		t.mu.Lock()
		defer t.mu.Unlock()
		run := t.runs[sessionID]
		run.Phase, run.Current, run.Total = phase, current, total
		run.Message = phase.Label()
		run.UpdatedAt = time.Now()
		if run.StartedAt.IsZero() {
			run.StartedAt = run.UpdatedAt
		}
		t.runs[sessionID] = run
	}
}

// Finish marks a run complete, or failed with a reason.
func (t *ProgressTracker) Finish(sessionID string, err error) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	run := t.runs[sessionID]
	run.UpdatedAt = time.Now()
	if err != nil {
		run.Phase, run.Message = ImportPhaseFailed, err.Error()
	} else {
		run.Phase, run.Message = ImportPhaseDone, ImportPhaseDone.Label()
	}
	t.runs[sessionID] = run
	t.sweepLocked()
}

// Progress reads a run's latest snapshot.
func (t *ProgressTracker) Progress(sessionID string) (ImportProgress, bool) {
	if t == nil {
		return ImportProgress{}, false
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	run, ok := t.runs[sessionID]
	return run, ok
}

// Running reports whether a run is still in flight.
func (t *ProgressTracker) Running(sessionID string) bool {
	run, ok := t.Progress(sessionID)
	return ok && !run.Phase.Terminal()
}

// progressRetention is how long a finished run stays readable, so the page that
// polls it sees the completion rather than the entry vanishing under it.
const progressRetention = 10 * time.Minute

// sweepLocked drops finished runs that nobody is going to ask about again.
func (t *ProgressTracker) sweepLocked() {
	for id, run := range t.runs {
		if run.Phase.Terminal() && time.Since(run.UpdatedAt) > progressRetention {
			delete(t.runs, id)
		}
	}
}

// AI failure conditions a caller can act on differently.
//
// They are declared here rather than reused from the Gateway package because
// the catalogue must not import a transport: the adapter translates the
// Gateway's errors into these, and the import decides what to tell the admin.
var (
	// ErrAIQuotaExceeded means the platform's AI budget is spent. It is a
	// business condition an operator can fix, not an outage.
	ErrAIQuotaExceeded = errors.New("catalog: ai quota exceeded")
	// ErrAIUnauthorized means the Gateway rejected the key.
	ErrAIUnauthorized = errors.New("catalog: ai credentials rejected")
)
