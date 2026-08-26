package smartorder

// How far along a run is, as one number.
//
// A buyer watching an import wants one question answered — how much longer? —
// and the screen used to answer it with a list of stage names, each carrying a
// count that was always "0 / 804" because every stage reported its progress
// before doing any work. Four lines of text that told them nothing, four times
// over.
//
// So each stage owns a band of the bar, and progress inside a stage moves
// through that band. The bands are sized by how long each stage actually takes
// on a real file, not by how many stages there are: the exact tiers are a
// handful of queries, the fuzzy scorer is CPU over the whole residue, and
// adjudication is the one that waits on a network. Weighting them equally would
// produce a bar that sprints to 60% and then sits still, which is worse than no
// bar at all.

// stageBand is a stage's share of the overall bar, as percentages.
type stageBand struct{ start, end int }

// stageBands are measured shares, in pipeline order. They must be contiguous
// and end at 100, or the bar jumps.
var stageBands = map[Stage]stageBand{
	StageParse:      {0, 4},
	StageNormalize:  {4, 8},
	StageResolve:    {8, 30},
	StageCandidates: {30, 55},
	StageAdjudicate: {55, 84},
	StageSelect:     {84, 99},
	StageFinalize:   {99, 100},
}

// Band returns the stage's share of the overall bar.
func (s Stage) Band() (start, end int) {
	b, ok := stageBands[s]
	if !ok {
		return 0, 0
	}
	return b.start, b.end
}

// Label renders a stage in Arabic, for the one line of text the progress screen
// shows beside the number.
func (s Stage) Label() string {
	switch s {
	case StageParse:
		return "قراءة الملف"
	case StageNormalize:
		return "تجهيز الأصناف"
	case StageResolve:
		return "مطابقة الأكواد والأسماء"
	case StageCandidates:
		return "مطابقة الأصناف المتبقية"
	case StageAdjudicate:
		return "مطابقة ذكية للأصناف الصعبة"
	case StageSelect:
		return "البحث عن الموردين"
	case StageFinalize:
		return "إنهاء الطلب"
	}
	return "جارٍ المعالجة"
}

// Percent is how far through the whole run this event sits.
//
// Within a stage it interpolates across that stage's band using the event's own
// counts, so the AI tier — the one that takes real time — moves the bar batch by
// batch instead of jumping from 55 to 84 after a silent minute.
func (e *Event) Percent() int {
	start, end := e.Stage.Band()
	if end <= start {
		return start
	}
	if e.Processed == nil || e.Total == nil || *e.Total <= 0 {
		return start
	}

	done := *e.Processed
	if done < 0 {
		done = 0
	}
	if done > *e.Total {
		done = *e.Total
	}
	return start + (end-start)*done / *e.Total
}

// RunPercent is how far a run has got, from the events recorded for it.
//
// It never goes backwards. Events arrive in order, but a stage that reports its
// start after a slower stage reported its finish would otherwise drag the bar
// back — and a bar that retreats reads as a fault even when nothing is wrong.
func RunPercent(events []*Event) int {
	high := 0
	for _, e := range events {
		if p := e.Percent(); p > high {
			high = p
		}
	}
	return high
}

// CurrentStage is the stage of the most recent event, for the caption.
func CurrentStage(events []*Event) Stage {
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Stage != "" {
			return events[i].Stage
		}
	}
	return ""
}
