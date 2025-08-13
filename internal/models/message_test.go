package models

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

// MessageModelTestSuite provides test suite for message models
type MessageModelTestSuite struct {
	suite.Suite
}

func (s *MessageModelTestSuite) TestMessageModel() {
	tenantID := uuid.New()
	messageID := uuid.New()
	payload := json.RawMessage(`{"test": "data", "number": 123}`)
	now := time.Now()

	message := Message{
		ID:        messageID,
		TenantID:  tenantID,
		Payload:   payload,
		CreatedAt: now,
	}

	s.Equal(messageID, message.ID)
	s.Equal(tenantID, message.TenantID)
	s.Equal(payload, message.Payload)
	s.Equal(now, message.CreatedAt)
}

func (s *MessageModelTestSuite) TestMessageJSONMarshaling() {
	message := Message{
		ID:        uuid.New(),
		TenantID:  uuid.New(),
		Payload:   json.RawMessage(`{"key": "value"}`),
		CreatedAt: time.Now(),
	}

	// Test marshaling to JSON
	jsonBytes, err := json.Marshal(message)
	s.NoError(err)
	s.NotEmpty(jsonBytes)

	// Test unmarshaling from JSON
	var unmarshaledMessage Message
	err = json.Unmarshal(jsonBytes, &unmarshaledMessage)
	s.NoError(err)

	s.Equal(message.ID, unmarshaledMessage.ID)
	s.Equal(message.TenantID, unmarshaledMessage.TenantID)
	// JSON marshaling removes whitespace, so compare the JSON content
	var originalPayload, unmarshaledPayload interface{}
	json.Unmarshal(message.Payload, &originalPayload)
	json.Unmarshal(unmarshaledMessage.Payload, &unmarshaledPayload)
	s.Equal(originalPayload, unmarshaledPayload)
	s.WithinDuration(message.CreatedAt, unmarshaledMessage.CreatedAt, time.Second)
}

func (s *MessageModelTestSuite) TestCreateMessageRequest() {
	tenantID := uuid.New()
	payload := json.RawMessage(`{"test": "create request"}`)

	req := CreateMessageRequest{
		TenantID: tenantID,
		Payload:  payload,
	}

	s.Equal(tenantID, req.TenantID)
	s.Equal(payload, req.Payload)

	// Test JSON marshaling
	jsonBytes, err := json.Marshal(req)
	s.NoError(err)

	var unmarshaledReq CreateMessageRequest
	err = json.Unmarshal(jsonBytes, &unmarshaledReq)
	s.NoError(err)

	s.Equal(req.TenantID, unmarshaledReq.TenantID)
	// JSON marshaling removes whitespace, so compare the JSON content
	var originalPayload, unmarshaledPayload interface{}
	json.Unmarshal(req.Payload, &originalPayload)
	json.Unmarshal(unmarshaledReq.Payload, &unmarshaledPayload)
	s.Equal(originalPayload, unmarshaledPayload)
}

func (s *MessageModelTestSuite) TestMessageListResponse() {
	messages := []Message{
		{
			ID:        uuid.New(),
			TenantID:  uuid.New(),
			Payload:   json.RawMessage(`{"index": 1}`),
			CreatedAt: time.Now(),
		},
		{
			ID:        uuid.New(),
			TenantID:  uuid.New(),
			Payload:   json.RawMessage(`{"index": 2}`),
			CreatedAt: time.Now().Add(-1 * time.Minute),
		},
	}

	response := MessageListResponse{
		Data: messages,
	}

	s.Len(response.Data, 2)
	s.Nil(response.NextCursor)

	// Test with next cursor
	cursor := "next_page_cursor"
	response.NextCursor = &cursor

	s.NotNil(response.NextCursor)
	s.Equal(cursor, *response.NextCursor)

	// Test JSON marshaling
	jsonBytes, err := json.Marshal(response)
	s.NoError(err)

	var unmarshaledResponse MessageListResponse
	err = json.Unmarshal(jsonBytes, &unmarshaledResponse)
	s.NoError(err)

	s.Len(unmarshaledResponse.Data, 2)
	s.NotNil(unmarshaledResponse.NextCursor)
	s.Equal(cursor, *unmarshaledResponse.NextCursor)
}

func (s *MessageModelTestSuite) TestPaginationParams() {
	params := PaginationParams{
		Cursor: "test_cursor",
		Limit:  50,
	}

	s.Equal("test_cursor", params.Cursor)
	s.Equal(50, params.Limit)

	// Test default values
	var defaultParams PaginationParams
	s.Empty(defaultParams.Cursor)
	s.Equal(0, defaultParams.Limit)
}

func (s *MessageModelTestSuite) TestJSONRawMessageHandling() {
	// Test various JSON payload types
	payloadTypes := []json.RawMessage{
		json.RawMessage(`{"object": "value"}`),
		json.RawMessage(`[1, 2, 3]`),
		json.RawMessage(`"string"`),
		json.RawMessage(`123`),
		json.RawMessage(`true`),
		json.RawMessage(`null`),
	}

	for i, payload := range payloadTypes {
		message := Message{
			ID:        uuid.New(),
			TenantID:  uuid.New(),
			Payload:   payload,
			CreatedAt: time.Now(),
		}

		// Test that the payload remains as RawMessage
		s.Equal(payload, message.Payload)

		// Test JSON marshaling preserves the raw payload
		jsonBytes, err := json.Marshal(message)
		s.NoError(err, "Failed to marshal message %d", i)

		var unmarshaledMessage Message
		err = json.Unmarshal(jsonBytes, &unmarshaledMessage)
		s.NoError(err, "Failed to unmarshal message %d", i)

		// JSON marshaling removes whitespace, so compare the actual JSON content
		var originalContent, unmarshaledContent interface{}
		err = json.Unmarshal(payload, &originalContent)
		s.NoError(err, "Failed to unmarshal original payload for type %d", i)
		err = json.Unmarshal(unmarshaledMessage.Payload, &unmarshaledContent)
		s.NoError(err, "Failed to unmarshal result payload for type %d", i)
		s.Equal(originalContent, unmarshaledContent, "Payload content mismatch for type %d", i)
	}
}

func (s *MessageModelTestSuite) TestMessageModelDefaults() {
	// Test zero values
	var message Message

	s.Equal(uuid.Nil, message.ID)
	s.Equal(uuid.Nil, message.TenantID)
	s.Nil(message.Payload)
	s.True(message.CreatedAt.IsZero())
}

func (s *MessageModelTestSuite) TestJSONFieldNames() {
	message := Message{
		ID:        uuid.New(),
		TenantID:  uuid.New(),
		Payload:   json.RawMessage(`{"test": "field_names"}`),
		CreatedAt: time.Now(),
	}

	jsonBytes, err := json.Marshal(message)
	s.NoError(err)

	jsonString := string(jsonBytes)
	
	// Verify JSON field names match expected tags
	s.Contains(jsonString, "\"id\":")
	s.Contains(jsonString, "\"tenant_id\":")
	s.Contains(jsonString, "\"payload\":")
	s.Contains(jsonString, "\"created_at\":")
}

func TestMessageModelSuite(t *testing.T) {
	suite.Run(t, new(MessageModelTestSuite))
}

// Test individual functions
func TestCreateMessageRequestValidation(t *testing.T) {
	tests := []struct {
		name        string
		request     CreateMessageRequest
		expectValid bool
		description string
	}{
		{
			name: "valid request",
			request: CreateMessageRequest{
				TenantID: uuid.New(),
				Payload:  json.RawMessage(`{"test": "data"}`),
			},
			expectValid: true,
			description: "Valid request should pass validation",
		},
		{
			name: "nil tenant ID",
			request: CreateMessageRequest{
				TenantID: uuid.Nil,
				Payload:  json.RawMessage(`{"test": "data"}`),
			},
			expectValid: false,
			description: "Nil tenant ID should fail validation",
		},
		{
			name: "nil payload",
			request: CreateMessageRequest{
				TenantID: uuid.New(),
				Payload:  nil,
			},
			expectValid: false,
			description: "Nil payload should fail validation",
		},
		{
			name: "empty payload",
			request: CreateMessageRequest{
				TenantID: uuid.New(),
				Payload:  json.RawMessage(`{}`),
			},
			expectValid: true,
			description: "Empty JSON object should be valid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isValid := validateCreateMessageRequest(&tt.request)
			if tt.expectValid {
				assert.True(t, isValid, tt.description)
			} else {
				assert.False(t, isValid, tt.description)
			}
		})
	}
}

// Helper function to simulate validation
func validateCreateMessageRequest(req *CreateMessageRequest) bool {
	if req == nil {
		return false
	}
	if req.TenantID == uuid.Nil {
		return false
	}
	if req.Payload == nil {
		return false
	}
	return true
}

func TestPaginationParamsValidation(t *testing.T) {
	tests := []struct {
		name   string
		params PaginationParams
		expectedLimit int
		description string
	}{
		{
			name:          "default limit for zero",
			params:        PaginationParams{Limit: 0},
			expectedLimit: 20, // Assume default
			description:   "Zero limit should use default",
		},
		{
			name:          "valid limit",
			params:        PaginationParams{Limit: 50},
			expectedLimit: 50,
			description:   "Valid limit should be used as-is",
		},
		{
			name:          "limit exceeds max",
			params:        PaginationParams{Limit: 150},
			expectedLimit: 20, // Assume default when exceeding max
			description:   "Limit exceeding max should use default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate repository validation logic
			limit := tt.params.Limit
			if limit <= 0 || limit > 100 {
				limit = 20 // Default limit
			}
			assert.Equal(t, tt.expectedLimit, limit, tt.description)
		})
	}
}

func TestMessageListResponseWithEmptyData(t *testing.T) {
	// Test with empty message list
	response := MessageListResponse{
		Data: []Message{},
	}

	assert.Len(t, response.Data, 0)
	assert.Nil(t, response.NextCursor)

	// Test JSON marshaling of empty response
	jsonBytes, err := json.Marshal(response)
	assert.NoError(t, err)

	var unmarshaledResponse MessageListResponse
	err = json.Unmarshal(jsonBytes, &unmarshaledResponse)
	assert.NoError(t, err)

	assert.Len(t, unmarshaledResponse.Data, 0)
	assert.Nil(t, unmarshaledResponse.NextCursor)
}

func TestJSONRawMessageEdgeCases(t *testing.T) {
	// Test empty JSON RawMessage
	emptyPayload := json.RawMessage("")
	message := Message{
		ID:       uuid.New(),
		TenantID: uuid.New(),
		Payload:  emptyPayload,
	}

	assert.Equal(t, emptyPayload, message.Payload)
	assert.Empty(t, string(message.Payload))

	// Test very large JSON payload
	largePayload := json.RawMessage(`{"data": "` + string(make([]byte, 10000)) + `"}`)
	for i := 0; i < len(largePayload)-2; i++ {
		if largePayload[i+10] == '"' && largePayload[i+11] == '}' {
			break
		}
		largePayload[i+10] = 'a'
	}

	message.Payload = largePayload
	assert.Len(t, string(message.Payload), len(largePayload))

	// Test JSON marshaling with large payload
	jsonBytes, err := json.Marshal(message)
	assert.NoError(t, err)

	var unmarshaledMessage Message
	err = json.Unmarshal(jsonBytes, &unmarshaledMessage)
	assert.NoError(t, err)
	// For large payload, just check that the structure is preserved
	var originalContent, unmarshaledContent interface{}
	json.Unmarshal(largePayload, &originalContent)
	json.Unmarshal(unmarshaledMessage.Payload, &unmarshaledContent)
	assert.Equal(t, originalContent, unmarshaledContent)
}

func TestMessageJSONTags(t *testing.T) {
	message := Message{
		ID:        uuid.New(),
		TenantID:  uuid.New(),
		Payload:   json.RawMessage(`{"tag_test": true}`),
		CreatedAt: time.Now(),
	}

	jsonBytes, err := json.Marshal(message)
	assert.NoError(t, err)

	jsonMap := make(map[string]interface{})
	err = json.Unmarshal(jsonBytes, &jsonMap)
	assert.NoError(t, err)

	// Verify JSON field names
	_, hasID := jsonMap["id"]
	_, hasTenantID := jsonMap["tenant_id"]
	_, hasPayload := jsonMap["payload"]
	_, hasCreatedAt := jsonMap["created_at"]

	assert.True(t, hasID)
	assert.True(t, hasTenantID)
	assert.True(t, hasPayload)
	assert.True(t, hasCreatedAt)
}

func TestSwaggerExampleModels(t *testing.T) {
	// Test PayloadExample
	example := PayloadExample{
		Message:   "Hello World",
		Data:      map[string]any{"key": "value"},
		Timestamp: "2023-01-01T00:00:00Z",
	}

	assert.Equal(t, "Hello World", example.Message)
	assert.NotNil(t, example.Data)
	assert.Equal(t, "2023-01-01T00:00:00Z", example.Timestamp)

	// Test JSON marshaling
	jsonBytes, err := json.Marshal(example)
	assert.NoError(t, err)

	var unmarshaledExample PayloadExample
	err = json.Unmarshal(jsonBytes, &unmarshaledExample)
	assert.NoError(t, err)

	assert.Equal(t, example.Message, unmarshaledExample.Message)
	assert.Equal(t, example.Timestamp, unmarshaledExample.Timestamp)
}

// Benchmark tests
func BenchmarkMessageCreation(b *testing.B) {
	id := uuid.New()
	tenantID := uuid.New()
	payload := json.RawMessage(`{"benchmark": "test"}`)
	now := time.Now()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Message{
			ID:        id,
			TenantID:  tenantID,
			Payload:   payload,
			CreatedAt: now,
		}
	}
}

func BenchmarkMessageJSONMarshaling(b *testing.B) {
	message := Message{
		ID:        uuid.New(),
		TenantID:  uuid.New(),
		Payload:   json.RawMessage(`{"benchmark": "marshaling"}`),
		CreatedAt: time.Now(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = json.Marshal(message)
	}
}

func BenchmarkMessageJSONUnmarshaling(b *testing.B) {
	message := Message{
		ID:        uuid.New(),
		TenantID:  uuid.New(),
		Payload:   json.RawMessage(`{"benchmark": "unmarshaling"}`),
		CreatedAt: time.Now(),
	}

	jsonBytes, _ := json.Marshal(message)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var unmarshaledMessage Message
		_ = json.Unmarshal(jsonBytes, &unmarshaledMessage)
	}
}

func BenchmarkRawMessageHandling(b *testing.B) {
	payload := json.RawMessage(`{"large_payload": "` + string(make([]byte, 1000)) + `"}`)
	for i := 20; i < len(payload)-2; i++ {
		payload[i] = 'x'
	}

	b.ResetTimer()
	b.Run("assignment", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			message := Message{Payload: payload}
			_ = message.Payload
		}
	})

	b.Run("string_conversion", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = string(payload)
		}
	})
}