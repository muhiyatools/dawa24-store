package org

import (
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// EmployeeInstitutionalWork links a user to an institutional work group.
type EmployeeInstitutionalWork struct {
	ID                  int64     `json:"id"`
	OrganizationID      int64     `json:"organization_id"`
	UserID              int64     `json:"user_id"`
	InstitutionalWorkID int64     `json:"institutional_work_id"`
	WorkTitle           string    `json:"work_title,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// InstitutionalFilterMode selects Laravel's two documented filter semantics.
type InstitutionalFilterMode int

const (
	// FilterSimple: intersect the user's works directly with the product's,
	// and additionally allow products with no institutional restriction (empty array).
	// Laravel: applyInstitutionalWorksFilter_Simple — customer dashboard.
	FilterSimple InstitutionalFilterMode = iota

	// FilterWithConnections: resolve connections first, then intersect.
	// Products with no restriction are NOT visible.
	// Laravel: applyInstitutionalWorksFilter_WithConnections — purchase-request pages.
	FilterWithConnections
)

// UserOrganizationStatus represents the approval state of a customer-to-vendor organization linkage.
type UserOrganizationStatus string

const (
	UserOrgStatusPending  UserOrganizationStatus = "pending"
	UserOrgStatusApproved UserOrganizationStatus = "approved"
	UserOrgStatusRejected UserOrganizationStatus = "rejected"
)

// UserOrganization links a customer user (pharmacy) with a vendor organization using their organization number.
type UserOrganization struct {
	ID                 int64                  `json:"id"`
	UserID             int64                  `json:"user_id"`
	UserName           string                 `json:"user_name,omitempty"`
	UserEmail          string                 `json:"user_email,omitempty"`
	CustomerOrgID      *int64                 `json:"customer_org_id,omitempty"`
	CustomerOrgName    string                 `json:"customer_org_name,omitempty"`
	VendorOrgID        int64                  `json:"vendor_org_id"`
	VendorOrgName      string                 `json:"vendor_org_name,omitempty"`
	VendorOrgType      string                 `json:"vendor_org_type,omitempty"`
	OrganizationNumber string                 `json:"organization_number"`
	Status             UserOrganizationStatus `json:"status"`
	Notes              string                 `json:"notes,omitempty"`
	CreatedAt          time.Time              `json:"created_at"`
	UpdatedAt          time.Time              `json:"updated_at"`
}

// ChangeRequestStatus defines the state of an organization change request.
type ChangeRequestStatus string

const (
	ChangeRequestPending   ChangeRequestStatus = "pending"
	ChangeRequestApproved  ChangeRequestStatus = "approved"
	ChangeRequestRejected  ChangeRequestStatus = "rejected"
	ChangeRequestCancelled ChangeRequestStatus = "cancelled"
)

// ProfileValues encapsulates commercial and profile snapshot fields for change requests.
type ProfileValues struct {
	NameAr             string       `json:"name_ar"`
	NameEn             string       `json:"name_en"`
	Type               string       `json:"type"` // "supplier", "company", "agency", "customer"
	MinOrderPrice      money.Amount `json:"min_order_price"`
	MaxOrderPrice      money.Amount `json:"max_order_price"`
	OrganizationNumber string       `json:"organization_number"`
	Email              string       `json:"email"`
	Phone              string       `json:"phone"`
	TaxNumber          string       `json:"tax_number"`
	Address            string       `json:"address"`
	DescriptionAr      string       `json:"description_ar"`
	DescriptionEn      string       `json:"description_en"`
	Image              string       `json:"image"`          // Logo URL
	CoverageImage      string       `json:"coverage_image"` // Cover URL
}

// OrganizationChangeRequest records a proposed modification to an organization's profile.
type OrganizationChangeRequest struct {
	ID              int64               `json:"id"`
	PublicID        string              `json:"public_id"`
	OrganizationID  int64               `json:"organization_id"`
	OrgName         string              `json:"org_name,omitempty"`
	OrgType         string              `json:"org_type,omitempty"`
	RequestedBy     *int64              `json:"requested_by,omitempty"`
	RequesterName   string              `json:"requester_name,omitempty"`
	RequesterEmail  string              `json:"requester_email,omitempty"`
	Status          ChangeRequestStatus `json:"status"`
	CurrentValues   ProfileValues       `json:"current_values"`
	ProposedValues  ProfileValues       `json:"proposed_values"`
	ReviewedBy      *int64              `json:"reviewed_by,omitempty"`
	ReviewerName    string              `json:"reviewer_name,omitempty"`
	ReviewedAt      *time.Time          `json:"reviewed_at,omitempty"`
	AdminNotes      string              `json:"admin_notes,omitempty"`
	RejectionReason string              `json:"rejection_reason,omitempty"`
	CreatedAt       time.Time           `json:"created_at"`
	UpdatedAt       time.Time           `json:"updated_at"`
}

// ChangedFieldItem represents one diff item between current and proposed values.
type ChangedFieldItem struct {
	FieldKey    string
	LabelAr     string
	LabelEn     string
	OldValue    string
	NewValue    string
	IsDifferent bool
}

// DiffFields computes all field comparisons and marks which ones were altered.
func (r *OrganizationChangeRequest) DiffFields() []ChangedFieldItem {
	if r == nil {
		return nil
	}
	cur := r.CurrentValues
	prop := r.ProposedValues

	items := []ChangedFieldItem{
		{
			FieldKey:    "name_ar",
			LabelAr:     "الاسم بالعربية",
			LabelEn:     "Arabic Name",
			OldValue:    cur.NameAr,
			NewValue:    prop.NameAr,
			IsDifferent: cur.NameAr != prop.NameAr,
		},
		{
			FieldKey:    "name_en",
			LabelAr:     "الاسم بالإنجليزية",
			LabelEn:     "English Name",
			OldValue:    cur.NameEn,
			NewValue:    prop.NameEn,
			IsDifferent: cur.NameEn != prop.NameEn,
		},
		{
			FieldKey:    "type",
			LabelAr:     "نوع / تصنيف المنشأة",
			LabelEn:     "Organization Type",
			OldValue:    cur.Type,
			NewValue:    prop.Type,
			IsDifferent: cur.Type != prop.Type,
		},
		{
			FieldKey:    "min_order_price",
			LabelAr:     "الحد الأدنى لقيمة الطلب",
			LabelEn:     "Min Order Price",
			OldValue:    cur.MinOrderPrice.String(),
			NewValue:    prop.MinOrderPrice.String(),
			IsDifferent: cur.MinOrderPrice.Minor() != prop.MinOrderPrice.Minor(),
		},
		{
			FieldKey:    "max_order_price",
			LabelAr:     "الحد الأقصى لقيمة الطلب",
			LabelEn:     "Max Order Price",
			OldValue:    cur.MaxOrderPrice.String(),
			NewValue:    prop.MaxOrderPrice.String(),
			IsDifferent: cur.MaxOrderPrice.Minor() != prop.MaxOrderPrice.Minor(),
		},
		{
			FieldKey:    "organization_number",
			LabelAr:     "رقم المنظمة",
			LabelEn:     "Organization Number",
			OldValue:    cur.OrganizationNumber,
			NewValue:    prop.OrganizationNumber,
			IsDifferent: cur.OrganizationNumber != prop.OrganizationNumber,
		},
		{
			FieldKey:    "email",
			LabelAr:     "البريد الإلكتروني",
			LabelEn:     "Email",
			OldValue:    cur.Email,
			NewValue:    prop.Email,
			IsDifferent: cur.Email != prop.Email,
		},
		{
			FieldKey:    "phone",
			LabelAr:     "رقم الهاتف",
			LabelEn:     "Phone Number",
			OldValue:    cur.Phone,
			NewValue:    prop.Phone,
			IsDifferent: cur.Phone != prop.Phone,
		},
		{
			FieldKey:    "tax_number",
			LabelAr:     "الرقم الضريبي",
			LabelEn:     "Tax Number",
			OldValue:    cur.TaxNumber,
			NewValue:    prop.TaxNumber,
			IsDifferent: cur.TaxNumber != prop.TaxNumber,
		},
		{
			FieldKey:    "address",
			LabelAr:     "العنوان الرئيسي",
			LabelEn:     "Main Address",
			OldValue:    cur.Address,
			NewValue:    prop.Address,
			IsDifferent: cur.Address != prop.Address,
		},
		{
			FieldKey:    "description_ar",
			LabelAr:     "الوصف (بالعربية)",
			LabelEn:     "Description (Arabic)",
			OldValue:    cur.DescriptionAr,
			NewValue:    prop.DescriptionAr,
			IsDifferent: cur.DescriptionAr != prop.DescriptionAr,
		},
		{
			FieldKey:    "description_en",
			LabelAr:     "الوصف (بالإنجليزية)",
			LabelEn:     "Description (English)",
			OldValue:    cur.DescriptionEn,
			NewValue:    prop.DescriptionEn,
			IsDifferent: cur.DescriptionEn != prop.DescriptionEn,
		},
		{
			FieldKey:    "image",
			LabelAr:     "شعار المنشأة (Logo)",
			LabelEn:     "Logo Image",
			OldValue:    cur.Image,
			NewValue:    prop.Image,
			IsDifferent: prop.Image != "" && prop.Image != cur.Image,
		},
		{
			FieldKey:    "coverage_image",
			LabelAr:     "صورة الغلاف (Cover Image)",
			LabelEn:     "Coverage Image",
			OldValue:    cur.CoverageImage,
			NewValue:    prop.CoverageImage,
			IsDifferent: prop.CoverageImage != "" && prop.CoverageImage != cur.CoverageImage,
		},
	}

	return items
}

// ChangedFieldLabels returns human-readable Arabic tags of modified fields.
func (r *OrganizationChangeRequest) ChangedFieldLabels() []string {
	var labels []string
	for _, diff := range r.DiffFields() {
		if diff.IsDifferent {
			labels = append(labels, diff.LabelAr)
		}
	}
	return labels
}
