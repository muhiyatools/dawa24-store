package platformadmin

import "testing"

// The exact failure that reached production twice: the database password, first
// with its username attached and then bare after the username was stripped.
func TestValidateAdminCredentialRejectsDatabasePassword(t *testing.T) {
	SetKnownDatabaseSecret("s3cr3t-db-pass")
	t.Cleanup(func() { SetKnownDatabaseSecret("") })

	for _, value := range []string{"s3cr3t-db-pass", "postgres:s3cr3t-db-pass", "  s3cr3t-db-pass  "} {
		if err := ValidateAdminCredential(value); err == nil {
			t.Errorf("ValidateAdminCredential(%q) = nil, want rejection", value)
		}
	}

	if err := ValidateAdminCredential("a-real-gateway-password"); err != nil {
		t.Errorf("ValidateAdminCredential(unrelated) = %v, want nil", err)
	}
}

// With no secret registered the check must not fire, or a deployment whose DSN
// carries no password could never configure the Gateway at all.
func TestValidateAdminCredentialWithoutKnownSecret(t *testing.T) {
	SetKnownDatabaseSecret("")
	if err := ValidateAdminCredential("anything-at-all"); err != nil {
		t.Errorf("ValidateAdminCredential = %v, want nil", err)
	}
}
