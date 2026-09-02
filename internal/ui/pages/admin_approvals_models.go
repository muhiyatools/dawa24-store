package pages

import (
	"github.com/muhiya/dawa24-store/internal/modules/attachments"
	"github.com/muhiya/dawa24-store/internal/modules/org"
)

// AdminApprovalsData holds the unified view models for org approvals, document audits, and document requests.
type AdminApprovalsData struct {
	ActiveTab        string
	StatusFilter     string
	Organizations    []*org.Organization
	AllOrganizations []*org.Organization
	OrgDocs          map[int64][]*attachments.Document
	UploadedDocs     []*attachments.Document
	DocRequests      []*attachments.DocumentRequest
	OrgNames         map[int64]string
	OrgPage          int
	OrgPerPage       int
	OrgTotalCount    int
}
