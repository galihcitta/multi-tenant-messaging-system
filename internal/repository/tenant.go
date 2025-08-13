package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/galihcitta/multi-tenant-messaging-system/internal/models"
)

type TenantRepository struct {
	db     *pgxpool.Pool
	logger *zap.Logger
}

func NewTenantRepository(db *pgxpool.Pool, logger *zap.Logger) *TenantRepository {
	return &TenantRepository{
		db:     db,
		logger: logger,
	}
}

func (r *TenantRepository) Create(ctx context.Context, tenant *models.Tenant) error {
	query := `
		INSERT INTO tenants (id, name, workers)
		VALUES ($1, $2, $3)
		RETURNING created_at, updated_at`

	row := r.db.QueryRow(ctx, query, tenant.ID, tenant.Name, tenant.Workers)
	
	if err := row.Scan(&tenant.CreatedAt, &tenant.UpdatedAt); err != nil {
		r.logger.Error("Failed to create tenant", zap.Error(err), zap.String("name", tenant.Name))
		return fmt.Errorf("failed to create tenant: %w", err)
	}

	// Create partition for this tenant in messages table
	if err := r.createMessagePartition(ctx, tenant.ID); err != nil {
		r.logger.Error("Failed to create message partition", zap.Error(err), zap.String("tenant_id", tenant.ID.String()))
		// Note: We might want to rollback the tenant creation here
		return fmt.Errorf("failed to create message partition: %w", err)
	}

	r.logger.Info("Tenant created successfully", zap.String("id", tenant.ID.String()), zap.String("name", tenant.Name))
	return nil
}

func (r *TenantRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Tenant, error) {
	query := `SELECT id, name, workers, created_at, updated_at FROM tenants WHERE id = $1`
	
	var tenant models.Tenant
	row := r.db.QueryRow(ctx, query, id)
	
	err := row.Scan(&tenant.ID, &tenant.Name, &tenant.Workers, &tenant.CreatedAt, &tenant.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("tenant not found: %s", id)
		}
		return nil, fmt.Errorf("failed to get tenant: %w", err)
	}

	return &tenant, nil
}

func (r *TenantRepository) GetAll(ctx context.Context) ([]models.Tenant, error) {
	query := `SELECT id, name, workers, created_at, updated_at FROM tenants ORDER BY created_at DESC`
	
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query tenants: %w", err)
	}
	defer rows.Close()

	var tenants []models.Tenant
	for rows.Next() {
		var tenant models.Tenant
		if err := rows.Scan(&tenant.ID, &tenant.Name, &tenant.Workers, &tenant.CreatedAt, &tenant.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan tenant: %w", err)
		}
		tenants = append(tenants, tenant)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	return tenants, nil
}

func (r *TenantRepository) UpdateWorkers(ctx context.Context, id uuid.UUID, workers int) error {
	query := `UPDATE tenants SET workers = $1, updated_at = NOW() WHERE id = $2`
	
	result, err := r.db.Exec(ctx, query, workers, id)
	if err != nil {
		return fmt.Errorf("failed to update tenant workers: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("tenant not found: %s", id)
	}

	r.logger.Info("Tenant workers updated", zap.String("id", id.String()), zap.Int("workers", workers))
	return nil
}

func (r *TenantRepository) Delete(ctx context.Context, id uuid.UUID) error {
	// First drop the message partition
	if err := r.dropMessagePartition(ctx, id); err != nil {
		r.logger.Error("Failed to drop message partition", zap.Error(err), zap.String("tenant_id", id.String()))
		// Continue with tenant deletion even if partition drop fails
	}

	query := `DELETE FROM tenants WHERE id = $1`
	
	result, err := r.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete tenant: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("tenant not found: %s", id)
	}

	r.logger.Info("Tenant deleted successfully", zap.String("id", id.String()))
	return nil
}

func (r *TenantRepository) createMessagePartition(ctx context.Context, tenantID uuid.UUID) error {
	partitionName := fmt.Sprintf("messages_%s", strings.ReplaceAll(tenantID.String(), "-", "_"))
	query := fmt.Sprintf(`
		CREATE TABLE %s PARTITION OF messages
		FOR VALUES IN ('%s')`, partitionName, tenantID.String())

	_, err := r.db.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to create partition %s: %w", partitionName, err)
	}

	r.logger.Info("Message partition created", zap.String("partition", partitionName))
	return nil
}

func (r *TenantRepository) dropMessagePartition(ctx context.Context, tenantID uuid.UUID) error {
	partitionName := fmt.Sprintf("messages_%s", strings.ReplaceAll(tenantID.String(), "-", "_"))
	query := fmt.Sprintf("DROP TABLE IF EXISTS %s", partitionName)

	_, err := r.db.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to drop partition %s: %w", partitionName, err)
	}

	r.logger.Info("Message partition dropped", zap.String("partition", partitionName))
	return nil
}