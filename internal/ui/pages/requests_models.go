package pages

import (
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/workflow"
)

// RequestsData is the /requests inbox view model.
type RequestsData struct {
	Requests     []*workflow.Request
	CurrentOrgID int64
	Suppliers    []*org.Organization
}

// TabClass returns the button classes for a status filter tab.
func TabClass(active, current string) string {
	if active == current {
		return "btn btn-sm btn-primary"
	}
	return "btn btn-sm btn-secondary"
}

// reqTypeLabel maps a request type onto an Arabic label.
func reqTypeLabel(t workflow.RequestType) string {
	switch t {
	case workflow.RequestAction:
		return "إجراء"
	case workflow.RequestApproval:
		return "اعتماد"
	default:
		return "مستند"
	}
}

// reqStatusLabel maps a request status onto an Arabic label.
func reqStatusLabel(s workflow.RequestStatus) string {
	switch s {
	case workflow.RequestAccepted:
		return "مقبول"
	case workflow.RequestDeclined:
		return "مرفوض"
	case workflow.RequestCancelled:
		return "ملغى"
	default:
		return "معلق"
	}
}
