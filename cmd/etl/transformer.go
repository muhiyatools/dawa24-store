package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/arabic"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// SourceUser represents a user record from MariaDB.
type SourceUser struct {
	ID        int64
	Name      string
	Email     string
	Password  string
	Role      string
	Phone     string
	CreatedAt time.Time
}

// TargetUser represents a transformed user record for PostgreSQL.
type TargetUser struct {
	ID           int64
	PublicID     string
	Email        string
	PasswordHash string
	Name         i18n.Text
	Role         string
	Status       string
	Language     string
	Phone        string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// SourceProduct represents a product from MariaDB.
type SourceProduct struct {
	ID          int64
	NameAr      string
	NameEn      string
	Slug        string
	Description string
	CategoryID  int64
	Price       float64
	Stock       int
	VendorOrgID int64
	CreatedAt   time.Time
}

// TargetProduct represents a transformed product for PostgreSQL.
type TargetProduct struct {
	ID                   int64
	PublicID             string
	CategoryID           int64
	Name                 i18n.Text
	Slug                 string
	Description          i18n.Text
	DosageForm           string
	RequiresPrescription bool
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// TargetVariant represents a transformed variant for PostgreSQL.
type TargetVariant struct {
	ID        int64
	PublicID  string
	ProductID int64
	SKU       string
	Price     money.Amount
	Stock     int
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Transformer handles data cleaning, Arabic normalization, and money conversions.
type Transformer struct{}

func NewTransformer() *Transformer {
	return &Transformer{}
}

func (t *Transformer) TransformUser(src *SourceUser) *TargetUser {
	cleanEmail := strings.ToLower(strings.TrimSpace(src.Email))
	normName := arabic.Normalize(src.Name)

	return &TargetUser{
		ID:           src.ID,
		PublicID:     generateUUID(),
		Email:        cleanEmail,
		PasswordHash: src.Password, // Laravel bcrypt hashes ($2y$) are 100% compatible with Go bcrypt
		Name: i18n.Text{
			"ar": normName,
			"en": src.Name,
		},
		Role:      src.Role,
		Status:    "active",
		Language:  "ar",
		Phone:     cleanPhone(src.Phone),
		CreatedAt: src.CreatedAt,
		UpdatedAt: src.CreatedAt,
	}
}

func (t *Transformer) TransformProduct(src *SourceProduct) (*TargetProduct, *TargetVariant) {
	normAr := arabic.Normalize(src.NameAr)
	amount, _ := money.Parse(fmt.Sprintf("%.2f", src.Price))

	p := &TargetProduct{
		ID:         src.ID,
		PublicID:   generateUUID(),
		CategoryID: src.CategoryID,
		Name: i18n.Text{
			"ar": normAr,
			"en": src.NameEn,
		},
		Slug: src.Slug,
		Description: i18n.Text{
			"ar": src.Description,
			"en": src.Description,
		},
		DosageForm:           "tablet",
		RequiresPrescription: false,
		CreatedAt:            src.CreatedAt,
		UpdatedAt:            src.CreatedAt,
	}

	v := &TargetVariant{
		ID:        src.ID,
		PublicID:  generateUUID(),
		ProductID: src.ID,
		SKU:       src.Slug + "-DEFAULT",
		Price:     amount,
		Stock:     src.Stock,
		CreatedAt: src.CreatedAt,
		UpdatedAt: src.CreatedAt,
	}

	return p, v
}

func cleanPhone(phone string) string {
	cleaned := strings.ReplaceAll(phone, " ", "")
	cleaned = strings.ReplaceAll(cleaned, "-", "")
	return cleaned
}

func generateUUID() string {
	return "etl_" + time.Now().Format("20060102150405")
}
