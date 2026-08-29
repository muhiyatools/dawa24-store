package platformadmin

import (
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Refusing to send the wrong secret to a third party.
//
// The Gateway administrator credential is sent as HTTP Basic auth to whatever
// host is configured in إعدادات النظام. That makes this one field the shortest
// path there is from "an operator pasted the wrong thing" to "a production
// secret has left the building".
//
// It is not a hypothetical. The live configuration was found holding
//
//	postgres:RBSW2NW9-dy4d-63ZLK0DC
//
// — the production PostgreSQL superuser credential — which the client dutifully
// split on the colon and sent to the Gateway host on every provisioning call,
// every plan listing and every usage query. Nobody noticed, because the failure
// mode of a wrong credential here is silent: provisioning returned a user id
// with no key, and every caller fell through to its fallback.
//
// The check below is deliberately about shape, not about any particular secret.
// It cannot know which password is which, and it does not try. It knows what a
// database connection string looks like, and it knows that a value carrying the
// name of a database role or a scheme prefix is not a Gateway credential.

// databaseUsers are the role names that turn up at the front of a DSN-derived
// credential. An operator whose Gateway account really is called "postgres"
// has bigger problems than this check.
var databaseUsers = []string{"postgres", "root", "mysql", "mariadb", "dawa24_app"}

// secretSchemes are URL schemes that mean the value is a connection string, not
// a password.
var secretSchemes = []string{
	"postgres://", "postgresql://", "mysql://", "redis://", "rediss://",
	"mongodb://", "amqp://", "http://", "https://",
}

// ValidateAdminCredential rejects a value that is evidently not a Gateway
// administrator credential.
//
// It errs towards accepting: a false rejection blocks an operator from
// configuring AI at all, while the class of mistake it exists to catch has one
// very recognisable shape. Anything it is unsure about is allowed through.
func ValidateAdminCredential(raw string) error {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil
	}

	lower := strings.ToLower(value)
	for _, scheme := range secretSchemes {
		if strings.HasPrefix(lower, scheme) {
			return apperr.Validation("gateway.credential_is_connection_string",
				"القيمة المُدخلة تبدو سلسلة اتصال (Connection String) وليست بيانات اعتماد بوابة الذكاء الاصطناعي. "+
					"هذه القيمة تُرسل إلى خادم البوابة عند كل عملية، فلا يجوز وضع بيانات قاعدة البيانات هنا.", nil)
		}
	}

	user, _, hasColon := strings.Cut(value, ":")
	if !hasColon {
		return nil
	}
	user = strings.ToLower(strings.TrimSpace(user))
	for _, name := range databaseUsers {
		if user == name {
			return apperr.Validation("gateway.credential_is_database_user",
				"اسم المستخدم في بيانات الاعتماد يطابق اسم مستخدم قاعدة بيانات. "+
					"هذه القيمة تُرسل إلى خادم البوابة الخارجي عند كل عملية — "+
					"يرجى إدخال بيانات اعتماد مدير بوابة الذكاء الاصطناعي فقط.", nil)
		}
	}
	return nil
}

// CredentialLooksMisconfigured reports the same condition without producing an
// error, for a settings screen that wants to warn about a value already stored
// rather than block one being submitted.
//
// Existing deployments are the reason this exists separately: the wrong
// credential is already in the database on at least one of them, and an
// operator opening the screen needs to be told so even though they are not the
// one who typed it.
func (g *GatewaySettings) CredentialLooksMisconfigured() bool {
	return g != nil && ValidateAdminCredential(g.APIKey) != nil
}
