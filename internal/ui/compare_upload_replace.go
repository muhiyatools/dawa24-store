package ui

import (
	"context"
	"fmt"
	"mime/multipart"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// The two halves of "a bulk import replaces what came before it".
//
// trimToPlanQuota decides what a batch may contain; activeCompareFileIDs
// records what the batch is about to replace. The replacement itself is
// deferred until the new files have been staged successfully — see
// compare.Service.ReplacePreviousFiles.

// trimToPlanQuota keeps the first maxAllowed files of a batch and names the
// rest, instead of refusing the whole upload.
//
// It used to refuse: over the plan's list count, the upload came back with
// "archive or delete your old lists to proceed", which asked the vendor to do
// by hand the very thing the upload does for them — the previous lists are
// replaced automatically, so they were never what filled the quota. The client
// already trims the selection this way before sending it (compare_tool.js);
// this is the authoritative version of the same rule, for a client that did
// not run it.
//
// maxAllowed of 0 means unlimited.
func trimToPlanQuota(headers []*multipart.FileHeader, maxAllowed int, lang string) ([]*multipart.FileHeader, []string) {
	if maxAllowed <= 0 || len(headers) <= maxAllowed {
		return headers, nil
	}
	skipped := make([]string, 0, len(headers)-maxAllowed)
	for _, h := range headers[maxAllowed:] {
		skipped = append(skipped, h.Filename+" ("+
			fmt.Sprintf(i18n.T(lang, "compare.upload.quota_skipped"), maxAllowed)+")")
	}
	return headers[:maxAllowed], skipped
}

// activeCompareFileIDs lists the compare files the vendor holds right now.
//
// Taken before anything of the new batch is created, so the ids name the
// generation being replaced and can never include a file from the batch that
// replaces it. Returns nothing on a read error: a snapshot that could not be
// taken must archive nothing rather than guess.
func (h *UIHandler) activeCompareFileIDs(ctx context.Context, userID int64, orgID *int64) []int64 {
	if h.compareSvc == nil {
		return nil
	}
	files, err := h.compareSvc.ListFiles(ctx, userID, orgID, nil)
	if err != nil {
		h.log.ErrorContext(ctx, "could not snapshot active compare files before upload", "error", err)
		return nil
	}
	ids := make([]int64, 0, len(files))
	for _, f := range files {
		if f != nil {
			ids = append(ids, f.ID)
		}
	}
	return ids
}
