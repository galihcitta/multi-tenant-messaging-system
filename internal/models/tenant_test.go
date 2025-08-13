package models

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

// TenantModelTestSuite provides test suite for tenant models
type TenantModelTestSuite struct {
	suite.Suite
}

func (s *TenantModelTestSuite) TestTenantModel() {
	tenant := Tenant{
		ID:        uuid.New(),
		Name:      "test-tenant",
		Workers:   5,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	s.NotEqual(uuid.Nil, tenant.ID)
	s.Equal("test-tenant", tenant.Name)
	s.Equal(5, tenant.Workers)
	s.False(tenant.CreatedAt.IsZero())
	s.False(tenant.UpdatedAt.IsZero())
}

func (s *TenantModelTestSuite) TestTenantJSONMarshaling() {
	tenant := Tenant{
		ID:        uuid.New(),
		Name:      "test-tenant",
		Workers:   3,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Test marshaling to JSON
	jsonBytes, err := json.Marshal(tenant)
	s.NoError(err)
	s.NotEmpty(jsonBytes)

	// Test unmarshaling from JSON
	var unmarshaledTenant Tenant
	err = json.Unmarshal(jsonBytes, &unmarshaledTenant)
	s.NoError(err)

	s.Equal(tenant.ID, unmarshaledTenant.ID)
	s.Equal(tenant.Name, unmarshaledTenant.Name)
	s.Equal(tenant.Workers, unmarshaledTenant.Workers)
	
	// Note: Time precision might differ slightly in JSON marshaling
	s.WithinDuration(tenant.CreatedAt, unmarshaledTenant.CreatedAt, time.Second)
	s.WithinDuration(tenant.UpdatedAt, unmarshaledTenant.UpdatedAt, time.Second)
}

func (s *TenantModelTestSuite) TestCreateTenantRequest() {
	req := CreateTenantRequest{
		Name:    "new-tenant",
		Workers: 8,
	}

	s.Equal("new-tenant", req.Name)
	s.Equal(8, req.Workers)

	// Test JSON tags
	jsonBytes, err := json.Marshal(req)
	s.NoError(err)

	var unmarshaledReq CreateTenantRequest
	err = json.Unmarshal(jsonBytes, &unmarshaledReq)
	s.NoError(err)

	s.Equal(req.Name, unmarshaledReq.Name)
	s.Equal(req.Workers, unmarshaledReq.Workers)
}

func (s *TenantModelTestSuite) TestCreateTenantRequestValidation() {
	// Test with required name
	validReq := CreateTenantRequest{
		Name:    "valid-tenant",
		Workers: 5,
	}
	s.NotEmpty(validReq.Name)

	// Test optional workers field
	reqWithoutWorkers := CreateTenantRequest{
		Name: "tenant-default-workers",
		// Workers omitted - should be 0
	}
	s.Equal(0, reqWithoutWorkers.Workers)

	// Test negative workers
	reqNegativeWorkers := CreateTenantRequest{
		Name:    "negative-workers",
		Workers: -1,
	}
	s.Equal(-1, reqNegativeWorkers.Workers)
}

func (s *TenantModelTestSuite) TestUpdateTenantConcurrencyRequest() {
	req := UpdateTenantConcurrencyRequest{
		Workers: 10,
	}

	s.Equal(10, req.Workers)

	// Test JSON marshaling
	jsonBytes, err := json.Marshal(req)
	s.NoError(err)

	var unmarshaledReq UpdateTenantConcurrencyRequest
	err = json.Unmarshal(jsonBytes, &unmarshaledReq)
	s.NoError(err)

	s.Equal(req.Workers, unmarshaledReq.Workers)
}

func (s *TenantModelTestSuite) TestTenantModelDefaults() {
	// Test zero values
	var tenant Tenant

	s.Equal(uuid.Nil, tenant.ID)
	s.Empty(tenant.Name)
	s.Equal(0, tenant.Workers)
	s.True(tenant.CreatedAt.IsZero())
	s.True(tenant.UpdatedAt.IsZero())
}

func (s *TenantModelTestSuite) TestJSONFieldNames() {
	tenant := Tenant{
		ID:      uuid.New(),
		Name:    "json-test",
		Workers: 3,
	}

	jsonBytes, err := json.Marshal(tenant)
	s.NoError(err)

	jsonString := string(jsonBytes)
	
	// Verify JSON field names match expected tags
	s.Contains(jsonString, "\"id\":")
	s.Contains(jsonString, "\"name\":")
	s.Contains(jsonString, "\"workers\":")
	s.Contains(jsonString, "\"created_at\":")
	s.Contains(jsonString, "\"updated_at\":")
}

func TestTenantModelSuite(t *testing.T) {
	suite.Run(t, new(TenantModelTestSuite))
}

// Test individual functions
func TestTenantModelCreation(t *testing.T) {
	id := uuid.New()
	name := "test-tenant"
	workers := 5
	now := time.Now()

	tenant := Tenant{
		ID:        id,
		Name:      name,
		Workers:   workers,
		CreatedAt: now,
		UpdatedAt: now,
	}

	assert.Equal(t, id, tenant.ID)
	assert.Equal(t, name, tenant.Name)
	assert.Equal(t, workers, tenant.Workers)
	assert.Equal(t, now, tenant.CreatedAt)
	assert.Equal(t, now, tenant.UpdatedAt)
}

func TestCreateTenantRequestValidation(t *testing.T) {
	tests := []struct {
		name         string
		request      CreateTenantRequest
		expectValid  bool
		description  string
	}{
		{
			name:        "valid request",
			request:     CreateTenantRequest{Name: "valid", Workers: 5},
			expectValid: true,
			description: "Valid request should pass validation",
		},
		{
			name:        "empty name",
			request:     CreateTenantRequest{Name: "", Workers: 5},
			expectValid: false,
			description: "Empty name should fail validation if required",
		},
		{
			name:        "zero workers",
			request:     CreateTenantRequest{Name: "test", Workers: 0},
			expectValid: true,
			description: "Zero workers might be valid (default will be applied)",
		},
		{
			name:        "negative workers",
			request:     CreateTenantRequest{Name: "test", Workers: -1},
			expectValid: false,
			description: "Negative workers should fail validation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate basic validation logic
			isValid := validateCreateTenantRequest(&tt.request)
			if tt.expectValid {
				assert.True(t, isValid, tt.description)
			} else {
				assert.False(t, isValid, tt.description)
			}
		})
	}
}

// Helper function to simulate validation
func validateCreateTenantRequest(req *CreateTenantRequest) bool {
	if req == nil {
		return false
	}
	if req.Name == "" {
		return false // Assuming name is required
	}
	if req.Workers < 0 {
		return false // Negative workers not allowed
	}
	return true
}

func TestUpdateTenantConcurrencyValidation(t *testing.T) {
	tests := []struct {
		name        string
		workers     int
		expectValid bool
	}{
		{"valid positive", 5, true},
		{"valid one", 1, true},
		{"invalid zero", 0, false},
		{"invalid negative", -1, false},
		{"valid large", 100, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := UpdateTenantConcurrencyRequest{Workers: tt.workers}
			
			// Simulate validation (min=1 based on binding tag)
			isValid := req.Workers >= 1
			assert.Equal(t, tt.expectValid, isValid)
		})
	}
}

func TestTenantJSONTags(t *testing.T) {
	tenant := Tenant{
		ID:      uuid.New(),
		Name:    "tag-test",
		Workers: 7,
	}

	jsonBytes, err := json.Marshal(tenant)
	assert.NoError(t, err)

	jsonMap := make(map[string]interface{})
	err = json.Unmarshal(jsonBytes, &jsonMap)
	assert.NoError(t, err)

	// Verify JSON field names
	_, hasID := jsonMap["id"]
	_, hasName := jsonMap["name"]
	_, hasWorkers := jsonMap["workers"]
	_, hasCreatedAt := jsonMap["created_at"]
	_, hasUpdatedAt := jsonMap["updated_at"]

	assert.True(t, hasID)
	assert.True(t, hasName)
	assert.True(t, hasWorkers)
	assert.True(t, hasCreatedAt)
	assert.True(t, hasUpdatedAt)
}

func TestTenantModelEdgeCases(t *testing.T) {
	// Test with very long name
	longName := string(make([]byte, 1000))
	for i := range longName {
		longName = longName[:i] + "a" + longName[i+1:]
	}

	tenant := Tenant{
		ID:      uuid.New(),
		Name:    longName,
		Workers: 1,
	}

	assert.Equal(t, longName, tenant.Name)
	assert.Len(t, tenant.Name, 1000)

	// Test with maximum integer workers
	maxWorkers := int(^uint(0) >> 1) // Max int value
	tenant.Workers = maxWorkers
	assert.Equal(t, maxWorkers, tenant.Workers)

	// Test JSON marshaling with edge cases
	jsonBytes, err := json.Marshal(tenant)
	assert.NoError(t, err)

	var unmarshaledTenant Tenant
	err = json.Unmarshal(jsonBytes, &unmarshaledTenant)
	assert.NoError(t, err)

	assert.Equal(t, tenant.Name, unmarshaledTenant.Name)
	assert.Equal(t, tenant.Workers, unmarshaledTenant.Workers)
}

// Benchmark tests
func BenchmarkTenantCreation(b *testing.B) {
	id := uuid.New()
	name := "benchmark-tenant"
	workers := 5
	now := time.Now()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Tenant{
			ID:        id,
			Name:      name,
			Workers:   workers,
			CreatedAt: now,
			UpdatedAt: now,
		}
	}
}

func BenchmarkTenantJSONMarshaling(b *testing.B) {
	tenant := Tenant{
		ID:        uuid.New(),
		Name:      "benchmark-tenant",
		Workers:   5,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = json.Marshal(tenant)
	}
}

func BenchmarkTenantJSONUnmarshaling(b *testing.B) {
	tenant := Tenant{
		ID:        uuid.New(),
		Name:      "benchmark-tenant",
		Workers:   5,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	jsonBytes, _ := json.Marshal(tenant)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var unmarshaledTenant Tenant
		_ = json.Unmarshal(jsonBytes, &unmarshaledTenant)
	}
}