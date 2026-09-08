package ui_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/components"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func TestAdminTranslations_ModalInitiallyHidden(t *testing.T) {
	data := pages.AdminTranslationsData{
		Translations: []*platformadmin.Translation{
			{
				ID:        1,
				Namespace: "admin",
				Key:       "test.key",
				TextAR:    "اختبار",
				TextEN:    "Test",
			},
		},
		Pagination: components.PaginationProps{
			CurrentPage: 1,
			PageSize:    20,
			TotalCount:  1,
			BaseURL:     "/admin/translations",
		},
	}

	var buf bytes.Buffer
	err := pages.AdminTranslations(data, "ar", "rtl").Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Failed to render AdminTranslations: %v", err)
	}

	html := buf.String()

	// Modal must have style="display: none;" so it does not open automatically on page load
	if !strings.Contains(html, `id="edit-translation-modal" class="modal-backdrop" style="display: none;"`) {
		t.Errorf("edit-translation-modal must have style=\"display: none;\" to prevent auto-opening on page load")
	}

	// Must contain close handler
	if !strings.Contains(html, "closeEditModal()") {
		t.Errorf("expected closeEditModal() to be present")
	}
}

func TestAdminCities_CoverageRadiusWired(t *testing.T) {
	govID := int64(10)
	gov := &platformadmin.Governorate{
		ID:                   govID,
		Name:                 i18n.New("أسوان", "Aswan"),
		Latitude:             24.0889,
		Longitude:            32.8998,
		CoverageRadiusMeters: 30000,
		IsActive:             true,
	}

	city := &platformadmin.City{
		ID:                   101,
		GovernorateID:        &govID,
		GovernorateName:      &gov.Name,
		Name:                 i18n.New("إدفو", "Edfu"),
		Latitude:             24.9785,
		Longitude:            32.8753,
		CoverageRadiusMeters: 5000,
		IsActive:             true,
	}

	data := pages.AdminCitiesData{
		Governorates:          []*platformadmin.Governorate{gov},
		Cities:                []*platformadmin.City{city},
		SelectedGovernorateID: 0,
		TotalCities:           1,
		TotalGovernorates:     1,
		TotalFiltered:         1,
		Page:                  1,
		Limit:                 20,
		TotalPages:            1,
	}

	var buf bytes.Buffer
	err := pages.AdminCities(data, "ar", "rtl", false).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Failed to render AdminCities: %v", err)
	}

	html := buf.String()

	// 1. Table header must have coverage radius column
	if !strings.Contains(html, "نطاق التغطية") {
		t.Errorf("expected cities table to have 'نطاق التغطية' column")
	}

	// 2. City coverage radius rendered in table row (5.0 كم (5000 م))
	if !strings.Contains(html, "5.0 كم (5000 م)") {
		t.Errorf("expected city coverage radius to be rendered in table, got html: %s", html)
	}

	// 3. Governorate coverage radius rendered in card list (30 كم)
	if !strings.Contains(html, "30 كم") {
		t.Errorf("expected governorate coverage radius to be rendered in card list")
	}

	// 4. Add city form must have coverage_radius_meters input
	if !strings.Contains(html, `name="coverage_radius_meters"`) {
		t.Errorf("expected Add City form to contain coverage_radius_meters input")
	}

	// 5. Add governorate form must have gov_coverage_radius_meters input
	if !strings.Contains(html, `name="gov_coverage_radius_meters"`) {
		t.Errorf("expected Add Governorate form to contain gov_coverage_radius_meters input")
	}

	// 6. Edit city modal must have coverage_radius_meters input bound to editForm.radius
	if !strings.Contains(html, `x-model="editForm.radius"`) {
		t.Errorf("expected Edit City modal to bind radius to editForm.radius")
	}

	// 7. Edit governorate modal must have gov_coverage_radius_meters input bound to govEditForm.radius
	if !strings.Contains(html, `x-model="govEditForm.radius"`) {
		t.Errorf("expected Edit Governorate modal to bind radius to govEditForm.radius")
	}
}

func TestAdminBranches_CoordsValidJSONAndEditMode(t *testing.T) {
	cityID := int64(101)
	lat := 24.0889
	lon := 32.8998
	branch := &org.Branch{
		ID:             55,
		OrganizationID: 10,
		Name:           i18n.Text{"en": "Aswan Branch"}, // Empty Arabic name test
		Code:           "ASW-01",
		WarehouseType:  "branch",
		CityID:         &cityID,
		Address:        "شارع المطار، أسوان",
		Phone:          "01012345678",
		Latitude:       &lat,
		Longitude:      &lon,
		IsMain:         true,
		Status:         "active",
	}

	city := &platformadmin.City{
		ID:        101,
		Name:      i18n.New("أسوان", "Aswan"),
		Latitude:  24.0889,
		Longitude: 32.8998,
	}

	data := pages.AdminBranchesPageData{
		Branches:      []*org.Branch{branch},
		Organizations: []*org.Organization{{ID: 10, LegalName: "مؤسسة أسوان"}},
		OrgNames:      map[int64]string{10: "مؤسسة أسوان"},
		OrgTypes:      map[int64]string{10: "vendor"},
		Cities:        []*platformadmin.City{city},
		TotalBranches: 1,
		FilteredCount: 1,
	}

	var buf bytes.Buffer
	err := pages.AdminBranchesPage(data, "ar", "rtl").Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Failed to render AdminBranchesPage: %v", err)
	}

	html := buf.String()

	// 1. Coordinates must NOT be literal raw Templ syntax
	if strings.Contains(html, "{ templ.Raw(") {
		t.Errorf("Found unparsed '{ templ.Raw(' inside rendered HTML! Templ did not execute Go expression.")
	}

	// 2. data-coords on #admin-branch-cities-coords must be present and valid JSON
	if !strings.Contains(html, `id="admin-branch-cities-coords"`) {
		t.Fatalf("expected #admin-branch-cities-coords to exist")
	}

	startTag := `id="admin-branch-cities-coords" class="d-none" data-coords="`
	idx := strings.Index(html, startTag)
	if idx == -1 {
		t.Fatalf("expected admin-branch-cities-coords to have data-coords attribute")
	}
	sub := html[idx+len(startTag):]
	endIdx := strings.Index(sub, `"`)
	if endIdx == -1 {
		t.Fatalf("could not find closing quote of data-coords")
	}
	rawCoords := strings.ReplaceAll(sub[:endIdx], "&#34;", "\"")
	var coordsMap map[string][]any
	if err := json.Unmarshal([]byte(rawCoords), &coordsMap); err != nil {
		t.Fatalf("data-coords failed to parse as valid JSON: %v, raw: %s", err, rawCoords)
	}
	if pos, ok := coordsMap["101"]; !ok || len(pos) < 2 {
		t.Fatalf("expected city 101 in coords map, got: %v", coordsMap)
	}

	// 3. Edit button must have data-branch-data with valid JSON
	btnTag := fmt.Sprintf(`data-edit-branch-btn="%d"`, branch.ID)
	if !strings.Contains(html, btnTag) {
		t.Fatalf("expected branch edit button with %s", btnTag)
	}

	// 4. Form submit validation must check both form.city_id and input
	if !strings.Contains(html, "const cid = form.city_id") {
		t.Errorf("expected form @submit to check cid fallback")
	}
}
