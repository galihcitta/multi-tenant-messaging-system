package tenant

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/galihcitta/multi-tenant-messaging-system/internal/models"
	"github.com/galihcitta/multi-tenant-messaging-system/internal/repository"
	"github.com/galihcitta/multi-tenant-messaging-system/internal/services/messaging"
)

type Manager struct {
	tenantRepo   *repository.TenantRepository
	messageRepo  *repository.MessageRepository
	rabbitMQ     *messaging.RabbitMQManager
	logger       *zap.Logger
	workerPools  map[uuid.UUID]*messaging.WorkerPool
	poolsMutex   sync.RWMutex
	defaultWorkers int
}

func NewManager(
	tenantRepo *repository.TenantRepository,
	messageRepo *repository.MessageRepository,
	rabbitMQ *messaging.RabbitMQManager,
	defaultWorkers int,
	logger *zap.Logger,
) *Manager {
	return &Manager{
		tenantRepo:     tenantRepo,
		messageRepo:    messageRepo,
		rabbitMQ:       rabbitMQ,
		logger:         logger,
		workerPools:    make(map[uuid.UUID]*messaging.WorkerPool),
		defaultWorkers: defaultWorkers,
	}
}

func (m *Manager) CreateTenant(ctx context.Context, req *models.CreateTenantRequest) (*models.Tenant, error) {
	tenant := &models.Tenant{
		ID:      uuid.New(),
		Name:    req.Name,
		Workers: req.Workers,
	}

	// Set default worker count if not specified
	if tenant.Workers <= 0 {
		tenant.Workers = m.defaultWorkers
	}

	// Create tenant in database
	if err := m.tenantRepo.Create(ctx, tenant); err != nil {
		return nil, fmt.Errorf("failed to create tenant in database: %w", err)
	}

	// Spawn consumer for the new tenant
	if err := m.spawnConsumer(tenant); err != nil {
		// If consumer creation fails, try to cleanup the tenant
		if deleteErr := m.tenantRepo.Delete(ctx, tenant.ID); deleteErr != nil {
			m.logger.Error("Failed to cleanup tenant after consumer creation failure",
				zap.Error(deleteErr),
				zap.String("tenant_id", tenant.ID.String()))
		}
		return nil, fmt.Errorf("failed to spawn consumer: %w", err)
	}

	m.logger.Info("Tenant created successfully",
		zap.String("tenant_id", tenant.ID.String()),
		zap.String("name", tenant.Name),
		zap.Int("workers", tenant.Workers))

	return tenant, nil
}

func (m *Manager) GetTenant(ctx context.Context, id uuid.UUID) (*models.Tenant, error) {
	return m.tenantRepo.GetByID(ctx, id)
}

func (m *Manager) GetAllTenants(ctx context.Context) ([]models.Tenant, error) {
	return m.tenantRepo.GetAll(ctx)
}

func (m *Manager) DeleteTenant(ctx context.Context, id uuid.UUID) error {
	// Stop the consumer
	if err := m.stopConsumer(id); err != nil {
		m.logger.Error("Failed to stop consumer", zap.Error(err), zap.String("tenant_id", id.String()))
		// Continue with tenant deletion even if consumer stopping fails
	}

	// Delete tenant from database
	if err := m.tenantRepo.Delete(ctx, id); err != nil {
		return fmt.Errorf("failed to delete tenant from database: %w", err)
	}

	m.logger.Info("Tenant deleted successfully", zap.String("tenant_id", id.String()))
	return nil
}

func (m *Manager) UpdateTenantConcurrency(ctx context.Context, id uuid.UUID, req *models.UpdateTenantConcurrencyRequest) error {
	// Update in database
	if err := m.tenantRepo.UpdateWorkers(ctx, id, req.Workers); err != nil {
		return fmt.Errorf("failed to update tenant workers in database: %w", err)
	}

	// Update the worker pool if it's running
	m.poolsMutex.RLock()
	pool, exists := m.workerPools[id]
	m.poolsMutex.RUnlock()

	if exists {
		pool.UpdateWorkerCount(req.Workers)
		m.logger.Info("Updated worker pool concurrency",
			zap.String("tenant_id", id.String()),
			zap.Int("workers", req.Workers))
	}

	return nil
}

func (m *Manager) spawnConsumer(tenant *models.Tenant) error {
	m.poolsMutex.Lock()
	defer m.poolsMutex.Unlock()

	// Check if worker pool already exists
	if _, exists := m.workerPools[tenant.ID]; exists {
		return fmt.Errorf("consumer already exists for tenant %s", tenant.ID)
	}

	// Create worker pool
	workerPool := messaging.NewWorkerPool(
		tenant.ID,
		tenant.Workers,
		m.messageRepo,
		m.rabbitMQ,
		m.logger,
	)

	// Start the worker pool
	if err := workerPool.Start(); err != nil {
		return fmt.Errorf("failed to start worker pool: %w", err)
	}

	// Store the worker pool
	m.workerPools[tenant.ID] = workerPool

	m.logger.Info("Consumer spawned successfully",
		zap.String("tenant_id", tenant.ID.String()),
		zap.String("queue", workerPool.GetQueueName()),
		zap.Int("workers", workerPool.GetWorkerCount()))

	return nil
}

func (m *Manager) stopConsumer(tenantID uuid.UUID) error {
	m.poolsMutex.Lock()
	defer m.poolsMutex.Unlock()

	workerPool, exists := m.workerPools[tenantID]
	if !exists {
		return fmt.Errorf("no consumer found for tenant %s", tenantID)
	}

	// Stop the worker pool
	if err := workerPool.Stop(); err != nil {
		m.logger.Error("Error stopping worker pool", zap.Error(err), zap.String("tenant_id", tenantID.String()))
		// Don't return error, continue with cleanup
	}

	// Remove from map
	delete(m.workerPools, tenantID)

	m.logger.Info("Consumer stopped successfully", zap.String("tenant_id", tenantID.String()))
	return nil
}

func (m *Manager) PublishMessage(tenantID uuid.UUID, message []byte) error {
	queueName := messaging.GetTenantQueueName(tenantID)
	
	if err := m.rabbitMQ.PublishMessage(queueName, message); err != nil {
		return fmt.Errorf("failed to publish message to tenant %s: %w", tenantID, err)
	}

	m.logger.Debug("Message published to tenant queue",
		zap.String("tenant_id", tenantID.String()),
		zap.String("queue", queueName))

	return nil
}

func (m *Manager) RestoreConsumers(ctx context.Context) error {
	m.logger.Info("Restoring consumers for existing tenants")

	tenants, err := m.tenantRepo.GetAll(ctx)
	if err != nil {
		return fmt.Errorf("failed to get existing tenants: %w", err)
	}

	var errors []string
	for _, tenant := range tenants {
		if err := m.spawnConsumer(&tenant); err != nil {
			errorMsg := fmt.Sprintf("failed to restore consumer for tenant %s: %v", tenant.ID, err)
			errors = append(errors, errorMsg)
			m.logger.Error("Failed to restore consumer", zap.Error(err), zap.String("tenant_id", tenant.ID.String()))
		}
	}

	if len(errors) > 0 {
		m.logger.Warn("Some consumers failed to restore", zap.Strings("errors", errors))
	}

	m.logger.Info("Consumer restoration completed", zap.Int("total_tenants", len(tenants)))
	return nil
}

func (m *Manager) Shutdown() error {
	m.logger.Info("Shutting down tenant manager")

	m.poolsMutex.Lock()
	defer m.poolsMutex.Unlock()

	// Stop all worker pools
	var errors []error
	for tenantID, workerPool := range m.workerPools {
		if err := workerPool.Stop(); err != nil {
			errors = append(errors, fmt.Errorf("failed to stop worker pool for tenant %s: %w", tenantID, err))
		}
	}

	// Clear the map
	m.workerPools = make(map[uuid.UUID]*messaging.WorkerPool)

	if len(errors) > 0 {
		return fmt.Errorf("errors occurred during shutdown: %v", errors)
	}

	m.logger.Info("Tenant manager shutdown completed")
	return nil
}

func (m *Manager) GetTenantStats() map[uuid.UUID]*TenantStats {
	m.poolsMutex.RLock()
	defer m.poolsMutex.RUnlock()

	stats := make(map[uuid.UUID]*TenantStats)
	for tenantID, workerPool := range m.workerPools {
		stats[tenantID] = &TenantStats{
			TenantID:    tenantID,
			QueueName:   workerPool.GetQueueName(),
			WorkerCount: workerPool.GetWorkerCount(),
		}
	}

	return stats
}

type TenantStats struct {
	TenantID    uuid.UUID `json:"tenant_id"`
	QueueName   string    `json:"queue_name"`
	WorkerCount int       `json:"worker_count"`
}