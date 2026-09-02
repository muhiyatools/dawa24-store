package http

import (
	"bytes"
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
	"io"
	"net/http"
)

// maxAttachmentBytes is the per-file ceiling.
const maxAttachmentBytes = 10 << 20

// maxAttachmentsPerTurn bounds how many files one question may carry.
const maxAttachmentsPerTurn = 5

// Upload validates a file, stores its bytes, and returns an opaque reference.
//
// What changed and why it matters: the bytes used to be base64-encoded into a
// process-local map that was never evicted, and then persisted into the
// messages table as a data URL. A 10 MB PDF cost roughly 13 MB of resident heap
// for the life of the process AND 13 MB of JSONB per conversation, replayed to
// the browser on every history load. They now go to object storage, and what is
// kept here is a row.
func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor := authctx.FromContext(ctx)

	if h.storage == nil {
		writeFailure(w, http.StatusServiceUnavailable, assistant.Fail(assistant.CodeInternal))
		return
	}

	if err := r.ParseMultipartForm(maxAttachmentBytes); err != nil {
		writeFailure(w, http.StatusRequestEntityTooLarge,
			assistant.Fail(assistant.CodeAttachmentTooLarge))
		return
	}

	files := r.MultipartForm.File["file"]
	if len(files) == 0 {
		files = r.MultipartForm.File["files"]
	}
	if len(files) == 0 {
		writeFailure(w, http.StatusBadRequest, assistant.Fail(assistant.CodeInvalidRequest))
		return
	}
	if len(files) > maxAttachmentsPerTurn {
		writeFailure(w, http.StatusBadRequest, assistant.Fail(assistant.CodeInvalidRequest))
		return
	}

	type uploaded struct {
		Reference string  `json:"reference"`
		Filename  string  `json:"filename"`
		MIMEType  string  `json:"mime_type"`
		SizeMB    float64 `json:"size_mb"`
	}

	results := make([]uploaded, 0, len(files))
	for _, fh := range files {
		if fh.Size > maxAttachmentBytes {
			writeFailure(w, http.StatusRequestEntityTooLarge,
				assistant.Fail(assistant.CodeAttachmentTooLarge))
			return
		}

		f, err := fh.Open()
		if err != nil {
			writeFailure(w, http.StatusBadRequest, assistant.Fail(assistant.CodeAttachmentRejected))
			return
		}
		content, err := io.ReadAll(io.LimitReader(f, maxAttachmentBytes+1))
		_ = f.Close()
		if err != nil || len(content) > maxAttachmentBytes {
			writeFailure(w, http.StatusRequestEntityTooLarge,
				assistant.Fail(assistant.CodeAttachmentTooLarge))
			return
		}

		// Sniffed, not trusted: the declared Content-Type is attacker-supplied
		// and the extension is only used to refuse an executable outright.
		mime, _, err := assistant.SniffAndValidate(content, fh.Filename)
		if err != nil {
			writeFailure(w, http.StatusBadRequest, assistant.Fail(assistant.CodeAttachmentRejected))
			return
		}

		row := &assistant.AttachmentRow{
			OrganizationID: actor.OrgID,
			UserID:         actor.UserID,
			Filename:       assistant.SanitiseFilename(fh.Filename),
			MIMEType:       mime,
			SizeBytes:      int64(len(content)),
			ContentHash:    assistant.ComputeContentHash(content),
			StorageKey: fmt.Sprintf("capsule/%d/%d/%s",
				actor.OrgID, actor.UserID, uuid.NewString()),
		}

		if err := h.storage.Put(ctx, row.StorageKey, bytes.NewReader(content),
			row.SizeBytes, row.MIMEType); err != nil {
			h.log.ErrorContext(ctx, "assistant: store attachment", "error", err)
			writeFailure(w, http.StatusBadGateway, assistant.Fail(assistant.CodeInternal))
			return
		}
		if err := h.repo.CreateAttachment(ctx, row); err != nil {
			h.log.ErrorContext(ctx, "assistant: record attachment", "error", err)
			writeFailure(w, http.StatusInternalServerError, assistant.Fail(assistant.CodeInternal))
			return
		}

		results = append(results, uploaded{
			Reference: row.PublicID.String(),
			Filename:  row.Filename,
			MIMEType:  row.MIMEType,
			SizeMB:    float64(row.SizeBytes) / (1024 * 1024),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"attachments": results})
}

// resolveAttachments turns client-supplied references into files this caller
// owns, and reads any that have not been read before.
//
// A reference that does not resolve is silently dropped rather than failing the
// turn: it is either expired, already swept, or somebody else's, and in every
// case the right behaviour is to answer the question without it.
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

		kind := assistant.ClassifyMIME(row.MIMEType)
		if sendableDirectly(primary, kind) && withinLimit(primary, row.SizeBytes) {
			if dataURL, derr := h.dataURL(ctx, row); derr == nil {
				parts = append(parts, gateway.ContentPart{
					Kind:     partKindFor(kind),
					DataURL:  dataURL,
					Filename: row.Filename,
					MIMEType: row.MIMEType,
				})
				continue
			}
			h.log.WarnContext(ctx, "assistant: could not read attachment for direct send",
				"attachment", row.PublicID)
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
		if digest != "" {
			digests = append(digests, assistant.AttachmentDigest{
				Filename: row.Filename, Text: digest,
			})
		}
	}
	return atts, digests, parts
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
