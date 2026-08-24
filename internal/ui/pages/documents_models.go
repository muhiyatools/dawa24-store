package pages

import (
	"github.com/muhiya/dawa24-store/internal/modules/attachments"
)

// docTitlesAr is the UI-facing Arabic title per document type. The type set
// itself lives in the attachments module (RequirementsFor); only presentation
// lives here.
var docTitlesAr = map[attachments.DocumentType]string{
	attachments.DocCommercialRegister:  "السجل التجاري",
	attachments.DocTaxCard:             "البطاقة الضريبية",
	attachments.DocPharmacyLicense:     "ترخيص الصيدلية",
	attachments.DocPharmacistLicense:   "بطاقة الصيدلي",
	attachments.DocAuthorizationLetter: "خطاب تفويض",
}

// DocRequirement is one entry of the mandatory-document table (Rebuild V2
// §4.2): a document type, its Arabic title, and whether the audience must
// hold it to trade.
type DocRequirement struct {
	DocType  attachments.DocumentType
	TitleAr  string
	Required bool
}

// docRequirements returns the audience requirement set with UI titles.
func docRequirements(vendor bool) []DocRequirement {
	orgType := "customer"
	if vendor {
		orgType = "vendor"
	}
	reqs := attachments.RequirementsFor(orgType)
	out := make([]DocRequirement, 0, len(reqs))
	for _, r := range reqs {
		out = append(out, DocRequirement{
			DocType:  r.DocType,
			TitleAr:  docTitlesAr[r.DocType],
			Required: r.Required,
		})
	}
	return out
}

// OrganizationDocumentsData backs the shared customer/vendor documents screen.
type OrganizationDocumentsData struct {
	Requirements []DocRequirement
	Docs         []*attachments.Document
	Requests     []*attachments.DocumentRequest
	Missing      []DocRequirement
	Error        string
}

// BuildOrganizationDocumentsData groups the org's documents by requirement
// and computes which mandatory ones are missing. The latest document of each
// type is what the screen acts on (replace/delete).
func BuildOrganizationDocumentsData(docs []*attachments.Document, requests []*attachments.DocumentRequest, vendor bool) *OrganizationDocumentsData {
	data := &OrganizationDocumentsData{Docs: docs, Requests: requests}
	data.Requirements = docRequirements(vendor)

	hasVerified := false
	for _, d := range docs {
		if d != nil && d.Status == attachments.StatusVerified && d.DeletedAt == nil {
			hasVerified = true
			break
		}
	}

	if !hasVerified {
		data.Missing = []DocRequirement{
			{TitleAr: "مستند رسمي معتمد واحد على الأقل (السجل التجاري، ترخيص الصيدلية، أو البطاقة الضريبية)"},
		}
	}
	return data
}

// LatestFor returns the newest document of the given type, or nil.
func (d *OrganizationDocumentsData) LatestFor(t attachments.DocumentType) *attachments.Document {
	var latest *attachments.Document
	for _, doc := range d.Docs {
		if doc == nil || doc.DocumentType != t {
			continue
		}
		if latest == nil || doc.CreatedAt.After(latest.CreatedAt) {
			latest = doc
		}
	}
	return latest
}

// MissingTitles joins the missing requirements for the banner message.
func (d *OrganizationDocumentsData) MissingTitles() string {
	if len(d.Missing) == 0 {
		return ""
	}
	joined := ""
	for i, req := range d.Missing {
		if i > 0 {
			joined += "، "
		}
		joined += req.TitleAr
	}
	return joined
}
