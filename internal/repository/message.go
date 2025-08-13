package repository

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/galihcitta/multi-tenant-messaging-system/internal/models"
)

type MessageRepository struct {
	db     *pgxpool.Pool
	logger *zap.Logger
}

func NewMessageRepository(db *pgxpool.Pool, logger *zap.Logger) *MessageRepository {
	return &MessageRepository{
		db:     db,
		logger: logger,
	}
}

func (r *MessageRepository) Create(ctx context.Context, message *models.Message) error {
	query := `
		INSERT INTO messages (id, tenant_id, payload, created_at)
		VALUES ($1, $2, $3, $4)`

	if message.CreatedAt.IsZero() {
		message.CreatedAt = time.Now()
	}

	_, err := r.db.Exec(ctx, query, message.ID, message.TenantID, message.Payload, message.CreatedAt)
	if err != nil {
		r.logger.Error("Failed to create message", zap.Error(err), zap.String("id", message.ID.String()))
		return fmt.Errorf("failed to create message: %w", err)
	}

	r.logger.Debug("Message created successfully", zap.String("id", message.ID.String()))
	return nil
}

func (r *MessageRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Message, error) {
	query := `SELECT id, tenant_id, payload, created_at FROM messages WHERE id = $1`
	
	var message models.Message
	row := r.db.QueryRow(ctx, query, id)
	
	err := row.Scan(&message.ID, &message.TenantID, &message.Payload, &message.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to get message: %w", err)
	}

	return &message, nil
}

func (r *MessageRepository) List(ctx context.Context, params models.PaginationParams) (*models.MessageListResponse, error) {
	limit := params.Limit
	if limit <= 0 || limit > 100 {
		limit = 20 // Default limit
	}

	var query string
	var args []interface{}

	if params.Cursor == "" {
		// First page
		query = `
			SELECT id, tenant_id, payload, created_at 
			FROM messages 
			ORDER BY created_at DESC 
			LIMIT $1`
		args = []interface{}{limit + 1} // +1 to check if there's a next page
	} else {
		// Parse cursor
		cursor, err := r.parseCursor(params.Cursor)
		if err != nil {
			return nil, fmt.Errorf("invalid cursor: %w", err)
		}

		query = `
			SELECT id, tenant_id, payload, created_at 
			FROM messages 
			WHERE created_at < $1 
			ORDER BY created_at DESC 
			LIMIT $2`
		args = []interface{}{cursor, limit + 1}
	}

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query messages: %w", err)
	}
	defer rows.Close()

	var messages []models.Message
	for rows.Next() {
		var message models.Message
		if err := rows.Scan(&message.ID, &message.TenantID, &message.Payload, &message.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan message: %w", err)
		}
		messages = append(messages, message)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	response := &models.MessageListResponse{
		Data: messages,
	}

	// Check if there's a next page and set cursor
	if len(messages) > limit {
		// Remove the extra message
		response.Data = messages[:limit]
		// Set next cursor to the timestamp of the last message
		lastMessage := messages[limit-1]
		nextCursor := r.createCursor(lastMessage.CreatedAt)
		response.NextCursor = &nextCursor
	}

	return response, nil
}

func (r *MessageRepository) ListByTenant(ctx context.Context, tenantID uuid.UUID, params models.PaginationParams) (*models.MessageListResponse, error) {
	limit := params.Limit
	if limit <= 0 || limit > 100 {
		limit = 20 // Default limit
	}

	var query string
	var args []interface{}

	if params.Cursor == "" {
		// First page
		query = `
			SELECT id, tenant_id, payload, created_at 
			FROM messages 
			WHERE tenant_id = $1 
			ORDER BY created_at DESC 
			LIMIT $2`
		args = []interface{}{tenantID, limit + 1}
	} else {
		// Parse cursor
		cursor, err := r.parseCursor(params.Cursor)
		if err != nil {
			return nil, fmt.Errorf("invalid cursor: %w", err)
		}

		query = `
			SELECT id, tenant_id, payload, created_at 
			FROM messages 
			WHERE tenant_id = $1 AND created_at < $2 
			ORDER BY created_at DESC 
			LIMIT $3`
		args = []interface{}{tenantID, cursor, limit + 1}
	}

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query tenant messages: %w", err)
	}
	defer rows.Close()

	var messages []models.Message
	for rows.Next() {
		var message models.Message
		if err := rows.Scan(&message.ID, &message.TenantID, &message.Payload, &message.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan message: %w", err)
		}
		messages = append(messages, message)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	response := &models.MessageListResponse{
		Data: messages,
	}

	// Check if there's a next page and set cursor
	if len(messages) > limit {
		// Remove the extra message
		response.Data = messages[:limit]
		// Set next cursor to the timestamp of the last message
		lastMessage := messages[limit-1]
		nextCursor := r.createCursor(lastMessage.CreatedAt)
		response.NextCursor = &nextCursor
	}

	return response, nil
}

func (r *MessageRepository) createCursor(timestamp time.Time) string {
	// Use Unix timestamp with nanoseconds for precision
	cursor := strconv.FormatInt(timestamp.UnixNano(), 10)
	return base64.URLEncoding.EncodeToString([]byte(cursor))
}

func (r *MessageRepository) parseCursor(cursor string) (time.Time, error) {
	decoded, err := base64.URLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to decode cursor: %w", err)
	}

	timestampStr := string(decoded)
	timestamp, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse timestamp: %w", err)
	}

	return time.Unix(0, timestamp), nil
}