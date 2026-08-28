package pages

import (
	"github.com/muhiya/dawa24-store/internal/modules/attachments"
)

// docTitlesAr is the UI-facing Arabic title per document type.
var docTitlesAr = map[attachments.DocumentType]string{
	attachments.DocCommercialRegister:  "السجل التجاري (Commercial Register)",
	attachments.DocTaxCard:             "البطاقة الضريبية (Tax Card)",
	attachments.DocPharmacyLicense:     "ترخيص المنشأة الصيدلية (Facility License)",
	attachments.DocPharmacistLicense:   "ترخيص مزاولة المهنة للصيدلي (Pharmacist License)",
	attachments.DocNationalID:          "بطاقة الرقم القومي (National ID)",
	attachments.DocAuthorizationLetter: "خطاب التفويض الرسمي (Authorization Letter)",
	attachments.DocBankCertificate:     "شهادة الحساب البنكي (Bank Certificate)",
	attachments.DocOther:               "مستند رسمي إضافي (Other Document)",
}

// DocRequirement is one entry of the document requirement table.
type DocRequirement struct {
	DocType     attachments.DocumentType
	TitleAr     string
	Description string
	Required    bool
}

// docRequirements returns the audience requirement set with UI titles.
func docRequirements(vendor bool) []DocRequirement {
	if vendor {
		return []DocRequirement{
			{
				DocType:     attachments.DocCommercialRegister,
				TitleAr:     "السجل التجاري ساري المفعول",
				Description: "شهادة القيد بالسجل التجاري للمنشأة لم يمر عليها أكثر من 3 أشهر.",
				Required:    true,
			},
			{
				DocType:     attachments.DocTaxCard,
				TitleAr:     "البطاقة الضريبية",
				Description: "البطاقة الضريبية سارية ومسجلة باسم المنشأة.",
				Required:    true,
			},
			{
				DocType:     attachments.DocPharmacyLicense,
				TitleAr:     "ترخيص هيئة الدواء المصرية / ترخيص المخزن",
				Description: "ترخيص مخزن الأدوية أو المنشأة الصادرة من هيئة الدواء المصرية.",
				Required:    true,
			},
			{
				DocType:     attachments.DocAuthorizationLetter,
				TitleAr:     "خطاب تفويض المدير المسؤول",
				Description: "تفويض رسمي موثق للشخص المفوض بإدارة الحساب وإبرام الصفقات.",
				Required:    false,
			},
		}
	}

	return []DocRequirement{
		{
			DocType:     attachments.DocPharmacyLicense,
			TitleAr:     "ترخيص المنشأة الصيدلية الرسمية",
			Description: "ترخيص فتح الصيدلية الصادر من وزارة الصحة أو هيئة الدواء المصرية.",
			Required:    true,
		},
		{
			DocType:     attachments.DocPharmacistLicense,
			TitleAr:     "ترخيص مزاولة المهنة للصيدلي المدير",
			Description: "ترخيص مزاولة مهنة الصيدلة للصيدلي المسؤول أو المدير الفني للصيدلية.",
			Required:    true,
		},
		{
			DocType:     attachments.DocCommercialRegister,
			TitleAr:     "السجل التجاري (إن وجد)",
			Description: "مستخرج حديث من السجل التجاري للصيدلية أو المجموعة.",
			Required:    false,
		},
		{
			DocType:     attachments.DocTaxCard,
			TitleAr:     "البطاقة الضريبية",
			Description: "صورة واضحة من البطاقة الضريبية سارية الصلاحية.",
			Required:    false,
		},
		{
			DocType:     attachments.DocAuthorizationLetter,
			TitleAr:     "خطاب تفويض / توكيل الإدارة",
			Description: "تفويض رسمي موثق في حال كان المفوض غير الصيدلي المالك أو المدير.",
			Required:    false,
		},
	}
}

// OrganizationDocumentsData backs the shared customer/vendor documents screen.
type OrganizationDocumentsData struct {
	IsVendor     bool
	Requirements []DocRequirement
	Docs         []*attachments.Document
	Requests     []*attachments.DocumentRequest
	Missing      []DocRequirement
	Error        string
}

// BuildOrganizationDocumentsData groups the org's documents by requirement
// and computes which mandatory ones are missing.
func BuildOrganizationDocumentsData(docs []*attachments.Document, requests []*attachments.DocumentRequest, vendor bool) *OrganizationDocumentsData {
	data := &OrganizationDocumentsData{
		IsVendor: vendor,
		Docs:     docs,
		Requests: requests,
	}
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
		if doc == nil || doc.DocumentType != t || doc.DeletedAt != nil {
			continue
		}
		if latest == nil || doc.CreatedAt.After(latest.CreatedAt) {
			latest = doc
		}
	}
	return latest
}

// TotalRequirementsCount returns total requirements.
func (d *OrganizationDocumentsData) TotalRequirementsCount() int {
	return len(d.Requirements)
}

// VerifiedCount counts requirements that have an active verified document.
func (d *OrganizationDocumentsData) VerifiedCount() int {
	count := 0
	for _, req := range d.Requirements {
		if doc := d.LatestFor(req.DocType); doc != nil && doc.Status == attachments.StatusVerified {
			count++
		}
	}
	return count
}

// PendingCount counts requirements currently under administrative review.
func (d *OrganizationDocumentsData) PendingCount() int {
	count := 0
	for _, req := range d.Requirements {
		if doc := d.LatestFor(req.DocType); doc != nil && doc.Status == attachments.StatusPending {
			count++
		}
	}
	return count
}

// RejectedCount counts requirements that were rejected.
func (d *OrganizationDocumentsData) RejectedCount() int {
	count := 0
	for _, req := range d.Requirements {
		if doc := d.LatestFor(req.DocType); doc != nil && doc.Status == attachments.StatusRejected {
			count++
		}
	}
	return count
}

// CompletionPercentage returns integer percentage of verified documents against total requirements.
func (d *OrganizationDocumentsData) CompletionPercentage() int {
	if len(d.Requirements) == 0 {
		return 100
	}
	v := d.VerifiedCount()
	return (v * 100) / len(d.Requirements)
}

// ActiveDocRequests returns active (pending or submitted) requests from platform administration.
func (d *OrganizationDocumentsData) ActiveDocRequests() []*attachments.DocumentRequest {
	var active []*attachments.DocumentRequest
	for _, r := range d.Requests {
		if r != nil && (r.Status == attachments.DocReqPending || r.Status == attachments.DocReqSubmitted) {
			active = append(active, r)
		}
	}
	return active
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
