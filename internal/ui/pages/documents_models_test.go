package pages

import (
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/attachments"
)

func doc(id int64, t attachments.DocumentType, status attachments.DocumentStatus, at time.Time) *attachments.Document {
	return &attachments.Document{
		ID:           id,
		DocumentType: t,
		Status:       status,
		CreatedAt:    at,
	}
}

func TestBuildOrganizationDocumentsData_Missing(t *testing.T) {
	customer := BuildOrganizationDocumentsData(nil, false)
	if len(customer.Missing) != 4 {
		t.Fatalf("customer with no docs: want 4 missing, got %d", len(customer.Missing))
	}

	vendor := BuildOrganizationDocumentsData(nil, true)
	if len(vendor.Missing) != 3 {
		t.Fatalf("vendor with no docs: want 3 missing, got %d", len(vendor.Missing))
	}
	if vendor.Requirements[0].DocType != attachments.DocCommercialRegister || !vendor.Requirements[0].Required {
		t.Fatal("vendor commercial register must be the first required requirement")
	}
	if vendor.Requirements[3].DocType != attachments.DocAuthorizationLetter || vendor.Requirements[3].Required {
		t.Fatal("authorization letter must be the trailing optional requirement")
	}
}

func TestBuildOrganizationDocumentsData_VerifiedClearsMissing(t *testing.T) {
	now := time.Now()
	docs := []*attachments.Document{
		doc(1, attachments.DocCommercialRegister, attachments.StatusVerified, now),
		doc(2, attachments.DocTaxCard, attachments.StatusPending, now),
		doc(3, attachments.DocPharmacyLicense, attachments.StatusVerified, now),
	}
	customer := BuildOrganizationDocumentsData(docs, false)
	if len(customer.Missing) != 2 {
		t.Fatalf("want 2 missing (tax card pending + pharmacist card), got %d", len(customer.Missing))
	}
	for _, m := range customer.Missing {
		if m.DocType == attachments.DocCommercialRegister || m.DocType == attachments.DocPharmacyLicense {
			t.Fatalf("verified document %s must not be missing", m.DocType)
		}
	}

	docs = append(docs, doc(4, attachments.DocPharmacistLicense, attachments.StatusRejected, now))
	customer = BuildOrganizationDocumentsData(docs, false)
	if len(customer.Missing) != 2 {
		t.Fatalf("rejected pharmacist card still missing: got %d missing", len(customer.Missing))
	}
	if got := customer.MissingTitles(); got == "" {
		t.Fatal("MissingTitles must not be empty with a missing requirement")
	}

	docs = append(docs, doc(5, attachments.DocPharmacistLicense, attachments.StatusVerified, now.Add(time.Minute)))
	customer = BuildOrganizationDocumentsData(docs, false)
	if len(customer.Missing) != 1 {
		t.Fatalf("verified pharmacist card leaves only the pending tax card: got %d missing", len(customer.Missing))
	}
	if customer.Missing[0].DocType != attachments.DocTaxCard {
		t.Fatalf("want missing tax_card, got %s", customer.Missing[0].DocType)
	}
}

func TestOrganizationDocumentsData_LatestFor(t *testing.T) {
	now := time.Now()
	data := &OrganizationDocumentsData{Docs: []*attachments.Document{
		doc(1, attachments.DocCommercialRegister, attachments.StatusPending, now),
		doc(2, attachments.DocCommercialRegister, attachments.StatusVerified, now.Add(2*time.Minute)),
	}}
	if latest := data.LatestFor(attachments.DocCommercialRegister); latest == nil || latest.ID != 2 {
		t.Fatalf("want newest document (id 2), got %+v", latest)
	}
	if latest := data.LatestFor(attachments.DocTaxCard); latest != nil {
		t.Fatalf("want nil for missing type, got %+v", latest)
	}
}
