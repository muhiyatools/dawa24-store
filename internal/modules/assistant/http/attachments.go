package http

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
)

// resolveAttachments turns client-supplied references into files this caller
// owns, and reads any that have not been read before.
//
// What changed and why: a reference that could not be read used to be dropped
// in silence. The attachment still appeared beside the question in the
// transcript, so the turn looked to the user as though it had been sent with
// the file — and the answer came back about the text alone. Every "the
// assistant ignored my image" report is that shape. Now an unreadable file
// produces a note the model is given, so the answer says the file could not be
// read instead of pretending there was none.
func (h *Handler) resolveAttachments(
	ctx context.Context, actor authctx.Actor, refs []string,
) ([]assistant.Attachment, []assistant.AttachmentDigest, []gateway.ContentPart) {
	if len(refs) == 0 {
		return nil, nil, nil
	}
	if len(refs) > maxAttachmentsPerTurn {
		refs = refs[:maxAttachmentsPerTurn]
	}

	// Attachments go to the answering model. The catalogue does not decide that.
	//
	// It used to. Every file was gated on the Gateway's supports_vision /
	// supports_documents flags, and on max_attachment_mb — and on this Gateway
	// all three are unset for every model. The result was that a photographed
	// medicine box came back as "لا أستطيع رؤية الصور" from a model that, asked
	// directly, describes the picture correctly. The flags are operator
	// metadata, and nobody has filled them in.
	//
	// So the decision is ours, made from things we actually know: the file
	// passed our own type allowlist at upload, and it is within our own upload
	// ceiling. If the model genuinely cannot read it, the Gateway says so and
	// the reader pass below picks it up — a wrong guess costs one retry, while
	// trusting an empty flag cost the feature entirely.
	primary, capErr := h.gw.Capabilities(ctx, gateway.RolePrimary)
	if capErr != nil {
		primary = gateway.ConservativeDefaultCapabilities()
	}

	var (
		atts    []assistant.Attachment
		digests []assistant.AttachmentDigest
		parts   []gateway.ContentPart
	)
	for _, ref := range refs {
		row, err := h.repo.GetAttachment(ctx, ref, actor.OrgID, actor.UserID)
		if err != nil || row == nil {
			h.log.WarnContext(ctx, "assistant: attachment reference did not resolve",
				"user_id", actor.UserID)
			continue
		}

		atts = append(atts, assistant.Attachment{
			Handle:      row.PublicID.String(),
			Filename:    row.Filename,
			MIMEType:    row.MIMEType,
			SizeMB:      float64(row.SizeBytes) / (1024 * 1024),
			ContentHash: row.ContentHash,
			UserID:      row.UserID,
			OrgID:       row.OrganizationID,
			RowID:       row.ID,
		})

		if part, ok := h.directPart(ctx, primary, row); ok {
			parts = append(parts, part)
			continue
		}
		if text, ok := h.readPlainText(ctx, row); ok {
			digests = append(digests, assistant.AttachmentDigest{
				Filename: row.Filename, Text: text,
			})
			continue
		}

		digest := row.Digest
		if digest == "" {
			digest = h.readAttachment(ctx, actor, row)
			if digest != "" {
				if err := h.repo.SetAttachmentDigest(ctx, row.ID, digest); err != nil {
					h.log.WarnContext(ctx, "assistant: cache digest", "error", err)
				}
			}
		}
		if digest == "" {
			// Say so rather than answering as though nothing was attached.
			digest = "تعذّر قراءة محتوى هذا الملف. أبلغ المستخدم بذلك واطلب إعادة إرساله بصيغة أخرى."
		}
		digests = append(digests, assistant.AttachmentDigest{
			Filename: row.Filename, Text: digest,
		})
	}
	return atts, digests, parts
}

// directPart builds the multimodal part for a file the answering model can open
// itself, or reports that it cannot.
func (h *Handler) directPart(
	ctx context.Context, caps gateway.ModelCapabilities, row *assistant.AttachmentRow,
) (gateway.ContentPart, bool) {
	kind := assistant.ClassifyMIME(row.MIMEType)
	if !sendableDirectly(caps, kind) || !withinLimit(caps, row.SizeBytes) {
		return gateway.ContentPart{}, false
	}

	content, err := h.attachmentBytes(ctx, row)
	if err != nil {
		h.log.WarnContext(ctx, "assistant: could not read attachment for direct send",
			"attachment", row.PublicID, "error", err)
		return gateway.ContentPart{}, false
	}

	mime := row.MIMEType
	if kind == assistant.KindImage {
		// A four-megapixel photograph is downscaled on its way to the model.
		// See assistant/imageprep.go: above ~1500 pixels the model reads no
		// more of the label and the request gets slow enough to hit the turn
		// deadline.
		mime, content = assistant.PrepareImageForModel(mime, content)
	}

	return gateway.ContentPart{
		Kind:     partKindFor(kind),
		DataURL:  assistant.DataURL(mime, content),
		Filename: row.Filename,
		MIMEType: mime,
	}, true
}

// sendableDirectly decides whether a file goes to the answering model as-is.
//
// Images always do. They are the common case, every model this Gateway fronts
// has handled them in testing, and a picture described second-hand is worth
// much less than the picture. Audio and video go only when the model claims
// them, because a model that cannot decode audio fails in a way no fallback
// recovers. Documents go when claimed, and are otherwise read as plain text
// where that is possible.
func sendableDirectly(caps gateway.ModelCapabilities, kind string) bool {
	switch kind {
	case assistant.KindImage:
		return true
	case assistant.KindDocument:
		return caps.Documents
	case assistant.KindAudio:
		return caps.Audio
	case assistant.KindVideo:
		return caps.Video
	}
	return false
}

// withinLimit reports whether a file fits the ceiling that actually applies.
//
// A zero max_attachment_mb means the operator never set one, not that the model
// refuses attachments — every model in the live catalogue publishes zero,
// including the one that reports supports_vision=true. Reading zero as "refuse"
// was why a photographed medicine box came back as "لا أستطيع رؤية الصور": the
// model could see it and this function would not let it.
//
// So when the Gateway publishes no ceiling we apply our own upload ceiling,
// which is already far below the Gateway's 24 MiB body cap.
func withinLimit(caps gateway.ModelCapabilities, size int64) bool {
	limit := int64(caps.MaxAttachmentMB) << 20
	if caps.MaxAttachmentMB <= 0 {
		limit = maxAttachmentBytes
	}
	return size <= limit
}
