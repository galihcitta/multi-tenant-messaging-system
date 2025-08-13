package api

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/galihcitta/multi-tenant-messaging-system/internal/models"
	"github.com/galihcitta/multi-tenant-messaging-system/internal/services/tenant"
)

// TenantManagerInterface defines the methods required by the handler
type TenantManagerInterface interface {
	CreateTenant(ctx context.Context, req *models.CreateTenantRequest) (*models.Tenant, error)
	GetTenant(ctx context.Context, id uuid.UUID) (*models.Tenant, error)
	GetAllTenants(ctx context.Context) ([]models.Tenant, error)
	DeleteTenant(ctx context.Context, id uuid.UUID) error
	UpdateTenantConcurrency(ctx context.Context, id uuid.UUID, req *models.UpdateTenantConcurrencyRequest) error
	GetTenantStats() map[uuid.UUID]interface{}
}

type TenantHandler struct {
	tenantManager TenantManagerInterface
	logger        *zap.Logger
}

type ErrorResponse struct {
	Error string `json:"error"`
}

func NewTenantHandler(tenantManager TenantManagerInterface, logger *zap.Logger) *TenantHandler {
	return &TenantHandler{
		tenantManager: tenantManager,
		logger:        logger,
	}
}

// tenantManagerAdapter adapts the concrete Manager to the interface
type tenantManagerAdapter struct {
	manager *tenant.Manager
}

func (a *tenantManagerAdapter) CreateTenant(ctx context.Context, req *models.CreateTenantRequest) (*models.Tenant, error) {
	return a.manager.CreateTenant(ctx, req)
}

func (a *tenantManagerAdapter) GetTenant(ctx context.Context, id uuid.UUID) (*models.Tenant, error) {
	return a.manager.GetTenant(ctx, id)
}

func (a *tenantManagerAdapter) GetAllTenants(ctx context.Context) ([]models.Tenant, error) {
	return a.manager.GetAllTenants(ctx)
}

func (a *tenantManagerAdapter) DeleteTenant(ctx context.Context, id uuid.UUID) error {
	return a.manager.DeleteTenant(ctx, id)
}

func (a *tenantManagerAdapter) UpdateTenantConcurrency(ctx context.Context, id uuid.UUID, req *models.UpdateTenantConcurrencyRequest) error {
	return a.manager.UpdateTenantConcurrency(ctx, id, req)
}

func (a *tenantManagerAdapter) GetTenantStats() map[uuid.UUID]interface{} {
	stats := a.manager.GetTenantStats()
	result := make(map[uuid.UUID]interface{})
	for id, stat := range stats {
		result[id] = stat
	}
	return result
}

// NewTenantHandlerWithManager creates a handler with the concrete tenant manager
func NewTenantHandlerWithManager(tenantManager *tenant.Manager, logger *zap.Logger) *TenantHandler {
	return &TenantHandler{
		tenantManager: &tenantManagerAdapter{manager: tenantManager},
		logger:        logger,
	}
}

// CreateTenant godoc
// @Summary Create a new tenant
// @Description Create a new tenant with auto-spawned consumer
// @Tags tenants
// @Accept json
// @Produce json
// @Param tenant body models.CreateTenantRequest true "Tenant creation request"
// @Success 201 {object} models.Tenant
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /tenants [post]
func (h *TenantHandler) CreateTenant(c *gin.Context) {
	var req models.CreateTenantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	tenant, err := h.tenantManager.CreateTenant(c.Request.Context(), &req)
	if err != nil {
		h.logger.Error("Failed to create tenant", zap.Error(err), zap.String("name", req.Name))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusCreated, tenant)
}

// GetTenant godoc
// @Summary Get a tenant by ID
// @Description Get tenant details by ID
// @Tags tenants
// @Accept json
// @Produce json
// @Param id path string true "Tenant ID"
// @Success 200 {object} models.Tenant
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /tenants/{id} [get]
func (h *TenantHandler) GetTenant(c *gin.Context) {
	idParam := c.Param("id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid tenant ID format"})
		return
	}

	tenant, err := h.tenantManager.GetTenant(c.Request.Context(), id)
	if err != nil {
		h.logger.Error("Failed to get tenant", zap.Error(err), zap.String("id", id.String()))
		if err.Error() == "tenant not found: "+id.String() {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "tenant not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, tenant)
}

// GetAllTenants godoc
// @Summary Get all tenants
// @Description Get list of all tenants
// @Tags tenants
// @Accept json
// @Produce json
// @Success 200 {array} models.Tenant
// @Failure 500 {object} ErrorResponse
// @Router /tenants [get]
func (h *TenantHandler) GetAllTenants(c *gin.Context) {
	tenants, err := h.tenantManager.GetAllTenants(c.Request.Context())
	if err != nil {
		h.logger.Error("Failed to get tenants", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, tenants)
}

// DeleteTenant godoc
// @Summary Delete a tenant
// @Description Delete a tenant and stop its consumer
// @Tags tenants
// @Accept json
// @Produce json
// @Param id path string true "Tenant ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /tenants/{id} [delete]
func (h *TenantHandler) DeleteTenant(c *gin.Context) {
	idParam := c.Param("id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid tenant ID format"})
		return
	}

	err = h.tenantManager.DeleteTenant(c.Request.Context(), id)
	if err != nil {
		h.logger.Error("Failed to delete tenant", zap.Error(err), zap.String("id", id.String()))
		if err.Error() == "tenant not found: "+id.String() {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "tenant not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}

// UpdateTenantConcurrency godoc
// @Summary Update tenant worker concurrency
// @Description Update the number of workers for a tenant
// @Tags tenants
// @Accept json
// @Produce json
// @Param id path string true "Tenant ID"
// @Param concurrency body models.UpdateTenantConcurrencyRequest true "Worker count update request"
// @Success 200 {object} map[string]string
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /tenants/{id}/config/concurrency [put]
func (h *TenantHandler) UpdateTenantConcurrency(c *gin.Context) {
	idParam := c.Param("id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid tenant ID format"})
		return
	}

	var req models.UpdateTenantConcurrencyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	err = h.tenantManager.UpdateTenantConcurrency(c.Request.Context(), id, &req)
	if err != nil {
		h.logger.Error("Failed to update tenant concurrency", zap.Error(err), zap.String("id", id.String()))
		if err.Error() == "tenant not found: "+id.String() {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "tenant not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "concurrency updated successfully"})
}

// GetTenantStats godoc
// @Summary Get tenant statistics
// @Description Get statistics for all active tenants
// @Tags tenants
// @Accept json
// @Produce json
// @Success 200 {object} map[string]tenant.TenantStats
// @Router /tenants/stats [get]
func (h *TenantHandler) GetTenantStats(c *gin.Context) {
	stats := h.tenantManager.GetTenantStats()
	c.JSON(http.StatusOK, stats)
}