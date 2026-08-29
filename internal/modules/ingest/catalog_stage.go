package ingest

// Staging: reading the file, matching every row, and asking the model about the
// rows the matcher could not settle — without writing anything the vendor has
// not yet seen.
//
// The order of the two matching passes is the whole design. The deterministic
// engine runs while the rows stream past, because it is pure arithmetic and
// there is no reason to hold nine thousand rows in memory to do it. The AI
// stage runs ONCE, afterwards, over the residue the engine left — because
// everything that makes it cheap needs to see the whole file at once:
// duplicate rows collapse onto one question, and overlapping shortlists collapse
// into one shared catalogue window. Running it inside the streaming callback,
// as this used to, threw both savings away five hundred rows at a time.

import (
	"context"
	"fmt"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// maxOpenQuestions bounds the residue the AI stage will even consider.
//
// It is the request ceiling times the items a request holds: the exact number
// of distinct questions a full run can carry. Retrieval past it could never be
// sent, so running it would be pure CPU spent on a batch that would be dropped
// at the planning step anyway. A file that exceeds it is one the column mapping
// is wrong on, and the vendor is told the ceiling was reached.
var maxOpenQuestions = ceilings.MaxRequestsPerRun * ceilings.MaxItemsPerRequest

// StageImport parses every row, scores it against the shared catalogue, asks
// the model about what is left, and stages the result for the vendor to review.
// Nothing reaches live variants or stocks until they confirm.
func (s *Service) StageImport(ctx context.Context, session *Session) error {
	analysis, err := s.analyse(ctx, session)
	if err != nil {
		return err
	}
	analysis.Complete()

	if err := s.imports.ClearRows(ctx, session.ID); err != nil {
		s.log.WarnContext(ctx, "clear import rows failed", "error", err)
	}

	index, err := s.loadCatalogIndex(ctx)
	if err != nil {
		return err
	}

	book, err := s.openBook(ctx, session)
	if err != nil {
		return err
	}
	defer func() { _ = book.Close() }()

	run := &stagingRun{
		svc:      s,
		session:  session,
		index:    index,
		match:    stagingMatchOptions(session.Settings),
		byNorm:   map[string]*openRow{},
		bucketOf: map[int]matchBucket{},
	}

	opts := productmatch.DefaultProcessOptions()
	opts.Parse = parseOptionsFrom(session.Settings)
	opts.Duplicates = session.Settings.Duplicates
	opts.Vocabulary = s.vocabulary(ctx, session.OrganizationID)

	result, err := productmatch.Process(book, analysis.Layout, analysis.Mapping, opts,
		func(batch []*productmatch.Row) error { return run.stage(ctx, batch) })
	if err != nil {
		return err
	}

	run.enhance(ctx)

	session.Stats = result.Stats
	session.Findings = result.Issues
	session.TotalRows = result.Stats.SheetRows
	session.MatchedRows = run.counts.matched
	session.ReviewRows = run.counts.review
	session.UnmatchedRows = run.counts.unmatched
	session.ErrorRows = run.counts.errors + result.Stats.Rejected
	session.Phase = PhaseReview

	return s.imports.SaveDraft(ctx, session)
}

// loadCatalogIndex builds the in-memory shared catalogue both matching passes
// read. It is loaded as the system, because the shared catalogue belongs to no
// tenant.
func (s *Service) loadCatalogIndex(ctx context.Context) (*productmatch.Index, error) {
	products, err := s.catalog.ListMatchProducts(database.AsSystem(ctx))
	if err != nil {
		return nil, fmt.Errorf("load shared catalogue: %w", err)
	}
	master := make([]productmatch.MasterProduct, 0, len(products))
	for _, p := range products {
		master = append(master, productmatch.MasterProduct{
			ID: p.ID, NameAR: p.NameAR, NameEN: p.NameEN, SKU: p.SKU,
			Barcode: p.Barcode, Scientific: p.Scientific, DosageForm: p.DosageForm,
			Concentration: p.Concentration, Unit: p.Unit,
			Manufacturer: p.Manufacturer, PublicPrice: p.PublicPrice,
		})
	}
	return productmatch.NewIndex(master), nil
}

// openBook re-reads the stored upload for the sheet the vendor chose.
func (s *Service) openBook(ctx context.Context, session *Session) (*sheet.Book, error) {
	content, err := s.imports.File(ctx, session.ID)
	if err != nil {
		return nil, err
	}
	book, err := sheet.Open(content, session.Filename)
	if err != nil {
		return nil, apperr.Validation("import.unreadable", err.Error(), nil)
	}
	if session.Source.Sheet != "" {
		_ = book.Use(session.Source.Sheet)
	}
	return book, nil
}

// stagingMatchOptions are the deterministic engine's thresholds for staging.
//
// MinReview sits well below MinStrong on purpose: a row scored between the two
// is not applied, but it IS shown to the vendor with its candidates, and it is
// the residue the AI stage is asked about. Setting them equal would hide the
// rows most likely to be a real match written differently.
func stagingMatchOptions(settings Settings) productmatch.MatchOptions {
	opts := productmatch.DefaultMatchOptions()
	opts.MinStrong = settings.MinMatchScore
	if opts.MinStrong <= 0 {
		opts.MinStrong = 0.30
	}
	opts.MinReview = min(opts.MinStrong*0.5, 0.20)
	opts.TrustSupplierCode = settings.TrustSupplierCode
	return opts
}

// stagingRun carries the state of one staging pass.
type stagingRun struct {
	svc     *Service
	session *Session
	index   *productmatch.Index
	match   productmatch.MatchOptions

	counts counters

	// byNorm collects the residue as QUESTIONS rather than rows: every row with
	// the same normalised name asks the same thing, so they share one entry and
	// one retrieval. On a supplier file that repeats a product per warehouse
	// this is where most of the saving comes from, and it costs one map.
	byNorm map[string]*openRow
	open   []*openRow
	// bucketOf remembers which counter each open row was first tallied under,
	// so an AI match can move it without the review screen disagreeing with
	// itself.
	bucketOf map[int]matchBucket
	// ceilingHit records that the residue outgrew what one run may ask about.
	ceilingHit bool
}

// stage matches one batch of parsed rows and writes them to the staging table.
func (r *stagingRun) stage(ctx context.Context, batch []*productmatch.Row) error {
	staged := make([]RowOutcome, 0, len(batch))

	for _, row := range batch {
		m := r.index.Match(row, r.match)
		bucket := bucketOf(m)
		r.count(bucket, 1)
		r.bucketOf[row.Number] = bucket

		outcome := OutcomeStaged
		if row.HasErrors() {
			outcome = OutcomeError
			r.counts.errors++
		}

		// A row the reader rejected is not worth asking about: it will not be
		// committed whatever the answer, and paying for it is pure waste.
		if !bucket.settled() && outcome != OutcomeError {
			r.remember(row, m)
		}

		if !row.HasQuantity && r.session.Settings.DefaultQuantity > 0 {
			row.Quantity = r.session.Settings.DefaultQuantity
			row.HasQuantity = true
		}

		var productID *int64
		if m.ProductID > 0 {
			id := m.ProductID
			productID = &id
		}

		staged = append(staged, RowOutcome{
			SourceRow:         row.Number,
			Outcome:           outcome,
			MatchLevel:        string(m.Level),
			MatchScore:        m.Score,
			ProductID:         productID,
			DisplayName:       row.Name,
			SourceCode:        row.SKU,
			CustomVariantName: row.Name,
			Payload:           row,
			Candidates:        m.Candidates,
			Issues:            row.Issues,
			Message:           m.Reason,
		})
	}

	return r.svc.imports.AppendRows(ctx, r.session.ID, r.session.OrganizationID, staged)
}

// remember files an unsettled row under the question it asks.
//
// The first row to ask a question owns it; later rows carrying the same
// normalised name attach their row number and nothing else. The parsed row of
// the first is the one the identity guard re-reads, which is correct because
// the guard reads the name and the strength — the very things that made these
// rows one question.
func (r *stagingRun) remember(row *productmatch.Row, m productmatch.MatchResult) {
	if len(r.byNorm) >= maxOpenQuestions {
		r.ceilingHit = true
		return
	}
	key := productmatch.NormalizeText(row.DisplayName())
	if key == "" {
		return
	}
	if known, ok := r.byNorm[key]; ok {
		known.sourceRows = append(known.sourceRows, row.Number)
		return
	}
	q := &openRow{
		row:        row,
		sourceRows: []int{row.Number},
		normName:   key,
		score:      m.Score,
	}
	if m.ProductID > 0 {
		id := m.ProductID
		q.guess = &id
	}
	r.byNorm[key] = q
	r.open = append(r.open, q)
}

// enhance runs the AI stage over the whole residue and folds what it settled
// back onto the staged rows.
//
// Every failure here is silent by design: the vendor keeps a complete,
// deterministically matched staging table, which is a usable answer, and the
// review screen says what the stage managed rather than pretending it ran.
func (r *stagingRun) enhance(ctx context.Context) {
	s := r.svc
	if !r.session.Settings.UseAI || s.enhancer == nil {
		return
	}
	if len(r.open) == 0 {
		// Every row settled deterministically. The stage still records that it
		// was on, because "nothing was left to ask" and "the smart matching
		// never ran" are different things to tell a vendor.
		r.session.AI = AIStats{Ran: true}
		return
	}

	r.progress(ctx, 45, "جارٍ تجهيز الأصناف غير المطابقة للمراجعة الذكية")

	enh := NewEnhancement(s.enhancer, s.memory, r.index, s.log)
	enh.Stats.CeilingHit = r.ceilingHit
	enh.OnProgress = func(done, total int) {
		if total <= 0 {
			return
		}
		r.progress(ctx, 50+(done*45)/total,
			fmt.Sprintf("المطابقة الذكية: %d من %d صنف", done, total))
	}

	// Retrieval is deliberately outside the streaming loop and outside the
	// request budget: it is index arithmetic that costs nothing but CPU, and
	// running it once over de-duplicated questions is a fraction of what
	// running it per row would have been.
	askable := enh.Retrieve(r.open)

	matches := enh.Run(ctx, askable)
	r.session.AI = enh.Stats

	if len(matches) > 0 {
		if err := s.imports.ApplyAIMatches(ctx, r.session.ID, matches); err != nil {
			// The rows keep their deterministic outcome and the counters are
			// left alone, so the screen and the table still agree.
			s.log.WarnContext(ctx, "AI matches not written to staging rows",
				"import", r.session.PublicID, "matches", len(matches), "error", err)
			return
		}
		r.recount(matches)
	}

	s.log.InfoContext(ctx, "vendor import AI enhancement finished",
		"import", r.session.PublicID, "questions", len(r.open),
		"reviewed", enh.Stats.Reviewed, "cache_hits", enh.Stats.CacheHits,
		"requests", enh.Stats.Requests, "improved", enh.Stats.Improved,
		"abstained", enh.Stats.Abstained, "rejected", enh.Stats.Rejected,
		"ceiling_hit", enh.Stats.CeilingHit)
}

// recount moves every AI-settled row out of the counter it was tallied under.
//
// It decrements the row's own original bucket rather than assuming "needs
// review": a row with candidates but a weak best score was counted as
// unmatched, and decrementing the wrong counter makes the review screen
// disagree with its own tabs.
func (r *stagingRun) recount(matches []AIMatch) {
	for _, m := range matches {
		bucket, ok := r.bucketOf[m.SourceRow]
		if !ok || bucket.settled() {
			continue
		}
		r.count(bucket, -1)
		r.count(bucketMatched, 1)
		r.bucketOf[m.SourceRow] = bucketMatched
	}
}

// progress records how far staging has reached, for the waiting screen.
//
// It carries the caller's context because the write is tenant-scoped: a
// progress row written outside the vendor's tenancy is a row the policy refuses
// and a bar that never moves.
func (r *stagingRun) progress(ctx context.Context, percent int, note string) {
	if err := r.svc.imports.Progress(ctx, r.session.ID, percent, note); err != nil {
		r.svc.log.WarnContext(ctx, "import progress not recorded",
			"import", r.session.PublicID, "error", err)
	}
	r.session.ProgressPercent = percent
	r.session.ProgressNote = note
}

// count adjusts one of the match counters by delta.
func (r *stagingRun) count(b matchBucket, delta int) {
	switch b {
	case bucketMatched:
		r.counts.matched += delta
	case bucketReview:
		r.counts.review += delta
	default:
		r.counts.unmatched += delta
	}
}
