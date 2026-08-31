package ui_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func TestAuthLoginPage_Render(t *testing.T) {
	ctx := context.Background()

	var buf bytes.Buffer
	comp := pages.LoginPage("ar", "rtl", "")
	if err := comp.Render(ctx, &buf); err != nil {
		t.Fatalf("LoginPage.Render failed: %v", err)
	}

	html := buf.String()

	expectedSnippets := []string{
		"auth-page-wrapper",
		"auth-card",
		"auth-header",
		"auth-title",
		"منصة دواء 24",
		"تسجيل الدخول إلى حسابك الصيدلي أو التجاري",
		"pwd-input-wrap",
		"pwd-toggle-btn",
		"auth-footer",
		"/auth/register",
		"/auth/forgot",
	}

	for _, snippet := range expectedSnippets {
		if !strings.Contains(html, snippet) {
			t.Errorf("Expected LoginPage HTML to contain %q, but not found", snippet)
		}
	}
}

func TestAuthRegisterPage_Render(t *testing.T) {
	ctx := context.Background()

	form := pages.RegisterFormData{
		LegalName:   "صيدلية المستقبل",
		TradeNameAr: "صيدلية المستقبل",
	}

	var buf bytes.Buffer
	comp := pages.RegisterPage("ar", "rtl", form, nil)
	if err := comp.Render(ctx, &buf); err != nil {
		t.Fatalf("RegisterPage.Render failed: %v", err)
	}

	html := buf.String()

	expectedSnippets := []string{
		"auth-page-wrapper",
		"auth-card-wide",
		"auth-stepper",
		"onboard-step",
		"type-card",
		"data-account-type=\"customer\"",
		"data-account-type=\"vendor\"",
		"data-account-type=\"job_seeker\"",
		"pwd-strength-bars",
		"pwd-checklist",
		"auth-footer",
	}

	for _, snippet := range expectedSnippets {
		if !strings.Contains(html, snippet) {
			t.Errorf("Expected RegisterPage HTML to contain %q, but not found", snippet)
		}
	}
}

func TestMFAVerifyPage_Render(t *testing.T) {
	ctx := context.Background()

	var buf bytes.Buffer
	comp := pages.MFAVerifyPage("ar", "rtl", "pharmacist@dawa24.eg", "", "/dashboard")
	if err := comp.Render(ctx, &buf); err != nil {
		t.Fatalf("MFAVerifyPage.Render failed: %v", err)
	}

	html := buf.String()

	expectedSnippets := []string{
		"auth-page-wrapper",
		"auth-card",
		"auth-totp-input",
		"التحقق بخطوتين (MFA)",
		"pharmacist@dawa24.eg",
		"Google Authenticator",
		"mfa-recovery-code",
		"auth-footer",
	}

	for _, snippet := range expectedSnippets {
		if !strings.Contains(html, snippet) {
			t.Errorf("Expected MFAVerifyPage HTML to contain %q, but not found", snippet)
		}
	}
}

func TestAuthPasswordRecoveryPages_Render(t *testing.T) {
	ctx := context.Background()

	// Forgot Password
	var bufForgot bytes.Buffer
	compForgot := pages.ForgotPasswordPage("ar", "rtl")
	if err := compForgot.Render(ctx, &bufForgot); err != nil {
		t.Fatalf("ForgotPasswordPage.Render failed: %v", err)
	}
	htmlForgot := bufForgot.String()
	if !strings.Contains(htmlForgot, "auth-page-wrapper") || !strings.Contains(htmlForgot, "استعادة كلمة المرور") {
		t.Errorf("Expected ForgotPasswordPage to render properly")
	}

	// Reset Password
	var bufReset bytes.Buffer
	compReset := pages.ResetPasswordPage("ar", "rtl", "test-token-123")
	if err := compReset.Render(ctx, &bufReset); err != nil {
		t.Fatalf("ResetPasswordPage.Render failed: %v", err)
	}
	htmlReset := bufReset.String()
	if !strings.Contains(htmlReset, "auth-page-wrapper") || !strings.Contains(htmlReset, "تعيين كلمة المرور") || !strings.Contains(htmlReset, "test-token-123") {
		t.Errorf("Expected ResetPasswordPage to render properly")
	}
}
