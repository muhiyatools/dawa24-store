package ui_test

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// The assistant drawer shipped with two defects that made it unusable, and both
// were single lines of markup. These guard against their return.

// The composer sat below a message list styled `flex:1 1 auto; height:100%`.
// height:100% on a flex child overrides the flex sizing and claims the whole
// column, so the input row was pushed out of the parent's overflow:hidden box
// and could not be seen or reached at any viewport size.
func TestAssistantMessageListDoesNotClaimFullHeight(t *testing.T) {
	src := readAssistant(t)

	listStyle := regexp.MustCompile(`id="capsule-messages-container"[\s\S]{0,400}?style="([^"]*)"`)
	m := listStyle.FindStringSubmatch(src)
	if m == nil {
		t.Fatal("could not find the messages container; if it was renamed, update this test rather than deleting it")
	}
	style := m[1]

	if strings.Contains(style, "height:100%") {
		t.Errorf("messages container declares height:100%%, which hides the composer.\nstyle: %s", style)
	}
	if !strings.Contains(style, "min-height:0") {
		t.Errorf("messages container needs min-height:0 or it refuses to shrink.\nstyle: %s", style)
	}
	if !strings.Contains(style, "flex:1 1 auto") {
		t.Errorf("messages container must flex, not be fixed.\nstyle: %s", style)
	}
}

// Alpine wraps objects pushed into a reactive array in a proxy. Holding a
// reference to the ORIGINAL object and mutating it updates nothing, which is
// why the answer bubble stayed permanently empty while tokens streamed in.
func TestAssistantStreamTargetIsTheReactiveProxy(t *testing.T) {
	src := readAssistant(t)

	if regexp.MustCompile(`const assistantMsg = \{`).MatchString(src) {
		t.Error("assistantMsg is bound to a raw object literal; mutations will not render. " +
			"Push first, then read the element back out of this.messages.")
	}
	if !strings.Contains(src, "const assistantMsg = this.messages[this.messages.length - 1]") {
		t.Error("assistantMsg must reference the element inside this.messages so Alpine observes it")
	}
}

// The thinking state was a bordered card reading "جاري صياغة الإجابة…" that took
// more room than most answers and read as a stuck state.
func TestAssistantThinkingIndicatorIsNotACard(t *testing.T) {
	// Checked against the GENERATED file, not the template: the template still
	// mentions the old wording in the comment explaining why it went.
	b, err := os.ReadFile("components/capsule_assistant_templ.go")
	if err != nil {
		t.Fatalf("read generated assistant: %v", err)
	}
	rendered := string(b)

	if strings.Contains(rendered, "جاري صياغة الإجابة") {
		t.Error("the thinking state should be an animated indicator, not a labelled card")
	}
	if !strings.Contains(rendered, "capsule-thinking") {
		t.Error("expected the capsule-thinking indicator in the rendered output")
	}
}

func readAssistant(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("components/capsule_assistant.templ")
	if err != nil {
		t.Fatalf("read assistant template: %v", err)
	}
	return string(b)
}
