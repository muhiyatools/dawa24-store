package ui_test

import (
	"os"
	"strings"
	"testing"
)

// The drawer must never be able to end a turn showing nothing.
//
// Three real failures produced a blank bubble that stayed blank forever: a turn
// that emitted no delta because it spent its budget reasoning, a stream blocked
// or buffered between the server and the browser, and an EventSource that got a
// non-200 and closed without retrying. Each is covered by a mechanism here, and
// each mechanism is asserted rather than assumed.
func TestAssistantAlwaysResolvesATurn(t *testing.T) {
	src := readAssistantScript(t)

	checks := map[string]string{
		"the terminal frame's answer is used when no delta arrived": "if (!target.text && d.answer) target.text = d.answer;",
		"a watchdog exists for a stream that goes quiet":            "armWatchdog(",
		"there is a JSON fallback that reads the turn directly":     "/api/v1/assistant/turns/' + turnID)",
		"every ending goes through one place":                       "settle(target)",
		"an empty ending still says something":                      "لم يصل رد من المساعد",
	}
	for what, needle := range checks {
		if !strings.Contains(src, needle) {
			t.Errorf("missing: %s (looked for %q)", what, needle)
		}
	}
}

// The fallback is worthless if the endpoint it polls is not mounted.
func TestTurnStatusEndpointIsRouted(t *testing.T) {
	b, err := os.ReadFile("../modules/assistant/http/handler.go")
	if err != nil {
		t.Fatalf("read handler: %v", err)
	}
	if !strings.Contains(string(b), `g.Get("/api/v1/assistant/turns/{id}", h.TurnStatus)`) {
		t.Error("the JSON turn-status endpoint is not registered; the client's " +
			"fallback would poll a 404 and the answer would never appear")
	}
}

// Images must reach the answering model itself when it can see them.
//
// The live catalogue reports supports_vision=true and max_attachment_mb=0 for
// the primary model. Treating a zero ceiling as "refuse" is what produced
// "لا أستطيع رؤية الصور" for a photographed medicine box.
func TestZeroAttachmentCeilingDoesNotRefuseImages(t *testing.T) {
	b, err := os.ReadFile("../modules/assistant/http/attachments.go")
	if err != nil {
		t.Fatalf("read attachments: %v", err)
	}
	src := string(b)
	if !strings.Contains(src, "limit = maxAttachmentBytes") {
		t.Error("a model publishing no attachment ceiling must fall back to our " +
			"own upload ceiling, not refuse the file")
	}
	if !strings.Contains(src, "capabilityFor(primary, kind)") {
		t.Error("attachments must be offered to the primary model before the " +
			"reader model; describing a photo is worse than looking at it")
	}
}

func readAssistantScript(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("components/capsule_assistant_script.templ")
	if err != nil {
		t.Fatalf("read assistant script: %v", err)
	}
	return string(b)
}
