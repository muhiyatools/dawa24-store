package attachments

import (
	"context"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/stretchr/testify/assert"
)

func TestRequirementsFor(t *testing.T) {
	t.Run("customer", func(t *testing.T) {
		reqs := RequirementsFor("customer")
		assert.Len(t, reqs, 5)
		pharmacistFound, pharmacistRequired := false, false
		for _, r := range reqs {
			if r.DocType == DocPharmacistLicense {
				pharmacistFound, pharmacistRequired = true, r.Required
			}
		}
		assert.True(t, pharmacistFound)
		assert.True(t, pharmacistRequired)
	})

	t.Run("vendor", func(t *testing.T) {
		reqs := RequirementsFor("vendor")
		assert.Len(t, reqs, 4)
		for _, r := range reqs {
			assert.NotEqual(t, DocPharmacistLicense, r.DocType)
		}
	})

	t.Run("unknown treated as customer", func(t *testing.T) {
		assert.Len(t, RequirementsFor("wholesaler"), 5)
	})
}

func TestService_MissingRequiredDocuments(t *testing.T) {
	ctx := authctx.WithActor(context.Background(), authctx.Actor{
		OrganizationID: 77,
		UserID:         1,
	})

	now := time.Now().UTC()
	doc := func(typ DocumentType, status DocumentStatus, deleted bool) *Document {
		d := &Document{
			OrganizationID: ptrI64(77),
			DocumentType:   typ,
			Status:         status,
			CreatedAt:      now,
		}
		if deleted {
			d.DeletedAt = &now
		}
		return d
	}

	tests := []struct {
		name    string
		orgID   int64
		orgType string
		docs    []*Document
		want    []DocumentType
		wantErr bool
	}{
		{
			name:    "no org",
			orgID:   0,
			wantErr: true,
		},
		{
			name:    "complete customer clears",
			orgID:   77,
			orgType: "customer",
			docs: []*Document{
				doc(DocCommercialRegister, StatusVerified, false),
				doc(DocTaxCard, StatusVerified, false),
				doc(DocPharmacyLicense, StatusVerified, false),
				doc(DocPharmacistLicense, StatusVerified, false),
			},
		},
		{
			name:    "customer missing pharmacist card",
			orgID:   77,
			orgType: "customer",
			docs: []*Document{
				doc(DocCommercialRegister, StatusVerified, false),
				doc(DocTaxCard, StatusVerified, false),
				doc(DocPharmacyLicense, StatusVerified, false),
			},
			want: []DocumentType{DocPharmacistLicense},
		},
		{
			name:    "pending does not satisfy",
			orgID:   77,
			orgType: "customer",
			docs: []*Document{
				doc(DocCommercialRegister, StatusPending, false),
				doc(DocTaxCard, StatusVerified, false),
				doc(DocPharmacyLicense, StatusVerified, false),
				doc(DocPharmacistLicense, StatusVerified, false),
			},
			want: []DocumentType{DocCommercialRegister},
		},
		{
			name:    "rejected does not satisfy",
			orgID:   77,
			orgType: "customer",
			docs: []*Document{
				doc(DocCommercialRegister, StatusRejected, false),
				doc(DocTaxCard, StatusVerified, false),
				doc(DocPharmacyLicense, StatusVerified, false),
				doc(DocPharmacistLicense, StatusVerified, false),
			},
			want: []DocumentType{DocCommercialRegister},
		},
		{
			name:    "soft deleted does not satisfy",
			orgID:   77,
			orgType: "customer",
			docs: []*Document{
				doc(DocCommercialRegister, StatusVerified, true),
				doc(DocTaxCard, StatusVerified, false),
				doc(DocPharmacyLicense, StatusVerified, false),
				doc(DocPharmacistLicense, StatusVerified, false),
			},
			want: []DocumentType{DocCommercialRegister},
		},
		{
			name:    "authorization letter never required",
			orgID:   77,
			orgType: "customer",
			docs:   []*Document{doc(DocAuthorizationLetter, StatusPending, false)},
			want: []DocumentType{DocCommercialRegister, DocTaxCard, DocPharmacyLicense, DocPharmacistLicense},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &Service{repo: &stubRepo{docs: tt.docs}}
			got, err := svc.MissingRequiredDocuments(ctx, tt.orgID, tt.orgType)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestService_MissingRequiredDocuments_error(t *testing.T) {
	ctx := authctx.WithActor(context.Background(), authctx.Actor{OrganizationID: 1, UserID: 1})
	svc := &Service{repo: &stubRepo{listErr: errDocList}}
	_, err := svc.MissingRequiredDocuments(ctx, 1, "customer")
	assert.Error(t, err)
}

var errDocList = apperr.Internal(assert.AnError)

func ptrI64(v int64) *int64 { return &v }