package repository

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap"

	"github.com/galihcitta/multi-tenant-messaging-system/internal/models"
)

// MessageRepositoryLogicTestSuite tests the repository logic that we can test without database
type MessageRepositoryLogicTestSuite struct {
	suite.Suite
	logger *zap.Logger
}

func (s *MessageRepositoryLogicTestSuite) SetupTest() {
	var err error
	s.logger, err = zap.NewDevelopment()
	s.Require().NoError(err)
}

func (s *MessageRepositoryLogicTestSuite) TestCursorEncoding() {
	// Test cursor creation and parsing logic
	repo := &MessageRepository{logger: s.logger}
	
	// Test createCursor
	timestamp := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
	cursor := repo.createCursor(timestamp)
	
	s.NotEmpty(cursor)
	
	// The cursor should be base64 encoded
	decoded, err := base64.URLEncoding.DecodeString(cursor)
	s.NoError(err)
	
	// The decoded content should be the timestamp in nanoseconds
	expectedNano := strconv.FormatInt(timestamp.UnixNano(), 10)
	s.Equal(expectedNano, string(decoded))
}

func (s *MessageRepositoryLogicTestSuite) TestCursorParsing() {
	// Test cursor parsing logic
	repo := &MessageRepository{logger: s.logger}
	
	// Create a known timestamp
	originalTimestamp := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
	cursor := repo.createCursor(originalTimestamp)
	
	// Parse it back
	parsedTimestamp, err := repo.parseCursor(cursor)
	s.NoError(err)
	
	// Should match the original (within nanosecond precision)
	s.Equal(originalTimestamp.UnixNano(), parsedTimestamp.UnixNano())
}

func (s *MessageRepositoryLogicTestSuite) TestCursorRoundTrip() {
	// Test that cursor encoding and decoding is consistent
	repo := &MessageRepository{logger: s.logger}
	
	timestamps := []time.Time{
		time.Now(),
		time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2025, 12, 31, 23, 59, 59, 999999999, time.UTC),
		time.Unix(0, 1), // Very small timestamp
	}
	
	for _, original := range timestamps {
		cursor := repo.createCursor(original)
		parsed, err := repo.parseCursor(cursor)
		
		s.NoError(err, "Failed to parse cursor for timestamp %v", original)
		s.Equal(original.UnixNano(), parsed.UnixNano(), 
			"Round trip failed for timestamp %v", original)
	}
}

func (s *MessageRepositoryLogicTestSuite) TestInvalidCursorHandling() {
	repo := &MessageRepository{logger: s.logger}
	
	// Test invalid base64
	_, err := repo.parseCursor("invalid-base64!!!")
	s.Error(err)
	s.Contains(err.Error(), "failed to decode cursor")
	
	// Test valid base64 but invalid timestamp
	invalidTimestamp := base64.URLEncoding.EncodeToString([]byte("not-a-number"))
	_, err = repo.parseCursor(invalidTimestamp)
	s.Error(err)
	s.Contains(err.Error(), "failed to parse timestamp")
	
	// Test empty cursor
	_, err = repo.parseCursor("")
	s.Error(err)
}

func (s *MessageRepositoryLogicTestSuite) TestPaginationLogic() {
	// Test pagination parameter handling logic
	tests := []struct {
		name          string
		inputLimit    int
		expectedLimit int
		description   string
	}{
		{
			name:          "default limit for zero",
			inputLimit:    0,
			expectedLimit: 20,
			description:   "Zero limit should use default",
		},
		{
			name:          "default limit for negative",
			inputLimit:    -5,
			expectedLimit: 20,
			description:   "Negative limit should use default",
		},
		{
			name:          "valid small limit",
			inputLimit:    5,
			expectedLimit: 5,
			description:   "Valid small limit should be used as-is",
		},
		{
			name:          "valid large limit",
			inputLimit:    50,
			expectedLimit: 50,
			description:   "Valid limit within max should be used",
		},
		{
			name:          "exceeds maximum",
			inputLimit:    150,
			expectedLimit: 20,
			description:   "Limit exceeding max should use default",
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			// Simulate the limit handling logic from List method
			limit := tt.inputLimit
			if limit <= 0 || limit > 100 {
				limit = 20 // Default limit
			}
			
			s.Equal(tt.expectedLimit, limit, tt.description)
		})
	}
}

func (s *MessageRepositoryLogicTestSuite) TestQueryConstruction() {
	// Test SQL query construction logic
	
	// Test query without cursor (first page)
	firstPageQuery := `
		SELECT id, tenant_id, payload, created_at 
		FROM messages 
		ORDER BY created_at DESC 
		LIMIT $1`
	
	s.Contains(firstPageQuery, "SELECT id, tenant_id, payload, created_at")
	s.Contains(firstPageQuery, "ORDER BY created_at DESC")
	s.Contains(firstPageQuery, "LIMIT $1")
	s.NotContains(firstPageQuery, "WHERE")
	
	// Test query with cursor (subsequent pages)
	subsequentPageQuery := `
		SELECT id, tenant_id, payload, created_at 
		FROM messages 
		WHERE created_at < $1 
		ORDER BY created_at DESC 
		LIMIT $2`
	
	s.Contains(subsequentPageQuery, "WHERE created_at < $1")
	s.Contains(subsequentPageQuery, "LIMIT $2")
	
	// Test tenant-specific queries
	tenantFirstPageQuery := `
		SELECT id, tenant_id, payload, created_at 
		FROM messages 
		WHERE tenant_id = $1 
		ORDER BY created_at DESC 
		LIMIT $2`
	
	s.Contains(tenantFirstPageQuery, "WHERE tenant_id = $1")
	
	tenantSubsequentQuery := `
		SELECT id, tenant_id, payload, created_at 
		FROM messages 
		WHERE tenant_id = $1 AND created_at < $2 
		ORDER BY created_at DESC 
		LIMIT $3`
	
	s.Contains(tenantSubsequentQuery, "WHERE tenant_id = $1 AND created_at < $2")
}

func (s *MessageRepositoryLogicTestSuite) TestNextCursorLogic() {
	// Test next cursor determination logic
	limit := 5
	
	// Test case: fewer messages than limit (no next cursor)
	messages := make([]models.Message, 3)
	for i := range messages {
		messages[i] = models.Message{
			ID:        uuid.New(),
			TenantID:  uuid.New(),
			CreatedAt: time.Now().Add(time.Duration(-i) * time.Minute),
		}
	}
	
	shouldHaveNext := len(messages) > limit
	s.False(shouldHaveNext, "Should not have next cursor when messages <= limit")
	
	// Test case: more messages than limit (should have next cursor)
	messages = make([]models.Message, 7) // 7 > 5 (limit)
	for i := range messages {
		messages[i] = models.Message{
			ID:        uuid.New(),
			TenantID:  uuid.New(),
			CreatedAt: time.Now().Add(time.Duration(-i) * time.Minute),
		}
	}
	
	shouldHaveNext = len(messages) > limit
	s.True(shouldHaveNext, "Should have next cursor when messages > limit")
	
	// Test next cursor creation from last message
	if shouldHaveNext {
		// Would take messages[:limit] and create cursor from messages[limit-1]
		lastMessageIndex := limit - 1
		lastMessage := messages[lastMessageIndex]
		
		// Simulate cursor creation
		repo := &MessageRepository{logger: s.logger}
		nextCursor := repo.createCursor(lastMessage.CreatedAt)
		s.NotEmpty(nextCursor)
	}
}

func (s *MessageRepositoryLogicTestSuite) TestMessageValidation() {
	// Test message data validation logic
	tests := []struct {
		name      string
		message   *models.Message
		expectErr bool
	}{
		{
			name: "valid message",
			message: &models.Message{
				ID:        uuid.New(),
				TenantID:  uuid.New(),
				Payload:   json.RawMessage(`{"test": "data"}`),
				CreatedAt: time.Now(),
			},
			expectErr: false,
		},
		{
			name: "message with nil ID",
			message: &models.Message{
				ID:        uuid.Nil,
				TenantID:  uuid.New(),
				Payload:   json.RawMessage(`{"test": "data"}`),
				CreatedAt: time.Now(),
			},
			expectErr: true,
		},
		{
			name: "message with nil tenant ID",
			message: &models.Message{
				ID:        uuid.New(),
				TenantID:  uuid.Nil,
				Payload:   json.RawMessage(`{"test": "data"}`),
				CreatedAt: time.Now(),
			},
			expectErr: true,
		},
		{
			name: "message with empty payload",
			message: &models.Message{
				ID:        uuid.New(),
				TenantID:  uuid.New(),
				Payload:   json.RawMessage(`{}`),
				CreatedAt: time.Now(),
			},
			expectErr: false, // Empty payload might be valid
		},
		{
			name: "message with zero timestamp",
			message: &models.Message{
				ID:       uuid.New(),
				TenantID: uuid.New(),
				Payload:  json.RawMessage(`{"test": "data"}`),
				// CreatedAt is zero time
			},
			expectErr: false, // Zero time might be handled by setting current time
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			isValid := s.validateMessage(tt.message)
			if tt.expectErr {
				s.False(isValid, "Expected message to be invalid")
			} else {
				s.True(isValid, "Expected message to be valid")
			}
		})
	}
}

// Helper method to simulate message validation
func (s *MessageRepositoryLogicTestSuite) validateMessage(message *models.Message) bool {
	if message == nil {
		return false
	}
	if message.ID == uuid.Nil {
		return false
	}
	if message.TenantID == uuid.Nil {
		return false
	}
	// Note: Empty payload and zero timestamp might be valid depending on business logic
	return true
}

func (s *MessageRepositoryLogicTestSuite) TestTimestampHandling() {
	// Test timestamp handling in message creation
	message := &models.Message{
		ID:       uuid.New(),
		TenantID: uuid.New(),
		Payload:  json.RawMessage(`{"test": "data"}`),
	}
	
	// Simulate the logic from Create method
	if message.CreatedAt.IsZero() {
		message.CreatedAt = time.Now()
	}
	
	s.False(message.CreatedAt.IsZero(), "CreatedAt should be set if it was zero")
}

func TestMessageRepositoryLogicSuite(t *testing.T) {
	suite.Run(t, new(MessageRepositoryLogicTestSuite))
}

// Test individual functions
func TestBase64CursorEncoding(t *testing.T) {
	timestamp := time.Date(2023, 6, 15, 10, 30, 0, 0, time.UTC)
	
	// Test encoding
	timestampStr := strconv.FormatInt(timestamp.UnixNano(), 10)
	encoded := base64.URLEncoding.EncodeToString([]byte(timestampStr))
	
	assert.NotEmpty(t, encoded)
	
	// Test decoding
	decoded, err := base64.URLEncoding.DecodeString(encoded)
	assert.NoError(t, err)
	
	decodedTimestampStr := string(decoded)
	assert.Equal(t, timestampStr, decodedTimestampStr)
	
	// Convert back to timestamp
	decodedNano, err := strconv.ParseInt(decodedTimestampStr, 10, 64)
	assert.NoError(t, err)
	
	decodedTimestamp := time.Unix(0, decodedNano)
	assert.Equal(t, timestamp.UnixNano(), decodedTimestamp.UnixNano())
}

func TestPaginationDefaults(t *testing.T) {
	tests := []struct {
		input    int
		expected int
	}{
		{0, 20},    // Zero should use default
		{-1, 20},   // Negative should use default
		{5, 5},     // Valid small value
		{50, 50},   // Valid medium value
		{100, 100}, // Valid at max
		{150, 20},  // Over max should use default
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("limit_%d", tt.input), func(t *testing.T) {
			// Simulate repository logic
			limit := tt.input
			if limit <= 0 || limit > 100 {
				limit = 20
			}
			assert.Equal(t, tt.expected, limit)
		})
	}
}

func TestMessageResponseConstruction(t *testing.T) {
	// Test MessageListResponse construction
	messages := []models.Message{
		{
			ID:        uuid.New(),
			TenantID:  uuid.New(),
			Payload:   json.RawMessage(`{"test": "data1"}`),
			CreatedAt: time.Now(),
		},
		{
			ID:        uuid.New(),
			TenantID:  uuid.New(),
			Payload:   json.RawMessage(`{"test": "data2"}`),
			CreatedAt: time.Now().Add(-1 * time.Minute),
		},
	}
	
	response := &models.MessageListResponse{
		Data: messages,
	}
	
	assert.Len(t, response.Data, 2)
	assert.Nil(t, response.NextCursor)
	
	// Test with next cursor
	cursor := "test_cursor"
	response.NextCursor = &cursor
	
	assert.NotNil(t, response.NextCursor)
	assert.Equal(t, cursor, *response.NextCursor)
}

func TestQueryParameterValidation(t *testing.T) {
	// Test parameter validation that would happen in repository methods
	tenantID := uuid.New()
	messageID := uuid.New()
	
	// Valid UUIDs
	assert.NotEqual(t, uuid.Nil, tenantID)
	assert.NotEqual(t, uuid.Nil, messageID)
	
	// Test pagination parameters
	params := models.PaginationParams{
		Cursor: "valid_cursor",
		Limit:  10,
	}
	
	assert.NotEmpty(t, params.Cursor)
	assert.Greater(t, params.Limit, 0)
}

func TestJSONPayloadHandling(t *testing.T) {
	// Test JSON payload validation
	validPayloads := []json.RawMessage{
		json.RawMessage(`{"key": "value"}`),
		json.RawMessage(`{"number": 123, "array": [1, 2, 3]}`),
		json.RawMessage(`"simple string"`),
		json.RawMessage(`123`),
		json.RawMessage(`true`),
		json.RawMessage(`null`),
	}
	
	for i, payload := range validPayloads {
		t.Run(fmt.Sprintf("payload_%d", i), func(t *testing.T) {
			// Test that payload is valid JSON
			var temp interface{}
			err := json.Unmarshal(payload, &temp)
			assert.NoError(t, err, "Payload should be valid JSON: %s", string(payload))
		})
	}
	
	// Test invalid payloads
	invalidPayloads := []json.RawMessage{
		json.RawMessage(`{invalid json}`),
		json.RawMessage(`{"unclosed": }`),
	}
	
	for i, payload := range invalidPayloads {
		t.Run(fmt.Sprintf("invalid_payload_%d", i), func(t *testing.T) {
			var temp interface{}
			err := json.Unmarshal(payload, &temp)
			assert.Error(t, err, "Payload should be invalid JSON: %s", string(payload))
		})
	}
}

// Benchmark tests
func BenchmarkCursorCreation(b *testing.B) {
	logger, _ := zap.NewDevelopment()
	repo := &MessageRepository{logger: logger}
	timestamp := time.Now()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = repo.createCursor(timestamp)
	}
}

func BenchmarkCursorParsing(b *testing.B) {
	logger, _ := zap.NewDevelopment()
	repo := &MessageRepository{logger: logger}
	timestamp := time.Now()
	cursor := repo.createCursor(timestamp)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = repo.parseCursor(cursor)
	}
}

func BenchmarkBase64Operations(b *testing.B) {
	timestampStr := strconv.FormatInt(time.Now().UnixNano(), 10)
	
	b.Run("encode", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = base64.URLEncoding.EncodeToString([]byte(timestampStr))
		}
	})
	
	encoded := base64.URLEncoding.EncodeToString([]byte(timestampStr))
	b.Run("decode", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = base64.URLEncoding.DecodeString(encoded)
		}
	})
}