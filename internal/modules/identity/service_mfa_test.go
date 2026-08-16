package identity

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

func TestServiceProfileAndPermissions(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(repo, nil, logger)

	// Register user
	user, _, err := svc.Register(ctx, RegisterInput{
		Email:    "test-mfa@dawa24.eg",
		Password: "InitialPassword123!",
		NameAr:   "مستخدم تجريبي",
		NameEn:   "Test User",
		Role:     "customer",
		Language: i18n.AR,
	})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// 1. Profile update
	updatedUser, err := svc.UpdateProfile(ctx, user.ID, "اسم جديد", "New Name", "+201111111111", "Africa/Cairo", "ar")
	if err != nil {
		t.Fatalf("UpdateProfile failed: %v", err)
	}
	if updatedUser.Phone != "+201111111111" {
		t.Errorf("got phone %q, want +201111111111", updatedUser.Phone)
	}

	// 2. Address update
	addr := &UserAddress{
		UserID:    user.ID,
		Recipient: "Recipient",
		Address:   "Original Address",
		CityID:    1,
		IsDefault: false,
	}
	createdAddr, err := svc.CreateAddress(ctx, addr)
	if err != nil {
		t.Fatalf("CreateAddress failed: %v", err)
	}
	createdAddr.Address = "Updated Address"
	if err := svc.UpdateAddress(ctx, createdAddr); err != nil {
		t.Fatalf("UpdateAddress failed: %v", err)
	}

	// 3. UserBelongsToOrg & ResolvePermissions
	belongs, err := svc.UserBelongsToOrg(ctx, user.ID, 10)
	if err != nil || !belongs {
		t.Fatalf("UserBelongsToOrg failed: %v", err)
	}

	perms, err := svc.ResolvePermissions(ctx, user.ID, 10)
	if err != nil {
		t.Fatalf("ResolvePermissions failed: %v", err)
	}
	_ = perms

	// 4. Logout without session store is a no-op
	if err := svc.Logout(ctx, "nonexistent"); err != nil {
		t.Fatalf("Logout failed: %v", err)
	}
}
