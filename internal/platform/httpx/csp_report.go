package httpx

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
)

// CSPReportPayload models standard CSP violation reports sent by modern browsers.
type CSPReportPayload struct {
	CSPReport struct {
		DocumentURI        string `json:"document-uri"`
		Referrer           string `json:"referrer"`
		ViolatedDirective  string `json:"violated-directive"`
		EffectiveDirective string `json:"effective-directive"`
		OriginalPolicy     string `json:"original-policy"`
		Disposition        string `json:"disposition"`
		BlockedURI         string `json:"blocked-uri"`
		LineNumber         int    `json:"line-number"`
		SourceFile         string `json:"source-file"`
		StatusCode         int    `json:"status-code"`
		ScriptSample       string `json:"script-sample"`
	} `json:"csp-report"`
}

// CSPReportHandler receives browser CSP violation reports, bounds request size,
// and logs structured alerts for real-time exploit and injection monitoring.
func CSPReportHandler(log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		// Cap incoming payload size to 32 KB to avoid memory exhaustion
		r.Body = http.MaxBytesReader(w, r.Body, 32*1024)
		data, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Payload Too Large or Unreadable", http.StatusBadRequest)
			return
		}

		if len(data) == 0 {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		var payload CSPReportPayload
		if err := json.Unmarshal(data, &payload); err == nil && payload.CSPReport.DocumentURI != "" {
			rep := payload.CSPReport
			if log != nil {
				log.WarnContext(r.Context(), "content security policy violation reported",
					"blocked_uri", rep.BlockedURI,
					"violated_directive", rep.ViolatedDirective,
					"effective_directive", rep.EffectiveDirective,
					"document_uri", rep.DocumentURI,
					"source_file", rep.SourceFile,
					"line_number", rep.LineNumber,
					"disposition", rep.Disposition,
					"client_ip", clientIP(r),
				)
			}
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
