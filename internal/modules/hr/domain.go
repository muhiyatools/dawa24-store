// Package hr manages employee profiles, salary compensation, and organizational work schedules.
package hr

import (
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// Employee represents a staff member within an organization.
type Employee struct {
	ID             int64        `json:"id"`
	PublicID       string       `json:"public_id"`
	OrganizationID int64        `json:"organization_id"`
	UserID         int64        `json:"user_id"`
	EmployeeCode   string       `json:"employee_code"`
	JobTitle       string       `json:"job_title"`
	BaseSalary     money.Amount `json:"base_salary"`
	VariableSalary money.Amount `json:"variable_salary"`
	Status         string       `json:"status"`
	HiredAt        *time.Time   `json:"hired_at,omitempty"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
}

// WorkTime represents recurring branch business hours.
type WorkTime struct {
	ID             int64     `json:"id"`
	PublicID       string    `json:"public_id"`
	OrganizationID int64     `json:"organization_id"`
	DayNameAr      string    `json:"day_name_ar"`
	DayNameEn      string    `json:"day_name_en"`
	OpenTime       string    `json:"open_time,omitempty"`
	CloseTime      string    `json:"close_time,omitempty"`
	IsClosed       bool      `json:"is_closed"`
	SortOrder      int       `json:"sort_order"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Validate ensures employee profile data is valid.
func (e *Employee) Validate() error {
	if e.UserID <= 0 {
		return apperr.Validation("employee.user_required", "User ID is required.", nil)
	}
	if e.EmployeeCode == "" {
		return apperr.Validation("employee.code_required", "Employee code is required.", nil)
	}
	if e.JobTitle == "" {
		return apperr.Validation("employee.title_required", "Job title is required.", nil)
	}
	return nil
}
