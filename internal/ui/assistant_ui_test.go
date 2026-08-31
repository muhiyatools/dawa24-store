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
	if !strings.Contains(style, "flex:1 1") {
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
	b2, _ := os.ReadFile("components/capsule_assistant_script.templ")
	return string(b) + "\n" + string(b2)
}

// The drawer's size was written as a style attribute, so no media query could
// touch it: the assistant had no small-screen behaviour and on a short viewport
// it ran past the bottom edge with the composer out of reach and the answer
// unscrollable.
func TestAssistantDrawerSizeIsStyleable(t *testing.T) {
	src := readAssistant(t)

	if strings.Contains(src, `height:620px`) {
		t.Error("the drawer height is inline again; a media query cannot override a style attribute")
	}
	if !strings.Contains(src, `class="capsule-drawer"`) {
		t.Error("the drawer should carry the capsule-drawer class, which is where its size lives")
	}
	for _, need := range []string{
		".capsule-drawer {",
		"@media (max-width: 640px)", // full-screen sheet on a phone
		"height: 100dvh",            // visible viewport, not layout viewport
	} {
		if !strings.Contains(src, need) {
			t.Errorf("missing drawer rule: %s", need)
		}
	}
}

// A flex child with basis auto sizes to its content first, so a long answer can
// push the composer out of the parent before min-height:0 is consulted. Basis 0
// makes the scroll area take the leftover space and nothing more.
func TestAssistantScrollAreaUsesDefiniteBasis(t *testing.T) {
	src := readAssistant(t)

	i := strings.Index(src, `id="capsule-messages-container"`)
	if i < 0 {
		t.Fatal("messages container not found")
	}
	seg := src[i : i+600]

	if !strings.Contains(seg, "flex:1 1 0%") {
		t.Error("scroll area must use flex:1 1 0%, not basis auto")
	}
	if !strings.Contains(seg, "overflow-y:auto") {
		t.Error("scroll area must scroll")
	}
}

// Every answer is from the assistant; repeating its name and avatar above each
// one is noise in a two-party chat and costs a line of the drawer.
func TestAssistantMessagesHaveNoPerMessageByline(t *testing.T) {
	b, err := os.ReadFile("components/capsule_assistant_templ.go")
	if err != nil {
		t.Fatalf("read generated assistant: %v", err)
	}
	rendered := string(b)

	// The header keeps its title, so exactly one occurrence is expected.
	if n := strings.Count(rendered, "كبسولة</span>"); n != 1 {
		t.Errorf("expected the drawer header title only, found %d occurrences of the byline", n)
	}
}
