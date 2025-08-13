package models

import (
	"encoding/json"
	"time"
	"github.com/google/uuid"
)

// PayloadExample represents an example payload structure for Swagger documentation
// @Description Example payload structure (actual payload can be any valid JSON)
type PayloadExample struct {
	Message   string            `json:"message" example:"Hello World"`
	Data      map[string]any    `json:"data,omitempty"`
	Timestamp string            `json:"timestamp,omitempty" example:"2023-01-01T00:00:00Z"`
}

// MessageSwagger represents a message for Swagger documentation
// @Description Message object with structured payload
type MessageSwagger struct {
	ID        uuid.UUID      `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	TenantID  uuid.UUID      `json:"tenant_id" example:"550e8400-e29b-41d4-a716-446655440001"`
	Payload   PayloadExample `json:"payload"`
	CreatedAt time.Time      `json:"created_at" example:"2023-01-01T00:00:00Z"`
}

// CreateMessageRequestSwagger represents a message creation request for Swagger documentation
// @Description Message creation request with structured payload
type CreateMessageRequestSwagger struct {
	TenantID uuid.UUID      `json:"tenant_id" binding:"required" example:"550e8400-e29b-41d4-a716-446655440001"`
	Payload  PayloadExample `json:"payload" binding:"required"`
}

// MessageListResponseSwagger represents a paginated message list for Swagger documentation
// @Description Paginated list of messages with structured payload
type MessageListResponseSwagger struct {
	Data       []MessageSwagger `json:"data"`
	NextCursor *string          `json:"next_cursor,omitempty" example:"next_cursor_token"`
}

type Message struct {
	ID        uuid.UUID       `json:"id" db:"id"`
	TenantID  uuid.UUID       `json:"tenant_id" db:"tenant_id"`
	Payload   json.RawMessage `json:"payload" db:"payload"`
	CreatedAt time.Time       `json:"created_at" db:"created_at"`
}

type CreateMessageRequest struct {
	TenantID uuid.UUID       `json:"tenant_id" binding:"required"`
	Payload  json.RawMessage `json:"payload" binding:"required"`
}

type MessageListResponse struct {
	Data       []Message `json:"data"`
	NextCursor *string   `json:"next_cursor,omitempty"`
}

type PaginationParams struct {
	Cursor string `form:"cursor"`
	Limit  int    `form:"limit"`
}