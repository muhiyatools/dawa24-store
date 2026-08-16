package main

import (
	"testing"
	"time"
)

func TestETLUserTransformationAndValidation(t *testing.T) {
	v := NewValidator()
	tr := NewTransformer()

	src := &SourceUser{
		ID:        42,
		Name:      "أحمد محمد علي",
		Email:     " Ahmed@Example.com ",
		Password:  "$2y$10$92IXUNpkjO0rOQ5byMi.Ye4oKoEa3Ro9llC/.og/at2.uheWG/igi",
		Role:      "customer",
		Phone:     "+20 100 123 4567",
		CreatedAt: time.Now(),
	}

	if err := v.ValidateUser(src); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}

	tgt := tr.TransformUser(src)

	if tgt.Email != "ahmed@example.com" {
		t.Fatalf("expected normalized email 'ahmed@example.com', got '%s'", tgt.Email)
	}

	if tgt.Name["ar"] != "احمد محمد علي" { // Normalized Arabic without hamzas
		t.Fatalf("expected normalized Arabic name 'احمد محمد علي', got '%s'", tgt.Name["ar"])
	}

	if tgt.Phone != "+201001234567" {
		t.Fatalf("expected cleaned phone '+201001234567', got '%s'", tgt.Phone)
	}
}

func TestETLProductTransformationAndMoneyConversion(t *testing.T) {
	v := NewValidator()
	tr := NewTransformer()

	src := &SourceProduct{
		ID:          101,
		NameAr:      "بنادول إكسترا أقراص",
		NameEn:      "Panadol Extra Tablets",
		Slug:        "panadol-extra-tablets",
		Description: "Effective pain relief",
		CategoryID:  1,
		Price:       45.50,
		Stock:       200,
		VendorOrgID: 10,
		CreatedAt:   time.Now(),
	}

	if err := v.ValidateProduct(src); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}

	p, vObj := tr.TransformProduct(src)

	if p.Name["ar"] != "بنادول اكسترا اقراص" {
		t.Fatalf("expected normalized Arabic name 'بنادول اكسترا اقراص', got '%s'", p.Name["ar"])
	}

	if vObj.Price.Minor() != 4550 {
		t.Fatalf("expected money amount 4550 minor units (45.50 EGP), got %d", vObj.Price.Minor())
	}
}
