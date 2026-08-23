package spreadsheet_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"

	"github.com/muhiya/dawa24-store/internal/shared/spreadsheet"
)

func TestReadRows_XLSX(t *testing.T) {
	f := excelize.NewFile()
	sheet := "Sheet1"
	_ = f.SetCellValue(sheet, "A1", "الباركود")
	_ = f.SetCellValue(sheet, "B1", "اسم الصنف")
	_ = f.SetCellValue(sheet, "C1", "السعر")
	_ = f.SetCellValue(sheet, "A2", "6221142001234")
	_ = f.SetCellValue(sheet, "B2", "بانادول إكسترا")
	_ = f.SetCellValue(sheet, "C2", "48.50")

	var buf bytes.Buffer
	require.NoError(t, f.Write(&buf))

	rows, err := spreadsheet.ReadRows(buf.Bytes())
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, "الباركود", rows[0][0])
	assert.Equal(t, "بانادول إكسترا", rows[1][1])
}

func TestReadRows_CSV(t *testing.T) {
	csvData := []byte("الباركود,اسم الصنف,السعر\n6221142001234,بانادول إكسترا,48.50\n")
	rows, err := spreadsheet.ReadRows(csvData)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, "الباركود", rows[0][0])
	assert.Equal(t, "بانادول إكسترا", rows[1][1])
}

func TestReadRows_HTMLTable(t *testing.T) {
	htmlData := []byte(`
		<!DOCTYPE html>
		<html>
		<body>
			<table>
				<tr><th>الباركود</th><th>اسم الصنف</th><th>السعر</th></tr>
				<tr><td>6221142001234</td><td>بانادول إكسترا</td><td>48.50</td></tr>
			</table>
		</body>
		</html>
	`)
	rows, err := spreadsheet.ReadRows(htmlData)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, "الباركود", rows[0][0])
	assert.Equal(t, "بانادول إكسترا", rows[1][1])
}

func TestReadHeadersAndPreview(t *testing.T) {
	csvData := []byte("كود,اسم,كمية,سعر\n1,صنف 1,10,20\n2,صنف 2,5,30\n3,صنف 3,1,50\n")
	headers, preview, err := spreadsheet.ReadHeadersAndPreview(csvData, 2)
	require.NoError(t, err)
	assert.Equal(t, []string{"كود", "اسم", "كمية", "سعر"}, headers)
	require.Len(t, preview, 2)
	assert.Equal(t, "صنف 1", preview[0][1])
	assert.Equal(t, "صنف 2", preview[1][1])
}
