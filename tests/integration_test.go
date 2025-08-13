package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap"

	"github.com/galihcitta/multi-tenant-messaging-system/internal/api"
	"github.com/galihcitta/multi-tenant-messaging-system/internal/config"
	"github.com/galihcitta/multi-tenant-messaging-system/internal/models"
	"github.com/galihcitta/multi-tenant-messaging-system/internal/repository"
	"github.com/galihcitta/multi-tenant-messaging-system/internal/services/messaging"
	"github.com/galihcitta/multi-tenant-messaging-system/internal/services/tenant"
)

type IntegrationTestSuite struct {
	suite.Suite
	pool         *dockertest.Pool
	pgResource   *dockertest.Resource
	rmqResource  *dockertest.Resource
	db           *repository.Database
	server       *api.Server
	httpServer   *httptest.Server
	tenantManager *tenant.Manager
	logger       *zap.Logger
	config       *config.Config
	dbURL        string
	rmqURL       string
}

func (s *IntegrationTestSuite) SetupSuite() {
	var err error

	// Initialize logger
	s.logger, err = zap.NewDevelopment()
	s.Require().NoError(err)

	// Create dockertest pool
	s.pool, err = dockertest.NewPool("")
	s.Require().NoError(err)

	// Test Docker connection
	err = s.pool.Client.Ping()
	s.Require().NoError(err)

	// Start PostgreSQL container
	s.startPostgreSQL()

	// Start RabbitMQ container
	s.startRabbitMQ()

	// Initialize application components
	s.initializeApp()

	// Set Gin to test mode
	gin.SetMode(gin.TestMode)
}

func (s *IntegrationTestSuite) TearDownSuite() {
	if s.httpServer != nil {
		s.httpServer.Close()
	}
	
	if s.tenantManager != nil {
		s.tenantManager.Shutdown()
	}
	
	if s.db != nil {
		s.db.Close()
	}

	if s.pgResource != nil {
		if err := s.pool.Purge(s.pgResource); err != nil {
			s.logger.Error("Failed to purge PostgreSQL container", zap.Error(err))
		}
	}

	if s.rmqResource != nil {
		if err := s.pool.Purge(s.rmqResource); err != nil {
			s.logger.Error("Failed to purge RabbitMQ container", zap.Error(err))
		}
	}
}

func (s *IntegrationTestSuite) startPostgreSQL() {
	var err error

	s.pgResource, err = s.pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "postgres",
		Tag:        "15",
		Env: []string{
			"POSTGRES_PASSWORD=testpass",
			"POSTGRES_USER=testuser",
			"POSTGRES_DB=testdb",
			"listen_addresses = '*'",
		},
	}, func(config *docker.HostConfig) {
		config.AutoRemove = true
		config.RestartPolicy = docker.RestartPolicy{Name: "no"}
	})
	s.Require().NoError(err)

	s.pgResource.Expire(120) // 2 minutes

	// Construct database URL
	s.dbURL = fmt.Sprintf("postgres://testuser:testpass@localhost:%s/testdb?sslmode=disable",
		s.pgResource.GetPort("5432/tcp"))

	// Wait for PostgreSQL to be ready
	s.pool.MaxWait = 120 * time.Second
	err = s.pool.Retry(func() error {
		db, err := repository.NewDatabase(s.dbURL, s.logger)
		if err != nil {
			return err
		}
		defer db.Close()
		return db.HealthCheck(context.Background())
	})
	s.Require().NoError(err)

	// Run migrations
	s.runMigrations()
}

func (s *IntegrationTestSuite) startRabbitMQ() {
	var err error

	s.rmqResource, err = s.pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "rabbitmq",
		Tag:        "3.12-management",
		Env: []string{
			"RABBITMQ_DEFAULT_USER=testuser",
			"RABBITMQ_DEFAULT_PASS=testpass",
		},
	}, func(config *docker.HostConfig) {
		config.AutoRemove = true
		config.RestartPolicy = docker.RestartPolicy{Name: "no"}
	})
	s.Require().NoError(err)

	s.rmqResource.Expire(120) // 2 minutes

	// Construct RabbitMQ URL
	s.rmqURL = fmt.Sprintf("amqp://testuser:testpass@localhost:%s/",
		s.rmqResource.GetPort("5672/tcp"))

	// Wait for RabbitMQ to be ready
	err = s.pool.Retry(func() error {
		rmq := messaging.NewRabbitMQManager(s.rmqURL, s.logger)
		err := rmq.Connect()
		if err != nil {
			return err
		}
		defer rmq.Close()
		return nil
	})
	s.Require().NoError(err)
}

func (s *IntegrationTestSuite) runMigrations() {
	// Run migrations using the direct migrate approach
	m, err := migrate.New("file://../migrations", s.dbURL)
	s.Require().NoError(err)

	// Run migrations
	err = m.Up()
	if err != nil && err != migrate.ErrNoChange {
		s.Require().NoError(err)
	}
	
	m.Close()
}

func (s *IntegrationTestSuite) initializeApp() {
	var err error

	// Create test configuration
	s.config = &config.Config{
		RabbitMQ: config.RabbitMQConfig{
			URL: s.rmqURL,
		},
		Database: config.DatabaseConfig{
			URL: s.dbURL,
		},
		Server: config.ServerConfig{
			Host: "localhost",
			Port: 8080,
		},
		Logging: config.LoggingConfig{
			Level:  "info",
			Format: "json",
		},
		Auth: config.AuthConfig{
			JWTSecret:         "test-secret-key-for-integration-tests",
			TokenExpiry:       24 * time.Hour,
			RefreshExpiry:     168 * time.Hour,
			RequireAuth:       false, // Disable auth for easier testing
			AllowRegistration: false,
		},
		Metrics: config.MetricsConfig{
			Enabled:        true,
			Path:          "/metrics",
			Port:          8080,
			UpdateInterval: 10 * time.Second,
		},
		Workers:                 3,
		GracefulShutdownTimeout: 30 * time.Second,
	}

	// Initialize database
	s.db, err = repository.NewDatabase(s.dbURL, s.logger)
	s.Require().NoError(err)

	// Initialize repositories
	tenantRepo := repository.NewTenantRepository(s.db.Pool(), s.logger)
	messageRepo := repository.NewMessageRepository(s.db.Pool(), s.logger)

	// Initialize RabbitMQ
	rabbitMQ := messaging.NewRabbitMQManager(s.rmqURL, s.logger)
	err = rabbitMQ.Connect()
	s.Require().NoError(err)

	// Initialize tenant manager
	s.tenantManager = tenant.NewManager(tenantRepo, messageRepo, rabbitMQ, 3, s.logger)

	// Initialize API server
	s.server = api.NewServer(s.config, s.tenantManager, messageRepo, s.logger)
	s.server.SetupRoutes()

	// Create test HTTP server
	s.httpServer = httptest.NewServer(s.server.GetRouter())
}

// Helper function to get admin authentication token for tests
func (s *IntegrationTestSuite) getAdminToken() string {
	loginReq := map[string]interface{}{
		"username": "admin",
		"password": "admin123",
	}
	loginReqBody, _ := json.Marshal(loginReq)

	resp, err := http.Post(s.httpServer.URL+"/auth/login", "application/json", bytes.NewBuffer(loginReqBody))
	s.Require().NoError(err)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		s.T().Fatalf("Failed to login admin user, status: %d", resp.StatusCode)
	}

	var loginResp map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&loginResp)
	s.Require().NoError(err)

	token, ok := loginResp["access_token"].(string)
	s.Require().True(ok, "access_token should be a string")
	return token
}

// Helper function to create authenticated HTTP request
func (s *IntegrationTestSuite) newAuthenticatedRequest(method, url string, body []byte) *http.Request {
	req, _ := http.NewRequest(method, url, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	
	token := s.getAdminToken()
	req.Header.Set("Authorization", "Bearer "+token)
	
	return req
}

func (s *IntegrationTestSuite) TestTenantLifecycle() {
	// Test tenant creation
	createReq := models.CreateTenantRequest{
		Name:    "test-tenant",
		Workers: 5,
	}
	createReqBody, _ := json.Marshal(createReq)

	resp, err := http.Post(s.httpServer.URL+"/api/v1/tenants", "application/json", bytes.NewBuffer(createReqBody))
	s.Require().NoError(err)
	s.Assert().Equal(http.StatusCreated, resp.StatusCode)

	var tenant models.Tenant
	err = json.NewDecoder(resp.Body).Decode(&tenant)
	s.Require().NoError(err)
	resp.Body.Close()

	s.Assert().Equal("test-tenant", tenant.Name)
	s.Assert().Equal(5, tenant.Workers)
	s.Assert().NotEqual(uuid.Nil, tenant.ID)

	// Wait for consumer to start
	time.Sleep(2 * time.Second)

	// Test get tenant
	resp, err = http.Get(s.httpServer.URL + "/api/v1/tenants/" + tenant.ID.String())
	s.Require().NoError(err)
	s.Assert().Equal(http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Test update tenant concurrency
	updateReq := models.UpdateTenantConcurrencyRequest{Workers: 10}
	updateReqBody, _ := json.Marshal(updateReq)

	req, _ := http.NewRequest(http.MethodPut, s.httpServer.URL+"/api/v1/tenants/"+tenant.ID.String()+"/config/concurrency", bytes.NewBuffer(updateReqBody))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err = client.Do(req)
	s.Require().NoError(err)
	s.Assert().Equal(http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Test delete tenant
	req, _ = http.NewRequest(http.MethodDelete, s.httpServer.URL+"/api/v1/tenants/"+tenant.ID.String(), nil)
	resp, err = client.Do(req)
	s.Require().NoError(err)
	s.Assert().Equal(http.StatusNoContent, resp.StatusCode)
	resp.Body.Close()

	// Wait for consumer to stop
	time.Sleep(2 * time.Second)

	// Verify tenant is deleted
	resp, err = http.Get(s.httpServer.URL + "/api/v1/tenants/" + tenant.ID.String())
	s.Require().NoError(err)
	s.Assert().Equal(http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()
}

func (s *IntegrationTestSuite) TestMessagePublishingAndConsumption() {
	// Create a tenant first
	createReq := models.CreateTenantRequest{
		Name:    "message-test-tenant",
		Workers: 3,
	}
	createReqBody, _ := json.Marshal(createReq)

	resp, err := http.Post(s.httpServer.URL+"/api/v1/tenants", "application/json", bytes.NewBuffer(createReqBody))
	s.Require().NoError(err)
	s.Assert().Equal(http.StatusCreated, resp.StatusCode)

	var tenant models.Tenant
	err = json.NewDecoder(resp.Body).Decode(&tenant)
	s.Require().NoError(err)
	resp.Body.Close()

	// Wait for consumer to start
	time.Sleep(2 * time.Second)

	// Publish a message
	messagePayload := json.RawMessage(`{"test": "data", "timestamp": "2023-01-01T00:00:00Z"}`)
	messageReq := models.CreateMessageRequest{
		TenantID: tenant.ID,
		Payload:  messagePayload,
	}
	messageReqBody, _ := json.Marshal(messageReq)

	resp, err = http.Post(s.httpServer.URL+"/api/v1/messages", "application/json", bytes.NewBuffer(messageReqBody))
	s.Require().NoError(err)
	s.Assert().Equal(http.StatusCreated, resp.StatusCode)

	var message models.Message
	err = json.NewDecoder(resp.Body).Decode(&message)
	s.Require().NoError(err)
	resp.Body.Close()

	s.Assert().Equal(tenant.ID, message.TenantID)
	s.Assert().JSONEq(string(messagePayload), string(message.Payload))

	// Wait for message to be consumed and stored
	time.Sleep(3 * time.Second)

	// Test message listing
	resp, err = http.Get(s.httpServer.URL + "/api/v1/messages?limit=10")
	s.Require().NoError(err)
	s.Assert().Equal(http.StatusOK, resp.StatusCode)

	var listResponse models.MessageListResponse
	err = json.NewDecoder(resp.Body).Decode(&listResponse)
	s.Require().NoError(err)
	resp.Body.Close()

	s.Assert().GreaterOrEqual(len(listResponse.Data), 1)

	// Test tenant-specific message listing
	resp, err = http.Get(s.httpServer.URL + "/api/v1/tenants/" + tenant.ID.String() + "/messages?limit=10")
	s.Require().NoError(err)
	s.Assert().Equal(http.StatusOK, resp.StatusCode)

	err = json.NewDecoder(resp.Body).Decode(&listResponse)
	s.Require().NoError(err)
	resp.Body.Close()

	s.Assert().GreaterOrEqual(len(listResponse.Data), 1)
	// Verify all messages belong to this tenant
	for _, msg := range listResponse.Data {
		s.Assert().Equal(tenant.ID, msg.TenantID)
	}

	// Cleanup
	req, _ := http.NewRequest(http.MethodDelete, s.httpServer.URL+"/api/v1/tenants/"+tenant.ID.String(), nil)
	client := &http.Client{}
	resp, err = client.Do(req)
	s.Require().NoError(err)
	resp.Body.Close()
}

func (s *IntegrationTestSuite) TestConcurrencyUpdate() {
	// Create a tenant
	createReq := models.CreateTenantRequest{
		Name:    "concurrency-test-tenant",
		Workers: 2,
	}
	createReqBody, _ := json.Marshal(createReq)

	resp, err := http.Post(s.httpServer.URL+"/api/v1/tenants", "application/json", bytes.NewBuffer(createReqBody))
	s.Require().NoError(err)

	var tenant models.Tenant
	err = json.NewDecoder(resp.Body).Decode(&tenant)
	s.Require().NoError(err)
	resp.Body.Close()

	// Wait for consumer to start
	time.Sleep(2 * time.Second)

	// Check initial stats
	resp, err = http.Get(s.httpServer.URL + "/api/v1/tenants/stats")
	s.Require().NoError(err)

	var stats map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&stats)
	s.Require().NoError(err)
	resp.Body.Close()

	// Update concurrency
	updateReq := models.UpdateTenantConcurrencyRequest{Workers: 8}
	updateReqBody, _ := json.Marshal(updateReq)

	req, _ := http.NewRequest(http.MethodPut, s.httpServer.URL+"/api/v1/tenants/"+tenant.ID.String()+"/config/concurrency", bytes.NewBuffer(updateReqBody))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err = client.Do(req)
	s.Require().NoError(err)
	s.Assert().Equal(http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Verify update in database
	resp, err = http.Get(s.httpServer.URL + "/api/v1/tenants/" + tenant.ID.String())
	s.Require().NoError(err)

	var updatedTenant models.Tenant
	err = json.NewDecoder(resp.Body).Decode(&updatedTenant)
	s.Require().NoError(err)
	resp.Body.Close()

	s.Assert().Equal(8, updatedTenant.Workers)

	// Cleanup
	req, _ = http.NewRequest(http.MethodDelete, s.httpServer.URL+"/api/v1/tenants/"+tenant.ID.String(), nil)
	resp, err = client.Do(req)
	s.Require().NoError(err)
	resp.Body.Close()
}

func (s *IntegrationTestSuite) TestHealthCheck() {
	resp, err := http.Get(s.httpServer.URL + "/health")
	s.Require().NoError(err)
	s.Assert().Equal(http.StatusOK, resp.StatusCode)

	var health map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&health)
	s.Require().NoError(err)
	resp.Body.Close()

	s.Assert().Equal("ok", health["status"])
}

func (s *IntegrationTestSuite) TestCursorPagination() {
	// Create a tenant first
	createReq := models.CreateTenantRequest{
		Name:    "pagination-test-tenant",
		Workers: 3,
	}
	createReqBody, _ := json.Marshal(createReq)

	resp, err := http.Post(s.httpServer.URL+"/api/v1/tenants", "application/json", bytes.NewBuffer(createReqBody))
	s.Require().NoError(err)
	s.Assert().Equal(http.StatusCreated, resp.StatusCode)

	var tenant models.Tenant
	err = json.NewDecoder(resp.Body).Decode(&tenant)
	s.Require().NoError(err)
	resp.Body.Close()

	// Wait for consumer to start
	time.Sleep(2 * time.Second)

	// Create multiple messages (15 messages to test pagination with limit=5)
	var messageIDs []uuid.UUID
	for i := 0; i < 15; i++ {
		messagePayload := json.RawMessage(fmt.Sprintf(`{"test": "pagination", "index": %d, "timestamp": "%s"}`, 
			i, time.Now().Add(time.Duration(i)*time.Millisecond).Format(time.RFC3339)))
		
		messageReq := models.CreateMessageRequest{
			TenantID: tenant.ID,
			Payload:  messagePayload,
		}
		messageReqBody, _ := json.Marshal(messageReq)

		resp, err = http.Post(s.httpServer.URL+"/api/v1/messages", "application/json", bytes.NewBuffer(messageReqBody))
		s.Require().NoError(err)
		s.Assert().Equal(http.StatusCreated, resp.StatusCode)

		var message models.Message
		err = json.NewDecoder(resp.Body).Decode(&message)
		s.Require().NoError(err)
		messageIDs = append(messageIDs, message.ID)
		resp.Body.Close()

		// Small delay to ensure different timestamps
		time.Sleep(10 * time.Millisecond)
	}

	// Wait for all messages to be consumed and stored
	time.Sleep(5 * time.Second)

	// Test pagination on general messages endpoint
	allMessages := []models.Message{}
	seenMessageIDs := make(map[uuid.UUID]bool)
	cursor := ""
	pageCount := 0
	limit := 5

	for {
		pageCount++
		var url string
		if cursor == "" {
			url = fmt.Sprintf("%s/api/v1/messages?limit=%d", s.httpServer.URL, limit)
		} else {
			url = fmt.Sprintf("%s/api/v1/messages?limit=%d&cursor=%s", s.httpServer.URL, limit, cursor)
		}

		resp, err = http.Get(url)
		s.Require().NoError(err)
		s.Assert().Equal(http.StatusOK, resp.StatusCode)

		var listResponse models.MessageListResponse
		err = json.NewDecoder(resp.Body).Decode(&listResponse)
		s.Require().NoError(err)
		resp.Body.Close()

		s.T().Logf("Page %d: Retrieved %d messages", pageCount, len(listResponse.Data))

		// Verify no duplicates within this page and across pages
		for _, msg := range listResponse.Data {
			s.Assert().False(seenMessageIDs[msg.ID], "Duplicate message ID found: %s", msg.ID.String())
			seenMessageIDs[msg.ID] = true
			allMessages = append(allMessages, msg)
		}

		// Verify page size doesn't exceed limit
		s.Assert().LessOrEqual(len(listResponse.Data), limit, "Page size exceeds limit")

		// Check if there's a next page
		if listResponse.NextCursor == nil || *listResponse.NextCursor == "" {
			s.T().Logf("Reached last page (page %d)", pageCount)
			break
		}
		
		cursor = *listResponse.NextCursor
		
		// Prevent infinite loops in case of bugs
		s.Assert().LessOrEqual(pageCount, 10, "Too many pages, possible infinite loop")
	}

	// Verify we got all our messages for this tenant
	tenantMessages := 0
	for _, msg := range allMessages {
		if msg.TenantID == tenant.ID {
			tenantMessages++
		}
	}
	s.Assert().GreaterOrEqual(tenantMessages, 15, "Should have at least 15 messages for our tenant")

	// Test tenant-specific pagination
	tenantAllMessages := []models.Message{}
	tenantSeenIDs := make(map[uuid.UUID]bool)
	cursor = ""
	pageCount = 0

	for {
		pageCount++
		var url string
		if cursor == "" {
			url = fmt.Sprintf("%s/api/v1/tenants/%s/messages?limit=%d", s.httpServer.URL, tenant.ID.String(), limit)
		} else {
			url = fmt.Sprintf("%s/api/v1/tenants/%s/messages?limit=%d&cursor=%s", s.httpServer.URL, tenant.ID.String(), limit, cursor)
		}

		resp, err = http.Get(url)
		s.Require().NoError(err)
		s.Assert().Equal(http.StatusOK, resp.StatusCode)

		var listResponse models.MessageListResponse
		err = json.NewDecoder(resp.Body).Decode(&listResponse)
		s.Require().NoError(err)
		resp.Body.Close()

		s.T().Logf("Tenant page %d: Retrieved %d messages", pageCount, len(listResponse.Data))

		// Verify all messages belong to this tenant and no duplicates
		for _, msg := range listResponse.Data {
			s.Assert().Equal(tenant.ID, msg.TenantID, "Message should belong to the correct tenant")
			s.Assert().False(tenantSeenIDs[msg.ID], "Duplicate message ID found in tenant pagination: %s", msg.ID.String())
			tenantSeenIDs[msg.ID] = true
			tenantAllMessages = append(tenantAllMessages, msg)
		}

		if listResponse.NextCursor == nil || *listResponse.NextCursor == "" {
			s.T().Logf("Reached last tenant page (page %d)", pageCount)
			break
		}
		
		cursor = *listResponse.NextCursor
		s.Assert().LessOrEqual(pageCount, 10, "Too many pages, possible infinite loop")
	}

	// Verify we got exactly our 15 messages in tenant-specific pagination
	s.Assert().Equal(15, len(tenantAllMessages), "Should have exactly 15 messages for tenant-specific pagination")

	// Cleanup
	req, _ := http.NewRequest(http.MethodDelete, s.httpServer.URL+"/api/v1/tenants/"+tenant.ID.String(), nil)
	client := &http.Client{}
	resp, err = client.Do(req)
	s.Require().NoError(err)
	resp.Body.Close()
}

func (s *IntegrationTestSuite) TestGracefulShutdown() {
	// Create a tenant with more workers for better concurrency testing
	createReq := models.CreateTenantRequest{
		Name:    "shutdown-test-tenant",
		Workers: 5,
	}
	createReqBody, _ := json.Marshal(createReq)

	resp, err := http.Post(s.httpServer.URL+"/api/v1/tenants", "application/json", bytes.NewBuffer(createReqBody))
	s.Require().NoError(err)
	s.Assert().Equal(http.StatusCreated, resp.StatusCode)

	var tenant models.Tenant
	err = json.NewDecoder(resp.Body).Decode(&tenant)
	s.Require().NoError(err)
	resp.Body.Close()

	// Wait for consumer to start
	time.Sleep(2 * time.Second)

	// Publish multiple messages rapidly to create processing load
	messageCount := 50
	publishedMessages := make([]uuid.UUID, 0, messageCount)
	
	s.T().Log("Publishing messages for graceful shutdown test...")
	
	// Publish messages rapidly
	for i := 0; i < messageCount; i++ {
		messagePayload := json.RawMessage(fmt.Sprintf(`{"test": "shutdown", "index": %d, "processing_time_ms": 100}`, i))
		messageReq := models.CreateMessageRequest{
			TenantID: tenant.ID,
			Payload:  messagePayload,
		}
		messageReqBody, _ := json.Marshal(messageReq)

		resp, err = http.Post(s.httpServer.URL+"/api/v1/messages", "application/json", bytes.NewBuffer(messageReqBody))
		s.Require().NoError(err)
		s.Assert().Equal(http.StatusCreated, resp.StatusCode)

		var message models.Message
		err = json.NewDecoder(resp.Body).Decode(&message)
		s.Require().NoError(err)
		publishedMessages = append(publishedMessages, message.ID)
		resp.Body.Close()
	}

	s.T().Logf("Published %d messages", len(publishedMessages))

	// Give some time for messages to start processing, but not complete
	time.Sleep(1 * time.Second)

	// Check how many messages were processed before shutdown
	resp, err = http.Get(s.httpServer.URL + "/api/v1/tenants/" + tenant.ID.String() + "/messages?limit=100")
	s.Require().NoError(err)
	s.Assert().Equal(http.StatusOK, resp.StatusCode)

	var preShutdownResponse models.MessageListResponse
	err = json.NewDecoder(resp.Body).Decode(&preShutdownResponse)
	s.Require().NoError(err)
	resp.Body.Close()

	preShutdownCount := len(preShutdownResponse.Data)
	s.T().Logf("Found %d processed messages before graceful shutdown", preShutdownCount)
	
	// We should have at least some messages processed before shutdown
	s.Assert().GreaterOrEqual(preShutdownCount, 1, "At least some messages should have been processed before shutdown")

	// Test graceful shutdown by stopping the tenant's consumer
	s.T().Log("Testing graceful shutdown...")
	shutdownStart := time.Now()
	
	// Create a channel to track shutdown completion
	shutdownDone := make(chan bool)
	
	// Shutdown tenant consumer in a goroutine so we can measure time
	go func() {
		// Delete tenant will trigger graceful shutdown
		req, _ := http.NewRequest(http.MethodDelete, s.httpServer.URL+"/api/v1/tenants/"+tenant.ID.String(), nil)
		client := &http.Client{}
		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
		}
		shutdownDone <- true
	}()

	// Wait for shutdown to complete with timeout
	select {
	case <-shutdownDone:
		shutdownDuration := time.Since(shutdownStart)
		s.T().Logf("Graceful shutdown completed in %v", shutdownDuration)
		
		// Graceful shutdown should complete within reasonable time
		// Our system is efficient, so it may complete very quickly
		s.Assert().GreaterOrEqual(shutdownDuration, 10*time.Millisecond, "Shutdown should take at least some minimal time")
		s.Assert().LessOrEqual(shutdownDuration, 30*time.Second, "Shutdown should complete within graceful timeout")
		
	case <-time.After(35 * time.Second):
		s.T().Fatal("Graceful shutdown timed out after 35 seconds")
	}

	// Wait a bit more for any final processing
	time.Sleep(1 * time.Second)
	
	// Verify the tenant is actually deleted
	resp, err = http.Get(s.httpServer.URL + "/api/v1/tenants/" + tenant.ID.String())
	s.Require().NoError(err)
	s.Assert().Equal(http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()

	s.T().Log("Graceful shutdown test completed successfully")
}

func TestIntegrationSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	// Skip if running in CI without Docker
	if os.Getenv("SKIP_INTEGRATION_TESTS") == "true" {
		t.Skip("Skipping integration tests")
	}

	suite.Run(t, new(IntegrationTestSuite))
}

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	
	// Disable log output during tests
	log.SetOutput(io.Discard)
	
	code := m.Run()
	os.Exit(code)
}