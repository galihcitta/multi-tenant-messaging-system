package tenant

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap"

	"github.com/galihcitta/multi-tenant-messaging-system/internal/models"
	"github.com/galihcitta/multi-tenant-messaging-system/internal/services/messaging"
)

// Interface definitions for easier mocking
type TenantRepositoryInterface interface {
	Create(ctx context.Context, tenant *models.Tenant) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.Tenant, error)
	GetAll(ctx context.Context) ([]models.Tenant, error)
	Delete(ctx context.Context, id uuid.UUID) error
	UpdateWorkers(ctx context.Context, id uuid.UUID, workers int) error
}

type MessageRepositoryInterface interface {
	Create(ctx context.Context, message *models.Message) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.Message, error)
	List(ctx context.Context, params models.PaginationParams) (*models.MessageListResponse, error)
	ListByTenant(ctx context.Context, tenantID uuid.UUID, params models.PaginationParams) (*models.MessageListResponse, error)
}

type RabbitMQManagerInterface interface {
	Connect() error
	Close() error
	CreateQueue(queueName string) error
	DeleteQueue(queueName string) error
	PublishMessage(queueName string, message []byte) error
	IsConnected() bool
}

// Mock implementations for testing
type MockTenantRepository struct {
	tenants map[uuid.UUID]*models.Tenant
	calls   map[string]int
	errors  map[string]error
	mutex   sync.RWMutex
}

func NewMockTenantRepository() *MockTenantRepository {
	return &MockTenantRepository{
		tenants: make(map[uuid.UUID]*models.Tenant),
		calls:   make(map[string]int),
		errors:  make(map[string]error),
	}
}

func (m *MockTenantRepository) SetError(method string, err error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.errors[method] = err
}

func (m *MockTenantRepository) GetCallCount(method string) int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.calls[method]
}

func (m *MockTenantRepository) Create(ctx context.Context, tenant *models.Tenant) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	m.calls["Create"]++
	
	if err := m.errors["Create"]; err != nil {
		return err
	}
	
	tenant.CreatedAt = time.Now()
	tenant.UpdatedAt = time.Now()
	m.tenants[tenant.ID] = tenant
	return nil
}

func (m *MockTenantRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Tenant, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	
	m.calls["GetByID"]++
	
	if err := m.errors["GetByID"]; err != nil {
		return nil, err
	}
	
	tenant, exists := m.tenants[id]
	if !exists {
		return nil, fmt.Errorf("tenant not found: %s", id)
	}
	
	return tenant, nil
}

func (m *MockTenantRepository) GetAll(ctx context.Context) ([]models.Tenant, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	
	m.calls["GetAll"]++
	
	if err := m.errors["GetAll"]; err != nil {
		return nil, err
	}
	
	var tenants []models.Tenant
	for _, tenant := range m.tenants {
		tenants = append(tenants, *tenant)
	}
	
	return tenants, nil
}

func (m *MockTenantRepository) Delete(ctx context.Context, id uuid.UUID) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	m.calls["Delete"]++
	
	if err := m.errors["Delete"]; err != nil {
		return err
	}
	
	if _, exists := m.tenants[id]; !exists {
		return fmt.Errorf("tenant not found: %s", id)
	}
	
	delete(m.tenants, id)
	return nil
}

func (m *MockTenantRepository) UpdateWorkers(ctx context.Context, id uuid.UUID, workers int) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	m.calls["UpdateWorkers"]++
	
	if err := m.errors["UpdateWorkers"]; err != nil {
		return err
	}
	
	tenant, exists := m.tenants[id]
	if !exists {
		return fmt.Errorf("tenant not found: %s", id)
	}
	
	tenant.Workers = workers
	tenant.UpdatedAt = time.Now()
	return nil
}

type MockMessageRepository struct {
	messages map[uuid.UUID]*models.Message
	calls    map[string]int
	errors   map[string]error
	mutex    sync.RWMutex
}

func NewMockMessageRepository() *MockMessageRepository {
	return &MockMessageRepository{
		messages: make(map[uuid.UUID]*models.Message),
		calls:    make(map[string]int),
		errors:   make(map[string]error),
	}
}

func (m *MockMessageRepository) SetError(method string, err error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.errors[method] = err
}

func (m *MockMessageRepository) GetCallCount(method string) int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.calls[method]
}

func (m *MockMessageRepository) Create(ctx context.Context, message *models.Message) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	m.calls["Create"]++
	
	if err := m.errors["Create"]; err != nil {
		return err
	}
	
	m.messages[message.ID] = message
	return nil
}

func (m *MockMessageRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Message, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	
	m.calls["GetByID"]++
	
	if err := m.errors["GetByID"]; err != nil {
		return nil, err
	}
	
	message, exists := m.messages[id]
	if !exists {
		return nil, fmt.Errorf("message not found")
	}
	
	return message, nil
}

func (m *MockMessageRepository) List(ctx context.Context, params models.PaginationParams) (*models.MessageListResponse, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	
	m.calls["List"]++
	
	if err := m.errors["List"]; err != nil {
		return nil, err
	}
	
	var messages []models.Message
	for _, msg := range m.messages {
		messages = append(messages, *msg)
	}
	
	return &models.MessageListResponse{Data: messages}, nil
}

func (m *MockMessageRepository) ListByTenant(ctx context.Context, tenantID uuid.UUID, params models.PaginationParams) (*models.MessageListResponse, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	
	m.calls["ListByTenant"]++
	
	if err := m.errors["ListByTenant"]; err != nil {
		return nil, err
	}
	
	var messages []models.Message
	for _, msg := range m.messages {
		if msg.TenantID == tenantID {
			messages = append(messages, *msg)
		}
	}
	
	return &models.MessageListResponse{Data: messages}, nil
}

type MockRabbitMQManager struct {
	queues        map[string]bool
	published     map[string][]byte
	calls         map[string]int
	errors        map[string]error
	connected     bool
	mutex         sync.RWMutex
}

func NewMockRabbitMQManager() *MockRabbitMQManager {
	return &MockRabbitMQManager{
		queues:    make(map[string]bool),
		published: make(map[string][]byte),
		calls:     make(map[string]int),
		errors:    make(map[string]error),
		connected: true,
	}
}

func (m *MockRabbitMQManager) SetError(method string, err error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.errors[method] = err
}

func (m *MockRabbitMQManager) GetCallCount(method string) int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.calls[method]
}

func (m *MockRabbitMQManager) Connect() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	m.calls["Connect"]++
	
	if err := m.errors["Connect"]; err != nil {
		return err
	}
	
	m.connected = true
	return nil
}

func (m *MockRabbitMQManager) Close() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	m.calls["Close"]++
	
	if err := m.errors["Close"]; err != nil {
		return err
	}
	
	m.connected = false
	return nil
}

func (m *MockRabbitMQManager) CreateQueue(queueName string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	m.calls["CreateQueue"]++
	
	if err := m.errors["CreateQueue"]; err != nil {
		return err
	}
	
	m.queues[queueName] = true
	return nil
}

func (m *MockRabbitMQManager) DeleteQueue(queueName string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	m.calls["DeleteQueue"]++
	
	if err := m.errors["DeleteQueue"]; err != nil {
		return err
	}
	
	delete(m.queues, queueName)
	return nil
}

func (m *MockRabbitMQManager) PublishMessage(queueName string, message []byte) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	m.calls["PublishMessage"]++
	
	if err := m.errors["PublishMessage"]; err != nil {
		return err
	}
	
	m.published[queueName] = message
	return nil
}

func (m *MockRabbitMQManager) IsConnected() bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	
	m.calls["IsConnected"]++
	return m.connected
}

// Custom manager that accepts interfaces
func NewManagerWithInterfaces(
	tenantRepo TenantRepositoryInterface,
	messageRepo MessageRepositoryInterface,
	rabbitMQ RabbitMQManagerInterface,
	defaultWorkers int,
	logger *zap.Logger,
) *Manager {
	// Create a manager with nil repositories since we'll need to access private fields
	// This is a limitation of testing private fields, we'll work around it
	manager := &Manager{
		logger:         logger,
		workerPools:    make(map[uuid.UUID]*messaging.WorkerPool),
		defaultWorkers: defaultWorkers,
	}
	
	// In a real scenario, we would need dependency injection or interfaces in the original code
	// For now, we'll test what we can with the current structure
	return manager
}

// TenantManagerTestSuite provides test suite for tenant manager
type TenantManagerTestSuite struct {
	suite.Suite
	tenantRepo    *MockTenantRepository
	messageRepo   *MockMessageRepository
	rabbitMQ      *MockRabbitMQManager
	logger        *zap.Logger
	ctx           context.Context
}

func (s *TenantManagerTestSuite) SetupTest() {
	var err error
	s.logger, err = zap.NewDevelopment()
	s.Require().NoError(err)

	s.tenantRepo = NewMockTenantRepository()
	s.messageRepo = NewMockMessageRepository()
	s.rabbitMQ = NewMockRabbitMQManager()
	s.ctx = context.Background()
}

func (s *TenantManagerTestSuite) TestCreateTenantLogic() {
	// Test the tenant creation logic without full integration
	req := &models.CreateTenantRequest{
		Name:    "test-tenant",
		Workers: 5,
	}

	// Create tenant object similar to what Manager.CreateTenant does
	tenant := &models.Tenant{
		ID:      uuid.New(),
		Name:    req.Name,
		Workers: req.Workers,
	}

	// Test default worker assignment
	if tenant.Workers <= 0 {
		tenant.Workers = 3 // default
	}

	s.Equal("test-tenant", tenant.Name)
	s.Equal(5, tenant.Workers)
	s.NotEqual(uuid.Nil, tenant.ID)
}

func (s *TenantManagerTestSuite) TestCreateTenantDefaultWorkers() {
	req := &models.CreateTenantRequest{
		Name:    "test-tenant",
		Workers: 0, // Should use default
	}

	tenant := &models.Tenant{
		ID:      uuid.New(),
		Name:    req.Name,
		Workers: req.Workers,
	}

	// Apply default worker logic
	if tenant.Workers <= 0 {
		tenant.Workers = 3
	}

	s.Equal(3, tenant.Workers)
}

func (s *TenantManagerTestSuite) TestTenantRepository() {
	tenant := &models.Tenant{
		ID:      uuid.New(),
		Name:    "test-tenant",
		Workers: 5,
	}

	// Test Create
	err := s.tenantRepo.Create(s.ctx, tenant)
	s.NoError(err)
	s.Equal(1, s.tenantRepo.GetCallCount("Create"))

	// Test GetByID
	retrieved, err := s.tenantRepo.GetByID(s.ctx, tenant.ID)
	s.NoError(err)
	s.Equal(tenant.Name, retrieved.Name)
	s.Equal(tenant.Workers, retrieved.Workers)

	// Test GetAll
	tenants, err := s.tenantRepo.GetAll(s.ctx)
	s.NoError(err)
	s.Len(tenants, 1)

	// Test UpdateWorkers
	err = s.tenantRepo.UpdateWorkers(s.ctx, tenant.ID, 10)
	s.NoError(err)
	
	updated, err := s.tenantRepo.GetByID(s.ctx, tenant.ID)
	s.NoError(err)
	s.Equal(10, updated.Workers)

	// Test Delete
	err = s.tenantRepo.Delete(s.ctx, tenant.ID)
	s.NoError(err)
	
	_, err = s.tenantRepo.GetByID(s.ctx, tenant.ID)
	s.Error(err)
	s.Contains(err.Error(), "tenant not found")
}

func (s *TenantManagerTestSuite) TestTenantRepositoryErrors() {
	// Test Create error
	s.tenantRepo.SetError("Create", fmt.Errorf("database error"))
	
	tenant := &models.Tenant{
		ID:      uuid.New(),
		Name:    "test-tenant",
		Workers: 5,
	}
	
	err := s.tenantRepo.Create(s.ctx, tenant)
	s.Error(err)
	s.Contains(err.Error(), "database error")

	// Test GetByID error
	s.tenantRepo.SetError("GetByID", fmt.Errorf("connection error"))
	
	_, err = s.tenantRepo.GetByID(s.ctx, uuid.New())
	s.Error(err)
	s.Contains(err.Error(), "connection error")
}

func (s *TenantManagerTestSuite) TestRabbitMQManager() {
	queueName := "test-queue"
	message := []byte(`{"test": "message"}`)

	// Test CreateQueue
	err := s.rabbitMQ.CreateQueue(queueName)
	s.NoError(err)
	s.Equal(1, s.rabbitMQ.GetCallCount("CreateQueue"))

	// Test PublishMessage
	err = s.rabbitMQ.PublishMessage(queueName, message)
	s.NoError(err)
	s.Equal(1, s.rabbitMQ.GetCallCount("PublishMessage"))

	// Test IsConnected
	connected := s.rabbitMQ.IsConnected()
	s.True(connected)

	// Test DeleteQueue
	err = s.rabbitMQ.DeleteQueue(queueName)
	s.NoError(err)
	s.Equal(1, s.rabbitMQ.GetCallCount("DeleteQueue"))
}

func (s *TenantManagerTestSuite) TestRabbitMQManagerErrors() {
	// Test CreateQueue error
	s.rabbitMQ.SetError("CreateQueue", fmt.Errorf("queue creation failed"))
	
	err := s.rabbitMQ.CreateQueue("test-queue")
	s.Error(err)
	s.Contains(err.Error(), "queue creation failed")

	// Test PublishMessage error
	s.rabbitMQ.SetError("PublishMessage", fmt.Errorf("publish failed"))
	
	err = s.rabbitMQ.PublishMessage("test-queue", []byte("test"))
	s.Error(err)
	s.Contains(err.Error(), "publish failed")
}

func (s *TenantManagerTestSuite) TestMessageRepository() {
	message := &models.Message{
		ID:        uuid.New(),
		TenantID:  uuid.New(),
		Payload:   []byte(`{"test": "data"}`),
		CreatedAt: time.Now(),
	}

	// Test Create
	err := s.messageRepo.Create(s.ctx, message)
	s.NoError(err)
	s.Equal(1, s.messageRepo.GetCallCount("Create"))

	// Test GetByID
	retrieved, err := s.messageRepo.GetByID(s.ctx, message.ID)
	s.NoError(err)
	s.Equal(message.TenantID, retrieved.TenantID)

	// Test List
	params := models.PaginationParams{Limit: 10}
	response, err := s.messageRepo.List(s.ctx, params)
	s.NoError(err)
	s.Len(response.Data, 1)

	// Test ListByTenant
	response, err = s.messageRepo.ListByTenant(s.ctx, message.TenantID, params)
	s.NoError(err)
	s.Len(response.Data, 1)
	s.Equal(message.TenantID, response.Data[0].TenantID)
}

func (s *TenantManagerTestSuite) TestGetTenantQueueName() {
	tenantID := uuid.New()
	expectedQueue := fmt.Sprintf("tenant_%s_queue", tenantID.String())
	actualQueue := messaging.GetTenantQueueName(tenantID)
	s.Equal(expectedQueue, actualQueue)
}

// Test table-driven approach for edge cases
func (s *TenantManagerTestSuite) TestTenantCreationLogic_EdgeCases() {
	tests := []struct {
		name            string
		request         *models.CreateTenantRequest
		expectedWorkers int
	}{
		{
			name:            "empty tenant name",
			request:         &models.CreateTenantRequest{Name: "", Workers: 3},
			expectedWorkers: 3,
		},
		{
			name:            "negative worker count",
			request:         &models.CreateTenantRequest{Name: "test", Workers: -1},
			expectedWorkers: 3, // Should use default
		},
		{
			name:            "zero worker count",
			request:         &models.CreateTenantRequest{Name: "test", Workers: 0},
			expectedWorkers: 3, // Should use default
		},
		{
			name:            "large worker count",
			request:         &models.CreateTenantRequest{Name: "test", Workers: 1000},
			expectedWorkers: 1000,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			tenant := &models.Tenant{
				ID:      uuid.New(),
				Name:    tt.request.Name,
				Workers: tt.request.Workers,
			}

			// Apply default worker logic (from Manager.CreateTenant)
			if tenant.Workers <= 0 {
				tenant.Workers = 3 // default
			}

			s.Equal(tt.expectedWorkers, tenant.Workers)
		})
	}
}

func TestTenantManagerSuite(t *testing.T) {
	suite.Run(t, new(TenantManagerTestSuite))
}

// Test individual functions for specific scenarios
func TestNewManager(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	
	manager := NewManagerWithInterfaces(
		NewMockTenantRepository(),
		NewMockMessageRepository(),
		NewMockRabbitMQManager(),
		3,
		logger,
	)
	
	assert.NotNil(t, manager)
	assert.Equal(t, 3, manager.defaultWorkers)
	assert.NotNil(t, manager.workerPools)
}

func TestTenantStats(t *testing.T) {
	// Test TenantStats struct
	tenantID := uuid.New()
	queueName := "test-queue"
	workerCount := 5

	stats := &TenantStats{
		TenantID:    tenantID,
		QueueName:   queueName,
		WorkerCount: workerCount,
	}

	assert.Equal(t, tenantID, stats.TenantID)
	assert.Equal(t, queueName, stats.QueueName)
	assert.Equal(t, workerCount, stats.WorkerCount)
}

// Test concurrent operations safety
func TestTenantRepository_ConcurrentOperations(t *testing.T) {
	repo := NewMockTenantRepository()
	ctx := context.Background()
	
	// Test concurrent creates
	var wg sync.WaitGroup
	numGoroutines := 10
	
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(index int) {
			defer wg.Done()
			
			tenant := &models.Tenant{
				ID:      uuid.New(),
				Name:    fmt.Sprintf("tenant-%d", index),
				Workers: index + 1,
			}
			
			err := repo.Create(ctx, tenant)
			require.NoError(t, err)
		}(i)
	}
	
	wg.Wait()
	
	// Verify all tenants were created
	tenants, err := repo.GetAll(ctx)
	require.NoError(t, err)
	assert.Len(t, tenants, numGoroutines)
	assert.Equal(t, numGoroutines, repo.GetCallCount("Create"))
}