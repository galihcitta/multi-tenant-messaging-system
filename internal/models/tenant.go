package models

import (
	"time"
	"github.com/google/uuid"
)

type Tenant struct {
	ID        uuid.UUID `json:"id" db:"id"`
	Name      string    `json:"name" db:"name"`
	Workers   int       `json:"workers" db:"workers"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

type CreateTenantRequest struct {
	Name    string `json:"name" binding:"required"`
	Workers int    `json:"workers,omitempty"`
}

type UpdateTenantConcurrencyRequest struct {
	Workers int `json:"workers" binding:"required,min=1"`
}