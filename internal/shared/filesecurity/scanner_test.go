package filesecurity_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"

	"github.com/muhiya/dawa24-store/internal/shared/filesecurity"
)

func TestCSV_RejectsHTTPAndDomains(t *testing.T) {
	// 1. Clean CSV
	cleanCSV := "name,price,qty\nPanadol Extra 500mg,45.00,10\nAugmentin 1g,115.00,5\n"
	err := filesecurity.ValidateSpreadsheetSecurity([]byte(cleanCSV), "products.csv")
	assert.NoError(t, err)

	// 2. HTTP URL
	httpCSV := "name,price,qty\nPanadol,45.00,10\nMalicious,10.00,http://evil.com\n"
	err = filesecurity.ValidateSpreadsheetSecurity([]byte(httpCSV), "products.csv")
	assert.ErrorIs(t, err, filesecurity.ErrSecurityBlocked)
	assert.Equal(t, filesecurity.SecurityErrorMessage, err.Error())

	// 3. WWW domain
	wwwCSV := "name,price,qty\nPanadol,45.00,10\nMalicious,10.00,www.badsite.com\n"
	err = filesecurity.ValidateSpreadsheetSecurity([]byte(wwwCSV), "products.csv")
	assert.ErrorIs(t, err, filesecurity.ErrSecurityBlocked)

	// 4. Raw domain
	domainCSV := "name,price,qty\nPanadol,45.00,10\nMalicious,10.00,attacker.xyz\n"
	err = filesecurity.ValidateSpreadsheetSecurity([]byte(domainCSV), "products.csv")
	assert.ErrorIs(t, err, filesecurity.ErrSecurityBlocked)

	// 5. IP address
	ipCSV := "name,price,qty\nPanadol,45.00,10\nMalicious,10.00,192.168.1.1\n"
	err = filesecurity.ValidateSpreadsheetSecurity([]byte(ipCSV), "products.csv")
	assert.ErrorIs(t, err, filesecurity.ErrSecurityBlocked)
}

func TestXLSX_RejectsHTTPAndDomains(t *testing.T) {
	// 1. Clean XLSX
	f := excelize.NewFile()
	f.SetCellValue("Sheet1", "A1", "اسم الدواء")
	f.SetCellValue("Sheet1", "B1", "السعر")
	f.SetCellValue("Sheet1", "A2", "بانادول إكسترا 500 مجم أقراص")
	f.SetCellValue("Sheet1", "B2", 55.00)
	var buf bytes.Buffer
	require.NoError(t, f.Write(&buf))

	err := filesecurity.ValidateSpreadsheetSecurity(buf.Bytes(), "clean.xlsx")
	assert.NoError(t, err)

	// 2. Malicious XLSX with URL in cell
	fBad := excelize.NewFile()
	fBad.SetCellValue("Sheet1", "A1", "اسم الدواء")
	fBad.SetCellValue("Sheet1", "B1", "الرابط")
	fBad.SetCellValue("Sheet1", "A2", "دواء تجريبي")
	fBad.SetCellValue("Sheet1", "B2", "https://phishing.site/steal")
	var bufBad bytes.Buffer
	require.NoError(t, fBad.Write(&bufBad))

	err = filesecurity.ValidateSpreadsheetSecurity(bufBad.Bytes(), "bad.xlsx")
	assert.ErrorIs(t, err, filesecurity.ErrSecurityBlocked)
	assert.Equal(t, "فشل الرفع لأسباب امنية", err.Error())

	// 3. Malicious XLSX with External Hyperlink
	fLink := excelize.NewFile()
	fLink.SetCellValue("Sheet1", "A1", "اضغط هنا")
	require.NoError(t, fLink.SetCellHyperLink("Sheet1", "A1", "http://c2.attacker.com", "External"))
	var bufLink bytes.Buffer
	require.NoError(t, fLink.Write(&bufLink))

	err = filesecurity.ValidateSpreadsheetSecurity(bufLink.Bytes(), "link.xlsx")
	assert.ErrorIs(t, err, filesecurity.ErrSecurityBlocked)
}

func TestAllowEmailsOption_ForTeamImport(t *testing.T) {
	teamCSV := "name,email,phone\nAhmed Ali,ahmed@example.com,01012345678\n"
	
	// Default mode rejects email as containing domain
	err := filesecurity.ValidateSpreadsheetSecurity([]byte(teamCSV), "team.csv")
	assert.ErrorIs(t, err, filesecurity.ErrSecurityBlocked)

	// In team mode (WithAllowEmails), email is permitted
	err = filesecurity.ValidateSpreadsheetSecurity([]byte(teamCSV), "team.csv", filesecurity.WithAllowEmails(true))
	assert.NoError(t, err)

	// Even in team mode, HTTP URLs and standalone domains are strictly blocked
	teamBadCSV := "name,email,phone\nAhmed Ali,http://evil.com,01012345678\n"
	err = filesecurity.ValidateSpreadsheetSecurity([]byte(teamBadCSV), "team.csv", filesecurity.WithAllowEmails(true))
	assert.ErrorIs(t, err, filesecurity.ErrSecurityBlocked)
}
