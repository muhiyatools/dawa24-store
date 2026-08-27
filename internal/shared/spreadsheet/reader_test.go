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

func TestReadRows_XMLSpreadsheet2003(t *testing.T) {
	xmlData := []byte(`<?xml version="1.0"?>
<?mso-application progid="Excel.Sheet"?>
<Workbook xmlns="urn:schemas-microsoft-com:office:spreadsheet"
 xmlns:o="urn:schemas-microsoft-com:office:office"
 xmlns:x="urn:schemas-microsoft-com:office:excel"
 xmlns:ss="urn:schemas-microsoft-com:office:spreadsheet"
 xmlns:html="http://www.w3.org/TR/REC-html40">
 <Worksheet ss:Name="Sheet1">
  <Table>
   <Row>
    <Cell><Data ss:Type="String">الباركود</Data></Cell>
    <Cell><Data ss:Type="String">اسم الصنف</Data></Cell>
    <Cell><Data ss:Type="String">السعر</Data></Cell>
    <Cell><Data ss:Type="String">الخصم</Data></Cell>
   </Row>
   <Row>
    <Cell><Data ss:Type="String">6221142001234</Data></Cell>
    <Cell><Data ss:Type="String">بانادول إكسترا</Data></Cell>
    <Cell><Data ss:Type="Number">48.50</Data></Cell>
    <Cell><Data ss:Type="String">25%</Data></Cell>
   </Row>
  </Table>
 </Worksheet>
</Workbook>`)
	rows, err := spreadsheet.ReadRows(xmlData)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, "الباركود", rows[0][0])
	assert.Equal(t, "بانادول إكسترا", rows[1][1])
	assert.Equal(t, "48.50", rows[1][2])
	assert.Equal(t, "25%", rows[1][3])
}

func TestParseCleanDiscountAndPrice(t *testing.T) {
	assert.Equal(t, 25.0, spreadsheet.ParseCleanDiscount("25%"))
	assert.Equal(t, 25.5, spreadsheet.ParseCleanDiscount("25.5 %"))
	assert.Equal(t, 20.0, spreadsheet.ParseCleanDiscount("0.20"))
	assert.Equal(t, 18.25, spreadsheet.ParseCleanDiscount("18,25%"))
	assert.Equal(t, 0.0, spreadsheet.ParseCleanDiscount(""))
	assert.Equal(t, 100.0, spreadsheet.ParseCleanDiscount("150%"))

	p, err := spreadsheet.ParseCleanPrice("125.50 EGP")
	require.NoError(t, err)
	assert.Equal(t, 125.50, p)

	p2, err := spreadsheet.ParseCleanPrice("75,25 ج.م")
	require.NoError(t, err)
	assert.Equal(t, 75.25, p2)
}

func TestFindHeaderRowIndex(t *testing.T) {
	rows := [][]string{
		{"شركة التوزيع الحديثة للمستلزمات الطبية", "", ""},
		{"تقرير أسعار يوم 2026-08-27", "", ""},
		{"كود الصنف", "اسم الصنف الدوائي", "السعر الأساسي", "نسبة الخصم"},
		{"101", "أوجمنتين 1 جم أقراص", "99.50", "20%"},
	}
	idx := spreadsheet.FindHeaderRowIndex(rows)
	assert.Equal(t, 2, idx)
}
