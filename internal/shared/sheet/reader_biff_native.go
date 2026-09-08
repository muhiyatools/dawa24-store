package sheet

// Reading an Excel 97-2003 workbook without a library.
//
// This package already had two BIFF decoders and a real distributor file
// defeated both. الدقهليه-1.XLS holds 764 rows in four columns; its record
// stream holds 767 shared strings and 767 LABELSST cells, so every product name
// is in the file and countable. github.com/shakinm/xlsReader panics on it
// ("slice bounds out of range") and github.com/extrame/xls returns 155 of the
// 767 strings and empty cells for the rest — and the import that followed said,
// truthfully as far as it could see, that 609 of the vendor's 764 products had
// no name and could not be matched. That is the screen this file was written
// for.
//
// What both libraries get wrong is the same thing: the shared-string table is
// larger than one BIFF record, so it continues into CONTINUE records, and a
// string may be cut in half at that boundary. The continuation then begins with
// a fresh encoding byte describing the REMAINDER of that string — the one rule
// in the format that cannot be discovered by reading a well-formed small file,
// and the one that decides whether the second half of a price list is readable.
//
// So the decoder here is deliberately narrow. It reads BIFF8, it reads the
// records that carry values, and it hands the grid to the same finishGrid the
// other readers use. It does not write, does not evaluate formulas, and does
// not format numbers: a date reaches the pipeline as its serial, which
// CoerceDate already reads.

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strconv"
)

// BIFF record numbers, in the order this file uses them.
const (
	recFormula    = 0x0006
	recEOF        = 0x000A
	recFilePass   = 0x002F
	recContinue   = 0x003C
	recBoundSheet = 0x0085
	recMulRK      = 0x00BD
	recMulBlank   = 0x00BE
	recRString    = 0x00D6
	recSST        = 0x00FC
	recLabelSST   = 0x00FD
	recBlank      = 0x0201
	recNumber     = 0x0203
	recLabel      = 0x0204
	recBoolErr    = 0x0205
	recString     = 0x0207
	recRK         = 0x027E
	recBOF        = 0x0809
)

// biffVersion8 is the only version this decoder reads. Earlier workbooks have a
// different string encoding and no shared-string table, and the existing
// decoders handle them; a file this one refuses simply falls through to them.
const biffVersion8 = 0x0600

// Bounds on what a workbook may claim, so a malformed file cannot make the
// importer allocate without limit.
const (
	maxBiffRows   = 1 << 20
	maxBiffCols   = 1 << 14
	maxSSTStrings = 1 << 21
)

// biffRecord is one record's number and body, with CONTINUE records kept
// separate rather than glued on: whether a byte sits at a record boundary is
// what the shared-string reader has to know.
type biffRecord struct {
	id   uint16
	body []byte
	// continues are the CONTINUE records that followed this one, in order.
	continues [][]byte
	// offset is where this record began in the workbook stream, which is how a
	// BOUNDSHEET points at its worksheet.
	offset int
}

// openXLSNative decodes a legacy workbook with this package's own BIFF reader.
func (b *Book) openXLSNative() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("native XLS decode panicked: %v", r)
		}
	}()

	container, err := openCompoundFile(b.content)
	if err != nil {
		return err
	}
	stream, err := workbookStream(container)
	if err != nil {
		return err
	}
	records, err := splitBiffRecords(stream)
	if err != nil {
		return err
	}

	globals, err := globalsOf(records)
	if err != nil {
		return err
	}
	if len(globals.sheets) == 0 {
		return errors.New("native XLS: workbook declares no worksheets")
	}
	// A workbook with no shared strings is one whose cells are all numeric, and
	// it decodes perfectly well with an empty table.
	var sst []string
	if globals.sst >= 0 {
		sst = readSharedStrings(records[globals.sst])
	}

	best, bestCells := -1, 0
	grids := make([][][]string, 0, len(globals.sheets))
	for _, sh := range globals.sheets {
		grid := decodeSheet(records, stream, sh.offset, sst)
		grids = append(grids, grid)

		info := SheetInfo{Name: sh.name, Rows: len(grid), Hidden: sh.hidden}
		for _, row := range grid {
			if len(row) > info.Width {
				info.Width = len(row)
			}
			for _, cell := range row {
				if CleanCell(cell) != "" {
					info.Cells++
				}
			}
		}
		b.source.Sheets = append(b.source.Sheets, info)
		if !info.Hidden && info.Cells > bestCells {
			bestCells, best = info.Cells, len(b.source.Sheets)-1
		}
	}
	if best < 0 || bestCells == 0 {
		return errors.New("native XLS: no data cells")
	}

	b.source.Sheets[best].Chosen = true
	b.source.Sheet = b.source.Sheets[best].Name
	b.rows = grids[best]
	b.finishGrid()
	return nil
}

// workbookStream returns the BIFF record stream, under either name Excel has
// used for it.
func workbookStream(c *compoundFile) ([]byte, error) {
	stream, err := c.stream("Workbook")
	if err == nil {
		return stream, nil
	}
	return c.stream("Book")
}

// splitBiffRecords walks the stream into records, attaching each CONTINUE to
// the record it continues.
//
// A record whose declared length runs past the end of the stream ends the walk
// rather than failing it. Real workbooks carry trailing structures this decoder
// does not read — the drawing layer, in the corpus file — and stopping there
// with every cell already collected is the correct outcome, where refusing the
// file would throw away the data over bytes nothing needed.
func splitBiffRecords(stream []byte) ([]biffRecord, error) {
	var out []biffRecord
	for i := 0; i+4 <= len(stream); {
		id := binary.LittleEndian.Uint16(stream[i : i+2])
		size := int(binary.LittleEndian.Uint16(stream[i+2 : i+4]))
		if i+4+size > len(stream) {
			break
		}
		body := stream[i+4 : i+4+size]
		if id == recContinue && len(out) > 0 {
			last := &out[len(out)-1]
			last.continues = append(last.continues, body)
		} else {
			out = append(out, biffRecord{id: id, body: body, offset: i})
		}
		i += 4 + size
	}
	if len(out) == 0 {
		return nil, errors.New("native XLS: no readable records")
	}
	return out, nil
}

// worksheetRef is one worksheet named by the workbook globals.
type worksheetRef struct {
	name   string
	offset int
	hidden bool
}

// workbookGlobals is what the first substream declares: where the worksheets
// are and where the shared strings live.
type workbookGlobals struct {
	sheets []worksheetRef
	// sst is the index of the shared-string record, or -1.
	sst int
}

// globalsOf reads the workbook globals substream.
func globalsOf(records []biffRecord) (workbookGlobals, error) {
	g := workbookGlobals{sst: -1}
	if records[0].id != recBOF || len(records[0].body) < 2 ||
		binary.LittleEndian.Uint16(records[0].body[0:2]) < biffVersion8 {
		return g, errors.New("native XLS: not a BIFF8 workbook")
	}
	for i, rec := range records {
		switch rec.id {
		case recFilePass:
			return g, errors.New("native XLS: workbook is password protected")
		case recSST:
			g.sst = i
		case recBoundSheet:
			if sh, ok := readBoundSheet(rec.body); ok {
				g.sheets = append(g.sheets, sh)
			}
		case recEOF:
			if len(g.sheets) > 0 {
				return g, nil
			}
		}
	}
	return g, nil
}

// readBoundSheet reads one worksheet's name, visibility and stream position.
func readBoundSheet(body []byte) (worksheetRef, bool) {
	const worksheetType = 0
	if len(body) < 8 {
		return worksheetRef{}, false
	}
	if body[5] != worksheetType {
		return worksheetRef{}, false // a chart or a macro sheet
	}
	return worksheetRef{
		name:   readShortUnicode(body[6:]),
		offset: int(binary.LittleEndian.Uint32(body[0:4])),
		hidden: body[4]&0x03 != 0,
	}, true
}

// readShortUnicode reads the one-byte-length string BOUNDSHEET carries.
func readShortUnicode(b []byte) string {
	if len(b) < 2 {
		return ""
	}
	return decodeChars(b[2:], int(b[0]), b[1]&sstHighByte != 0)
}

// readLongUnicode reads the two-byte-length string a cell record carries: a
// character count, an options byte, an optional run count, then the characters.
//
// Rich text is read for its characters and nothing else — the formatting runs
// that follow describe how Excel painted them, and this importer is reading a
// price list, not rendering one.
func readLongUnicode(b []byte) string {
	if len(b) < 3 {
		return ""
	}
	cch := int(binary.LittleEndian.Uint16(b[0:2]))
	flags := b[2]
	chars := 3
	if flags&sstRichSt != 0 {
		chars += 2
	}
	if flags&sstExtSt != 0 {
		chars += 4
	}
	if chars > len(b) {
		return ""
	}
	return decodeChars(b[chars:], cch, flags&sstHighByte != 0)
}

// decodeChars reads cch characters, from the first character byte, in either of
// BIFF8's two encodings.
func decodeChars(b []byte, cch int, high bool) string {
	if cch <= 0 {
		return ""
	}
	if high {
		n := min(cch*2, len(b)&^1)
		runes := make([]rune, 0, n/2)
		for i := 0; i+1 < n; i += 2 {
			runes = append(runes, rune(binary.LittleEndian.Uint16(b[i:i+2])))
		}
		return string(runes)
	}
	n := min(cch, len(b))
	runes := make([]rune, 0, n)
	for i := 0; i < n; i++ {
		runes = append(runes, rune(b[i]))
	}
	return string(runes)
}

// decodeSheet reads one worksheet substream into a dense grid.
func decodeSheet(records []biffRecord, stream []byte, offset int, sst []string) [][]string {
	start := indexOfOffset(records, offset)
	if start < 0 {
		return nil
	}
	cells := map[int]map[int]string{}
	maxRow, maxCol := -1, -1
	put := func(row, col int, value string) {
		if row < 0 || col < 0 || row >= maxBiffRows || col >= maxBiffCols {
			return
		}
		line, ok := cells[row]
		if !ok {
			line = map[int]string{}
			cells[row] = line
		}
		line[col] = value
		maxRow = max(maxRow, row)
		maxCol = max(maxCol, col)
	}

	// A formula's string result arrives in the record after it.
	pendingRow, pendingCol := -1, -1

	for i := start + 1; i < len(records); i++ {
		rec := records[i]
		if rec.id == recBOF {
			break // the next substream
		}
		if rec.id == recEOF {
			break
		}
		body := rec.body
		switch rec.id {
		case recLabelSST:
			if len(body) >= 10 {
				idx := int(binary.LittleEndian.Uint32(body[6:10]))
				if idx >= 0 && idx < len(sst) {
					put(cellRow(body), cellCol(body), sst[idx])
				}
			}
		case recLabel, recRString:
			if len(body) >= 9 {
				put(cellRow(body), cellCol(body), readLongUnicode(body[6:]))
			}
		case recNumber:
			if len(body) >= 14 {
				put(cellRow(body), cellCol(body),
					formatBiffNumber(math.Float64frombits(binary.LittleEndian.Uint64(body[6:14]))))
			}
		case recRK:
			if len(body) >= 10 {
				put(cellRow(body), cellCol(body),
					formatBiffNumber(decodeRK(binary.LittleEndian.Uint32(body[6:10]))))
			}
		case recMulRK:
			readMulRK(body, put)
		case recBoolErr:
			if len(body) >= 8 && body[7] == 0 {
				put(cellRow(body), cellCol(body), strconv.FormatBool(body[6] != 0))
			}
		case recFormula:
			if len(body) >= 14 {
				row, col := cellRow(body), cellCol(body)
				if isStringFormula(body[6:14]) {
					pendingRow, pendingCol = row, col
					continue
				}
				if !isNonNumericFormula(body[6:14]) {
					put(row, col, formatBiffNumber(
						math.Float64frombits(binary.LittleEndian.Uint64(body[6:14]))))
				}
			}
		case recString:
			if pendingRow >= 0 {
				put(pendingRow, pendingCol, readLongUnicode(body))
			}
		}
		if rec.id != recFormula {
			pendingRow, pendingCol = -1, -1
		}
	}

	if maxRow < 0 {
		return nil
	}
	grid := make([][]string, maxRow+1)
	for r := range grid {
		line := cells[r]
		if len(line) == 0 {
			continue
		}
		row := make([]string, maxCol+1)
		for c, v := range line {
			row[c] = v
		}
		grid[r] = row
	}
	return grid
}

// indexOfOffset finds the record that begins at a byte offset in the stream.
func indexOfOffset(records []biffRecord, offset int) int {
	for i, rec := range records {
		if rec.offset == offset {
			return i
		}
	}
	return -1
}

func cellRow(body []byte) int { return int(binary.LittleEndian.Uint16(body[0:2])) }
func cellCol(body []byte) int { return int(binary.LittleEndian.Uint16(body[2:4])) }

// readMulRK expands the run of RK values one record can carry.
func readMulRK(body []byte, put func(row, col int, value string)) {
	if len(body) < 6 {
		return
	}
	row := cellRow(body)
	col := cellCol(body)
	for i := 4; i+6 <= len(body)-2; i += 6 {
		put(row, col, formatBiffNumber(decodeRK(binary.LittleEndian.Uint32(body[i+2:i+6]))))
		col++
	}
}

// decodeRK expands Excel's packed 30-bit number.
func decodeRK(rk uint32) float64 {
	var value float64
	if rk&0x02 != 0 {
		value = float64(int32(rk) >> 2)
	} else {
		value = math.Float64frombits(uint64(rk&0xFFFFFFFC) << 32)
	}
	if rk&0x01 != 0 {
		value /= 100
	}
	return value
}

// isStringFormula and isNonNumericFormula read the eight result bytes a FORMULA
// record carries. A result whose last two bytes are 0xFFFF is not a float: the
// first byte says which of string, boolean, error or blank it is.
func isStringFormula(result []byte) bool {
	return len(result) == 8 && result[6] == 0xFF && result[7] == 0xFF && result[0] == 0
}

func isNonNumericFormula(result []byte) bool {
	return len(result) == 8 && result[6] == 0xFF && result[7] == 0xFF
}

// formatBiffNumber renders a cell's number the way the rest of this package
// expects to receive it: the shortest exact decimal, with no thousands
// separator and no currency.
func formatBiffNumber(f float64) string {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return ""
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}
