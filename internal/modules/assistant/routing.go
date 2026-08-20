package assistant

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/platform/gateway"
)

// Plan describes how one user turn will be executed across Gateway models.
type Plan struct {
	DirectParts []gateway.ContentPart
	PrePass     []Attachment
	Rejected    []RejectedAttachment
}

// RejectedAttachment records why a file was refused before any model invocation.
type RejectedAttachment struct {
	Attachment Attachment
	Reason     string // unsupported_type | too_large | no_capable_model
}

// PlanTurn classifies each attachment against live Gateway capabilities and determines
// whether it can be sent directly to the primary model, requires an attachment model pre-pass,
// or must be refused.
func (s *Service) PlanTurn(ctx context.Context, atts []Attachment) (Plan, error) {
	primaryCaps, err := s.gateway.Capabilities(ctx, gateway.RolePrimary)
	if err != nil {
		primaryCaps = gateway.ConservativeDefaultCapabilities()
	}
	attachCaps, err := s.gateway.Capabilities(ctx, gateway.RoleAttachment)
	if err != nil {
		attachCaps = gateway.ConservativeDefaultCapabilities()
	}

	plan := Plan{
		DirectParts: make([]gateway.ContentPart, 0),
		PrePass:     make([]Attachment, 0),
		Rejected:    make([]RejectedAttachment, 0),
	}

	for _, a := range atts {
		kind := ClassifyMIME(a.MIMEType)
		if kind == KindUnknown {
			plan.Rejected = append(plan.Rejected, RejectedAttachment{
				Attachment: a,
				Reason:     "unsupported_type",
			})
			continue
		}

		capMB := 10.0
		if primaryCaps.MaxAttachmentMB > 0 {
			capMB = float64(primaryCaps.MaxAttachmentMB)
		} else if attachCaps.MaxAttachmentMB > 0 {
			capMB = float64(attachCaps.MaxAttachmentMB)
		}

		if a.SizeMB > capMB && capMB > 0 {
			plan.Rejected = append(plan.Rejected, RejectedAttachment{
				Attachment: a,
				Reason:     "too_large",
			})
			continue
		}

		// 1. Direct pass to primary model if it supports this modality
		switch kind {
		case KindImage:
			if primaryCaps.Vision {
				plan.DirectParts = append(plan.DirectParts, gateway.ContentPart{
					Kind:     gateway.PartImage,
					DataURL:  a.DataURL,
					Filename: a.Filename,
					MIMEType: a.MIMEType,
				})
				continue
			}
		case KindVideo:
			if primaryCaps.Video {
				plan.DirectParts = append(plan.DirectParts, gateway.ContentPart{
					Kind:     gateway.PartVideo,
					DataURL:  a.DataURL,
					Filename: a.Filename,
					MIMEType: a.MIMEType,
				})
				continue
			}
		case KindAudio:
			if primaryCaps.Audio {
				plan.DirectParts = append(plan.DirectParts, gateway.ContentPart{
					Kind:     gateway.PartAudio,
					DataURL:  a.DataURL,
					Filename: a.Filename,
					MIMEType: a.MIMEType,
				})
				continue
			}
		case KindDocument:
			if primaryCaps.Documents {
				plan.DirectParts = append(plan.DirectParts, gateway.ContentPart{
					Kind:     gateway.PartFile,
					DataURL:  a.DataURL,
					Filename: a.Filename,
					MIMEType: a.MIMEType,
				})
				continue
			}
		}

		// 2. Pre-pass via attachment model if it supports this modality
		switch kind {
		case KindDocument:
			if attachCaps.Documents {
				plan.PrePass = append(plan.PrePass, a)
				continue
			}
		case KindAudio:
			if attachCaps.Audio {
				plan.PrePass = append(plan.PrePass, a)
				continue
			}
		case KindImage:
			if attachCaps.Vision {
				plan.PrePass = append(plan.PrePass, a)
				continue
			}
		case KindVideo:
			if attachCaps.Video {
				plan.PrePass = append(plan.PrePass, a)
				continue
			}
		}

		// 3. Neither model can process this modality
		plan.Rejected = append(plan.Rejected, RejectedAttachment{
			Attachment: a,
			Reason:     "no_capable_model",
		})
	}

	return plan, nil
}
