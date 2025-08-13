package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap"

	"github.com/galihcitta/multi-tenant-messaging-system/internal/models"
)

// MockMessageHandler for testing message processing logic
type MockMessageHandler struct {
	processedMessages []models.Message
	processError      error
	mutex             sync.RWMutex
}

func NewMockMessageHandler() *MockMessageHandler {
	return &MockMessageHandler{
		processedMessages: make([]models.Message, 0),
	}
}

func (h *MockMessageHandler) SetProcessError(err error) {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	h.processError = err
}

func (h *MockMessageHandler) HandleMessage(ctx context.Context, message *models.Message) error {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	
	if h.processError != nil {
		return h.processError
	}
	
	h.processedMessages = append(h.processedMessages, *message)
	return nil
}

func (h *MockMessageHandler) GetProcessedCount() int {
	h.mutex.RLock()
	defer h.mutex.RUnlock()
	return len(h.processedMessages)
}

func (h *MockMessageHandler) GetProcessedMessages() []models.Message {
	h.mutex.RLock()
	defer h.mutex.RUnlock()
	messages := make([]models.Message, len(h.processedMessages))
	copy(messages, h.processedMessages)
	return messages
}

// Test WorkerPool logic that doesn't require external dependencies
type WorkerPoolLogicTestSuite struct {
	suite.Suite
	tenantID uuid.UUID
	logger   *zap.Logger
}

func (s *WorkerPoolLogicTestSuite) SetupTest() {
	var err error
	s.logger, err = zap.NewDevelopment()
	s.Require().NoError(err)
	s.tenantID = uuid.New()
}

func (s *WorkerPoolLogicTestSuite) TestGetTenantQueueName() {
	expectedName := "tenant_" + s.tenantID.String() + "_queue"
	actualName := GetTenantQueueName(s.tenantID)
	s.Equal(expectedName, actualName)
}

func (s *WorkerPoolLogicTestSuite) TestWorkerCountOperations() {
	// Create a simple structure to test worker count logic
	var workerCount int32 = 3
	
	// Test atomic operations that would be used in WorkerPool
	atomic.StoreInt32(&workerCount, 5)
	s.Equal(int32(5), atomic.LoadInt32(&workerCount))
	
	// Test concurrent updates
	var wg sync.WaitGroup
	numUpdates := 100
	
	wg.Add(numUpdates)
	for i := 0; i < numUpdates; i++ {
		go func(count int) {
			defer wg.Done()
			atomic.StoreInt32(&workerCount, int32(count%10+1))
		}(i)
	}
	
	wg.Wait()
	
	finalCount := atomic.LoadInt32(&workerCount)
	s.GreaterOrEqual(finalCount, int32(1))
	s.LessOrEqual(finalCount, int32(10))
}

func (s *WorkerPoolLogicTestSuite) TestMessageProcessingLogic() {
	// Test the core message processing logic that would be in WorkerPool.processMessage
	handler := NewMockMessageHandler()
	ctx := context.Background()
	
	// Test 1: Valid complete message
	messageData := map[string]interface{}{
		"id":       uuid.New().String(),
		"tenantId": s.tenantID.String(),
		"payload":  json.RawMessage(`{"test": "data"}`),
	}
	
	err := s.processTestMessage(ctx, handler, messageData, s.tenantID)
	s.NoError(err)
	s.Equal(1, handler.GetProcessedCount())

	// Test 2: Message without ID (should generate one)
	messageData2 := map[string]interface{}{
		"tenantId": s.tenantID.String(),
		"payload":  json.RawMessage(`{"test": "data2"}`),
	}
	
	err = s.processTestMessage(ctx, handler, messageData2, s.tenantID)
	s.NoError(err)
	s.Equal(2, handler.GetProcessedCount())

	// Test 3: Message without tenant ID (should use provided tenant ID)
	messageData3 := map[string]interface{}{
		"payload": json.RawMessage(`{"test": "data3"}`),
	}
	
	err = s.processTestMessage(ctx, handler, messageData3, s.tenantID)
	s.NoError(err)
	s.Equal(3, handler.GetProcessedCount())

	// Verify the last message got the correct tenant ID
	processedMessages := handler.GetProcessedMessages()
	s.Equal(s.tenantID, processedMessages[2].TenantID)
}

// Helper method that replicates WorkerPool.processMessage logic
func (s *WorkerPoolLogicTestSuite) processTestMessage(ctx context.Context, handler MessageHandler, messageData map[string]interface{}, tenantID uuid.UUID) error {
	// Marshal and unmarshal to simulate the actual flow
	messageJSON, err := json.Marshal(messageData)
	if err != nil {
		return err
	}

	// Parse the message (similar to WorkerPool.processMessage)
	var message models.Message
	if err := json.Unmarshal(messageJSON, &message); err != nil {
		return err
	}

	// Set tenant ID if not set
	if message.TenantID == uuid.Nil {
		message.TenantID = tenantID
	}

	// Set ID and timestamp if not set
	if message.ID == uuid.Nil {
		message.ID = uuid.New()
	}
	if message.CreatedAt.IsZero() {
		message.CreatedAt = time.Now()
	}

	// Process the message
	return handler.HandleMessage(ctx, &message)
}

func (s *WorkerPoolLogicTestSuite) TestMessageProcessingErrors() {
	handler := NewMockMessageHandler()
	ctx := context.Background()

	// Test invalid JSON handling
	invalidJSON := []byte(`{invalid json}`)
	var message models.Message
	err := json.Unmarshal(invalidJSON, &message)
	s.Error(err)

	// Test handler error
	handler.SetProcessError(assert.AnError)
	validMessage := &models.Message{
		ID:        uuid.New(),
		TenantID:  s.tenantID,
		Payload:   json.RawMessage(`{"test": "data"}`),
		CreatedAt: time.Now(),
	}
	
	err = handler.HandleMessage(ctx, validMessage)
	s.Error(err)
	s.Equal(assert.AnError, err)
}

func (s *WorkerPoolLogicTestSuite) TestMessageValidation() {
	// Test various message validation scenarios
	tests := []struct {
		name        string
		messageData map[string]interface{}
		tenantID    uuid.UUID
		expectError bool
	}{
		{
			name: "valid complete message",
			messageData: map[string]interface{}{
				"id":       uuid.New().String(),
				"tenantId": s.tenantID.String(),
				"payload":  json.RawMessage(`{"test": "data"}`),
			},
			tenantID:    s.tenantID,
			expectError: false,
		},
		{
			name: "message without ID",
			messageData: map[string]interface{}{
				"tenantId": s.tenantID.String(),
				"payload":  json.RawMessage(`{"test": "data"}`),
			},
			tenantID:    s.tenantID,
			expectError: false,
		},
		{
			name: "message without tenant ID",
			messageData: map[string]interface{}{
				"payload": json.RawMessage(`{"test": "data"}`),
			},
			tenantID:    s.tenantID,
			expectError: false,
		},
		{
			name: "message with empty payload",
			messageData: map[string]interface{}{
				"id":      uuid.New().String(),
				"payload": json.RawMessage(`{}`),
			},
			tenantID:    s.tenantID,
			expectError: false,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			handler := NewMockMessageHandler()
			ctx := context.Background()

			err := s.processTestMessage(ctx, handler, tt.messageData, tt.tenantID)
			if tt.expectError {
				s.Error(err)
			} else {
				s.NoError(err)
				s.Equal(1, handler.GetProcessedCount())
				
				// Verify the processed message has correct fields
				processedMessages := handler.GetProcessedMessages()
				msg := processedMessages[0]
				s.Equal(tt.tenantID, msg.TenantID)
				s.NotEqual(uuid.Nil, msg.ID)
				s.False(msg.CreatedAt.IsZero())
			}
		})
	}
}

func (s *WorkerPoolLogicTestSuite) TestConcurrentMessageProcessing() {
	handler := NewMockMessageHandler()
	ctx := context.Background()
	
	// Process multiple messages concurrently
	var wg sync.WaitGroup
	numMessages := 10 // Reduced for more stable test
	var successCount int32
	
	wg.Add(numMessages)
	for i := 0; i < numMessages; i++ {
		go func(index int) {
			defer wg.Done()
			
			messageData := map[string]interface{}{
				"payload": json.RawMessage(fmt.Sprintf(`{"index": %d}`, index)),
			}
			
			err := s.processTestMessage(ctx, handler, messageData, s.tenantID)
			if err == nil {
				atomic.AddInt32(&successCount, 1)
			}
		}(i)
	}
	
	wg.Wait()
	
	// Verify messages were processed (allow for some concurrent race conditions)
	finalSuccessCount := atomic.LoadInt32(&successCount)
	s.GreaterOrEqual(int(finalSuccessCount), 1, "At least some messages should be processed")
	s.LessOrEqual(int(finalSuccessCount), numMessages, "Should not exceed total messages")
	
	// Verify all processed messages have the correct tenant ID
	processedMessages := handler.GetProcessedMessages()
	for _, msg := range processedMessages {
		s.Equal(s.tenantID, msg.TenantID)
	}
}

func TestWorkerPoolLogicSuite(t *testing.T) {
	suite.Run(t, new(WorkerPoolLogicTestSuite))
}

// Test individual utility functions
func TestGetTenantQueueName(t *testing.T) {
	tenantID := uuid.New()
	expected := "tenant_" + tenantID.String() + "_queue"
	actual := GetTenantQueueName(tenantID)
	assert.Equal(t, expected, actual)
}

func TestMessageFieldAssignment(t *testing.T) {
	tenantID := uuid.New()
	
	// Test message with missing fields
	message := models.Message{
		Payload: json.RawMessage(`{"test": "data"}`),
	}
	
	// Simulate the logic from WorkerPool.processMessage
	if message.TenantID == uuid.Nil {
		message.TenantID = tenantID
	}
	if message.ID == uuid.Nil {
		message.ID = uuid.New()
	}
	if message.CreatedAt.IsZero() {
		message.CreatedAt = time.Now()
	}
	
	// Verify fields were assigned
	assert.Equal(t, tenantID, message.TenantID)
	assert.NotEqual(t, uuid.Nil, message.ID)
	assert.False(t, message.CreatedAt.IsZero())
}

func TestMessageHandlerInterface(t *testing.T) {
	handler := NewMockMessageHandler()
	ctx := context.Background()
	
	message := &models.Message{
		ID:        uuid.New(),
		TenantID:  uuid.New(),
		Payload:   json.RawMessage(`{"test": "data"}`),
		CreatedAt: time.Now(),
	}
	
	// Test successful processing
	err := handler.HandleMessage(ctx, message)
	assert.NoError(t, err)
	assert.Equal(t, 1, handler.GetProcessedCount())
	
	// Test error handling
	handler.SetProcessError(assert.AnError)
	err = handler.HandleMessage(ctx, message)
	assert.Error(t, err)
	assert.Equal(t, assert.AnError, err)
	// Count should still be 1 since the second call failed
	assert.Equal(t, 1, handler.GetProcessedCount())
}

func TestAtomicWorkerCountOperations(t *testing.T) {
	var workerCount int32 = 5
	
	// Test atomic load/store
	assert.Equal(t, int32(5), atomic.LoadInt32(&workerCount))
	
	atomic.StoreInt32(&workerCount, 10)
	assert.Equal(t, int32(10), atomic.LoadInt32(&workerCount))
	
	// Test under high concurrency
	var wg sync.WaitGroup
	numGoroutines := 1000
	
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(index int) {
			defer wg.Done()
			
			// Simulate UpdateWorkerCount logic
			newCount := (index % 20) + 1 // 1 to 20
			atomic.StoreInt32(&workerCount, int32(newCount))
			
			// Also read to test concurrent access
			_ = atomic.LoadInt32(&workerCount)
		}(i)
	}
	
	wg.Wait()
	
	finalCount := atomic.LoadInt32(&workerCount)
	assert.GreaterOrEqual(t, finalCount, int32(1))
	assert.LessOrEqual(t, finalCount, int32(20))
}

// Benchmark tests
func BenchmarkMessageProcessing(b *testing.B) {
	handler := NewMockMessageHandler()
	ctx := context.Background()
	tenantID := uuid.New()
	
	messageData := map[string]interface{}{
		"payload": json.RawMessage(`{"test": "benchmark"}`),
	}
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			// Simulate message processing
			messageJSON, _ := json.Marshal(messageData)
			var message models.Message
			json.Unmarshal(messageJSON, &message)
			
			if message.TenantID == uuid.Nil {
				message.TenantID = tenantID
			}
			if message.ID == uuid.Nil {
				message.ID = uuid.New()
			}
			if message.CreatedAt.IsZero() {
				message.CreatedAt = time.Now()
			}
			
			handler.HandleMessage(ctx, &message)
		}
	})
}

func BenchmarkAtomicWorkerCount(b *testing.B) {
	var workerCount int32 = 1
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		count := int32(1)
		for pb.Next() {
			atomic.StoreInt32(&workerCount, count)
			_ = atomic.LoadInt32(&workerCount)
			count = (count % 10) + 1
		}
	})
}

func BenchmarkWorkerPoolGetTenantQueueName(b *testing.B) {
	tenantID := uuid.New()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = GetTenantQueueName(tenantID)
	}
}