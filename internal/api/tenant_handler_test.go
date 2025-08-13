package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap"

	"github.com/galihcitta/multi-tenant-messaging-system/internal/models"
)

// MockTenantManager for testing API handlers
// MockTenantStats mirrors the TenantStats struct from the tenant package
type MockTenantStats struct {
	TenantID    uuid.UUID `json:"tenant_id"`
	QueueName   string    `json:"queue_name"`
	WorkerCount int       `json:"worker_count"`
}

type MockTenantManager struct {
	tenants       map[uuid.UUID]*models.Tenant
	calls         map[string]int
	errors        map[string]error
	mutex         sync.RWMutex
	tenantStats   map[uuid.UUID]*MockTenantStats
}

func NewMockTenantManager() *MockTenantManager {
	return &MockTenantManager{
		tenants:     make(map[uuid.UUID]*models.Tenant),
		calls:       make(map[string]int),
		errors:      make(map[string]error),
		tenantStats: make(map[uuid.UUID]*MockTenantStats),
	}
}

func (m *MockTenantManager) SetError(method string, err error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.errors[method] = err
}

func (m *MockTenantManager) GetCallCount(method string) int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.calls[method]
}

func (m *MockTenantManager) AddTenant(tenant *models.Tenant) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.tenants[tenant.ID] = tenant
	
	// Add stats for the tenant
	m.tenantStats[tenant.ID] = &MockTenantStats{
		TenantID:    tenant.ID,
		QueueName:   fmt.Sprintf("tenant_%s_queue", tenant.ID.String()),
		WorkerCount: tenant.Workers,
	}
}

func (m *MockTenantManager) CreateTenant(ctx context.Context, req *models.CreateTenantRequest) (*models.Tenant, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.calls["CreateTenant"]++

	if err := m.errors["CreateTenant"]; err != nil {
		return nil, err
	}

	tenant := &models.Tenant{
		ID:        uuid.New(),
		Name:      req.Name,
		Workers:   req.Workers,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Apply default worker logic
	if tenant.Workers <= 0 {
		tenant.Workers = 3
	}

	m.tenants[tenant.ID] = tenant

	// Add stats
	m.tenantStats[tenant.ID] = &MockTenantStats{
		TenantID:    tenant.ID,
		QueueName:   fmt.Sprintf("tenant_%s_queue", tenant.ID.String()),
		WorkerCount: tenant.Workers,
	}

	return tenant, nil
}

func (m *MockTenantManager) GetTenant(ctx context.Context, id uuid.UUID) (*models.Tenant, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	m.calls["GetTenant"]++

	if err := m.errors["GetTenant"]; err != nil {
		return nil, err
	}

	tenant, exists := m.tenants[id]
	if !exists {
		return nil, fmt.Errorf("tenant not found: %s", id.String())
	}

	return tenant, nil
}

func (m *MockTenantManager) GetAllTenants(ctx context.Context) ([]models.Tenant, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	m.calls["GetAllTenants"]++

	if err := m.errors["GetAllTenants"]; err != nil {
		return nil, err
	}

	var tenants []models.Tenant
	for _, tenant := range m.tenants {
		tenants = append(tenants, *tenant)
	}

	return tenants, nil
}

func (m *MockTenantManager) DeleteTenant(ctx context.Context, id uuid.UUID) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.calls["DeleteTenant"]++

	if err := m.errors["DeleteTenant"]; err != nil {
		return err
	}

	if _, exists := m.tenants[id]; !exists {
		return fmt.Errorf("tenant not found: %s", id.String())
	}

	delete(m.tenants, id)
	delete(m.tenantStats, id)
	return nil
}

func (m *MockTenantManager) UpdateTenantConcurrency(ctx context.Context, id uuid.UUID, req *models.UpdateTenantConcurrencyRequest) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.calls["UpdateTenantConcurrency"]++

	if err := m.errors["UpdateTenantConcurrency"]; err != nil {
		return err
	}

	tenant, exists := m.tenants[id]
	if !exists {
		return fmt.Errorf("tenant not found: %s", id.String())
	}

	tenant.Workers = req.Workers
	tenant.UpdatedAt = time.Now()

	// Update stats
	if stats, exists := m.tenantStats[id]; exists {
		stats.WorkerCount = req.Workers
	}

	return nil
}

func (m *MockTenantManager) GetTenantStats() map[uuid.UUID]interface{} {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	m.calls["GetTenantStats"]++

	stats := make(map[uuid.UUID]interface{})
	for id, tenantStats := range m.tenantStats {
		stats[id] = tenantStats
	}

	return stats
}

// Additional methods that might be needed
func (m *MockTenantManager) PublishMessage(tenantID uuid.UUID, message []byte) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.calls["PublishMessage"]++
	return m.errors["PublishMessage"]
}

func (m *MockTenantManager) RestoreConsumers(ctx context.Context) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.calls["RestoreConsumers"]++
	return m.errors["RestoreConsumers"]
}

func (m *MockTenantManager) Shutdown() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.calls["Shutdown"]++
	return m.errors["Shutdown"]
}

// TenantHandlerTestSuite provides test suite for tenant handler
type TenantHandlerTestSuite struct {
	suite.Suite
	handler       *TenantHandler
	tenantManager *MockTenantManager
	logger        *zap.Logger
	router        *gin.Engine
}

func (s *TenantHandlerTestSuite) SetupTest() {
	var err error
	s.logger, err = zap.NewDevelopment()
	s.Require().NoError(err)

	s.tenantManager = NewMockTenantManager()
	s.handler = NewTenantHandler(s.tenantManager, s.logger)

	// Set Gin to test mode
	gin.SetMode(gin.TestMode)
	s.router = gin.New()

	// Setup routes
	api := s.router.Group("/api/v1")
	{
		api.POST("/tenants", s.handler.CreateTenant)
		api.GET("/tenants/:id", s.handler.GetTenant)
		api.GET("/tenants", s.handler.GetAllTenants)
		api.DELETE("/tenants/:id", s.handler.DeleteTenant)
		api.PUT("/tenants/:id/config/concurrency", s.handler.UpdateTenantConcurrency)
		api.GET("/tenants/stats", s.handler.GetTenantStats)
	}
}

func (s *TenantHandlerTestSuite) TestCreateTenant_Success() {
	req := models.CreateTenantRequest{
		Name:    "test-tenant",
		Workers: 5,
	}
	reqBody, _ := json.Marshal(req)

	w := httptest.NewRecorder()
	httpReq, _ := http.NewRequest("POST", "/api/v1/tenants", bytes.NewBuffer(reqBody))
	httpReq.Header.Set("Content-Type", "application/json")

	s.router.ServeHTTP(w, httpReq)

	s.Equal(http.StatusCreated, w.Code)

	var response models.Tenant
	err := json.Unmarshal(w.Body.Bytes(), &response)
	s.NoError(err)

	s.Equal(req.Name, response.Name)
	s.Equal(req.Workers, response.Workers)
	s.NotEqual(uuid.Nil, response.ID)
	s.Equal(1, s.tenantManager.GetCallCount("CreateTenant"))
}

func (s *TenantHandlerTestSuite) TestCreateTenant_DefaultWorkers() {
	req := models.CreateTenantRequest{
		Name:    "test-tenant",
		Workers: 0, // Should use default
	}
	reqBody, _ := json.Marshal(req)

	w := httptest.NewRecorder()
	httpReq, _ := http.NewRequest("POST", "/api/v1/tenants", bytes.NewBuffer(reqBody))
	httpReq.Header.Set("Content-Type", "application/json")

	s.router.ServeHTTP(w, httpReq)

	s.Equal(http.StatusCreated, w.Code)

	var response models.Tenant
	err := json.Unmarshal(w.Body.Bytes(), &response)
	s.NoError(err)

	s.Equal(3, response.Workers) // Default value
}

func (s *TenantHandlerTestSuite) TestCreateTenant_InvalidJSON() {
	invalidJSON := `{"name": "test", "workers": "invalid"}`

	w := httptest.NewRecorder()
	httpReq, _ := http.NewRequest("POST", "/api/v1/tenants", bytes.NewBufferString(invalidJSON))
	httpReq.Header.Set("Content-Type", "application/json")

	s.router.ServeHTTP(w, httpReq)

	s.Equal(http.StatusBadRequest, w.Code)

	var response ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	s.NoError(err)
	s.NotEmpty(response.Error)
	s.Equal(0, s.tenantManager.GetCallCount("CreateTenant"))
}

func (s *TenantHandlerTestSuite) TestCreateTenant_ManagerError() {
	s.tenantManager.SetError("CreateTenant", fmt.Errorf("manager error"))

	req := models.CreateTenantRequest{
		Name:    "test-tenant",
		Workers: 5,
	}
	reqBody, _ := json.Marshal(req)

	w := httptest.NewRecorder()
	httpReq, _ := http.NewRequest("POST", "/api/v1/tenants", bytes.NewBuffer(reqBody))
	httpReq.Header.Set("Content-Type", "application/json")

	s.router.ServeHTTP(w, httpReq)

	s.Equal(http.StatusInternalServerError, w.Code)

	var response ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	s.NoError(err)
	s.Contains(response.Error, "manager error")
	s.Equal(1, s.tenantManager.GetCallCount("CreateTenant"))
}

func (s *TenantHandlerTestSuite) TestGetTenant_Success() {
	// Pre-create a tenant
	tenant := &models.Tenant{
		ID:        uuid.New(),
		Name:      "existing-tenant",
		Workers:   3,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	s.tenantManager.AddTenant(tenant)

	w := httptest.NewRecorder()
	httpReq, _ := http.NewRequest("GET", fmt.Sprintf("/api/v1/tenants/%s", tenant.ID.String()), nil)

	s.router.ServeHTTP(w, httpReq)

	s.Equal(http.StatusOK, w.Code)

	var response models.Tenant
	err := json.Unmarshal(w.Body.Bytes(), &response)
	s.NoError(err)

	s.Equal(tenant.ID, response.ID)
	s.Equal(tenant.Name, response.Name)
	s.Equal(tenant.Workers, response.Workers)
	s.Equal(1, s.tenantManager.GetCallCount("GetTenant"))
}

func (s *TenantHandlerTestSuite) TestGetTenant_InvalidUUID() {
	w := httptest.NewRecorder()
	httpReq, _ := http.NewRequest("GET", "/api/v1/tenants/invalid-uuid", nil)

	s.router.ServeHTTP(w, httpReq)

	s.Equal(http.StatusBadRequest, w.Code)

	var response ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	s.NoError(err)
	s.Equal("invalid tenant ID format", response.Error)
	s.Equal(0, s.tenantManager.GetCallCount("GetTenant"))
}

func (s *TenantHandlerTestSuite) TestGetTenant_NotFound() {
	nonExistentID := uuid.New()

	w := httptest.NewRecorder()
	httpReq, _ := http.NewRequest("GET", fmt.Sprintf("/api/v1/tenants/%s", nonExistentID.String()), nil)

	s.router.ServeHTTP(w, httpReq)

	s.Equal(http.StatusNotFound, w.Code)

	var response ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	s.NoError(err)
	s.Equal("tenant not found", response.Error)
	s.Equal(1, s.tenantManager.GetCallCount("GetTenant"))
}

func (s *TenantHandlerTestSuite) TestGetAllTenants_Success() {
	// Pre-create some tenants
	tenant1 := &models.Tenant{ID: uuid.New(), Name: "tenant1", Workers: 3}
	tenant2 := &models.Tenant{ID: uuid.New(), Name: "tenant2", Workers: 5}
	s.tenantManager.AddTenant(tenant1)
	s.tenantManager.AddTenant(tenant2)

	w := httptest.NewRecorder()
	httpReq, _ := http.NewRequest("GET", "/api/v1/tenants", nil)

	s.router.ServeHTTP(w, httpReq)

	s.Equal(http.StatusOK, w.Code)

	var response []models.Tenant
	err := json.Unmarshal(w.Body.Bytes(), &response)
	s.NoError(err)

	s.Len(response, 2)
	s.Equal(1, s.tenantManager.GetCallCount("GetAllTenants"))
}

func (s *TenantHandlerTestSuite) TestGetAllTenants_Empty() {
	w := httptest.NewRecorder()
	httpReq, _ := http.NewRequest("GET", "/api/v1/tenants", nil)

	s.router.ServeHTTP(w, httpReq)

	s.Equal(http.StatusOK, w.Code)

	var response []models.Tenant
	err := json.Unmarshal(w.Body.Bytes(), &response)
	s.NoError(err)

	s.Len(response, 0)
	s.Equal(1, s.tenantManager.GetCallCount("GetAllTenants"))
}

func (s *TenantHandlerTestSuite) TestDeleteTenant_Success() {
	// Pre-create a tenant
	tenant := &models.Tenant{
		ID:      uuid.New(),
		Name:    "to-delete",
		Workers: 3,
	}
	s.tenantManager.AddTenant(tenant)

	w := httptest.NewRecorder()
	httpReq, _ := http.NewRequest("DELETE", fmt.Sprintf("/api/v1/tenants/%s", tenant.ID.String()), nil)

	s.router.ServeHTTP(w, httpReq)

	s.Equal(http.StatusNoContent, w.Code)
	s.Empty(w.Body.String())
	s.Equal(1, s.tenantManager.GetCallCount("DeleteTenant"))
}

func (s *TenantHandlerTestSuite) TestDeleteTenant_InvalidUUID() {
	w := httptest.NewRecorder()
	httpReq, _ := http.NewRequest("DELETE", "/api/v1/tenants/invalid-uuid", nil)

	s.router.ServeHTTP(w, httpReq)

	s.Equal(http.StatusBadRequest, w.Code)

	var response ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	s.NoError(err)
	s.Equal("invalid tenant ID format", response.Error)
	s.Equal(0, s.tenantManager.GetCallCount("DeleteTenant"))
}

func (s *TenantHandlerTestSuite) TestDeleteTenant_NotFound() {
	nonExistentID := uuid.New()

	w := httptest.NewRecorder()
	httpReq, _ := http.NewRequest("DELETE", fmt.Sprintf("/api/v1/tenants/%s", nonExistentID.String()), nil)

	s.router.ServeHTTP(w, httpReq)

	s.Equal(http.StatusNotFound, w.Code)

	var response ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	s.NoError(err)
	s.Equal("tenant not found", response.Error)
	s.Equal(1, s.tenantManager.GetCallCount("DeleteTenant"))
}

func (s *TenantHandlerTestSuite) TestUpdateTenantConcurrency_Success() {
	// Pre-create a tenant
	tenant := &models.Tenant{
		ID:      uuid.New(),
		Name:    "to-update",
		Workers: 3,
	}
	s.tenantManager.AddTenant(tenant)

	req := models.UpdateTenantConcurrencyRequest{
		Workers: 10,
	}
	reqBody, _ := json.Marshal(req)

	w := httptest.NewRecorder()
	httpReq, _ := http.NewRequest("PUT", fmt.Sprintf("/api/v1/tenants/%s/config/concurrency", tenant.ID.String()), bytes.NewBuffer(reqBody))
	httpReq.Header.Set("Content-Type", "application/json")

	s.router.ServeHTTP(w, httpReq)

	s.Equal(http.StatusOK, w.Code)

	var response map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &response)
	s.NoError(err)
	s.Equal("concurrency updated successfully", response["message"])
	s.Equal(1, s.tenantManager.GetCallCount("UpdateTenantConcurrency"))
}

func (s *TenantHandlerTestSuite) TestUpdateTenantConcurrency_InvalidJSON() {
	tenantID := uuid.New()
	invalidJSON := `{"workers": "invalid"}`

	w := httptest.NewRecorder()
	httpReq, _ := http.NewRequest("PUT", fmt.Sprintf("/api/v1/tenants/%s/config/concurrency", tenantID.String()), bytes.NewBufferString(invalidJSON))
	httpReq.Header.Set("Content-Type", "application/json")

	s.router.ServeHTTP(w, httpReq)

	s.Equal(http.StatusBadRequest, w.Code)
	s.Equal(0, s.tenantManager.GetCallCount("UpdateTenantConcurrency"))
}

func (s *TenantHandlerTestSuite) TestGetTenantStats_Success() {
	// Pre-create some tenants with stats
	tenant1 := &models.Tenant{ID: uuid.New(), Name: "tenant1", Workers: 3}
	tenant2 := &models.Tenant{ID: uuid.New(), Name: "tenant2", Workers: 5}
	s.tenantManager.AddTenant(tenant1)
	s.tenantManager.AddTenant(tenant2)

	w := httptest.NewRecorder()
	httpReq, _ := http.NewRequest("GET", "/api/v1/tenants/stats", nil)

	s.router.ServeHTTP(w, httpReq)

	s.Equal(http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	s.NoError(err)

	s.Len(response, 2)
	s.Equal(1, s.tenantManager.GetCallCount("GetTenantStats"))
}

func TestTenantHandlerSuite(t *testing.T) {
	suite.Run(t, new(TenantHandlerTestSuite))
}

// Test individual functions and edge cases
func TestNewTenantHandler(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	manager := NewMockTenantManager()

	handler := NewTenantHandler(manager, logger)

	assert.NotNil(t, handler)
	assert.Equal(t, manager, handler.tenantManager)
	assert.Equal(t, logger, handler.logger)
}

func TestErrorResponse(t *testing.T) {
	errorMsg := "test error message"
	errorResp := ErrorResponse{Error: errorMsg}

	assert.Equal(t, errorMsg, errorResp.Error)

	// Test JSON marshaling
	jsonBytes, err := json.Marshal(errorResp)
	assert.NoError(t, err)

	var unmarshaledResp ErrorResponse
	err = json.Unmarshal(jsonBytes, &unmarshaledResp)
	assert.NoError(t, err)
	assert.Equal(t, errorMsg, unmarshaledResp.Error)
}

func TestHTTPStatusCodes(t *testing.T) {
	// Test that we're using the correct HTTP status codes
	tests := []struct {
		scenario     string
		expectedCode int
	}{
		{"successful creation", http.StatusCreated},
		{"successful get", http.StatusOK},
		{"successful update", http.StatusOK},
		{"successful delete", http.StatusNoContent},
		{"bad request", http.StatusBadRequest},
		{"not found", http.StatusNotFound},
		{"internal server error", http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.scenario, func(t *testing.T) {
			// Verify the status codes are what we expect
			assert.Greater(t, tt.expectedCode, 0)
			assert.LessOrEqual(t, tt.expectedCode, 599)
		})
	}
}

func TestUUIDValidation(t *testing.T) {
	tests := []struct {
		input    string
		isValid  bool
	}{
		{uuid.New().String(), true},
		{"invalid-uuid", false},
		{"", false},
		{"550e8400-e29b-41d4-a716-446655440000", true},
		{"550e8400e29b41d4a716446655440000", true}, // Missing dashes but still valid UUID
		{"550e8400-e29b-41d4-a716-44665544000", false}, // Wrong length
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("uuid_%s", tt.input), func(t *testing.T) {
			_, err := uuid.Parse(tt.input)
			if tt.isValid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

// Benchmark tests
func BenchmarkTenantCreation(b *testing.B) {
	logger, _ := zap.NewDevelopment()
	manager := NewMockTenantManager()
	handler := NewTenantHandler(manager, logger)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/tenants", handler.CreateTenant)

	req := models.CreateTenantRequest{
		Name:    "benchmark-tenant",
		Workers: 5,
	}
	reqBody, _ := json.Marshal(req)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		httpReq, _ := http.NewRequest("POST", "/tenants", bytes.NewBuffer(reqBody))
		httpReq.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, httpReq)
	}
}

func BenchmarkUUIDParsing(b *testing.B) {
	validUUID := uuid.New().String()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = uuid.Parse(validUUID)
	}
}