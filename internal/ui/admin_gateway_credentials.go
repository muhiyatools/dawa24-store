package ui

import "github.com/muhiya/dawa24-store/internal/shared/apperr"

// credentialRejectionMessage renders a rejected Gateway administrator
// credential for the connection-test response.
//
// It lives here rather than in admin_handlers.go because that file is already
// the largest in the repository; nothing new is added to it.
func credentialRejectionMessage(err error) string {
	if e, ok := apperr.As(err); ok {
		return e.LocalizedMsg("ar")
	}
	return "بيانات الاعتماد المُدخلة غير صالحة."
}
