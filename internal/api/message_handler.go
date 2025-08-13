package api

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/galihcitta/multi-tenant-messaging-system/internal/models"
	"github.com/galihcitta/multi-tenant-messaging-system/internal/repository"
	"github.com/galihcitta/multi-tenant-messaging-system/internal/services/tenant"
)

type MessageHandler struct {
	messageRepo   *repository.MessageRepository
	tenantManager *tenant.Manager
	logger        *zap.Logger
}

func NewMessageHandler(messageRepo *repository.MessageRepository, tenantManager *tenant.Manager, logger *zap.Logger) *MessageHandler {
	return &MessageHandler{
		messageRepo:   messageRepo,
		tenantManager: tenantManager,
		logger:        logger,
	}
}

// CreateMessage godoc
// @Summary Create and publish a message
// @Description Create a message and publish it to the tenant's queue
// @Tags messages
// @Accept json
// @Produce json
// @Param message body models.CreateMessageRequestSwagger true "Message creation request"
// @Success 201 {object} models.MessageSwagger
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /messages [post]
func (h *MessageHandler) CreateMessage(c *gin.Context) {
	var req models.CreateMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	// Verify tenant exists
	_, err := h.tenantManager.GetTenant(c.Request.Context(), req.TenantID)
	if err != nil {
		h.logger.Error("Tenant not found for message", zap.Error(err), zap.String("tenant_id", req.TenantID.String()))
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "tenant not found"})
		return
	}

	// Create message object
	message := &models.Message{
		ID:       uuid.New(),
		TenantID: req.TenantID,
		Payload:  req.Payload,
	}

	// Serialize message for publishing
	messageData, err := json.Marshal(message)
	if err != nil {
		h.logger.Error("Failed to serialize message", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to serialize message"})
		return
	}

	// Publish message to tenant queue
	if err := h.tenantManager.PublishMessage(req.TenantID, messageData); err != nil {
		h.logger.Error("Failed to publish message", zap.Error(err), zap.String("message_id", message.ID.String()))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	h.logger.Info("Message published successfully",
		zap.String("message_id", message.ID.String()),
		zap.String("tenant_id", req.TenantID.String()))

	c.JSON(http.StatusCreated, message)
}

// GetMessage godoc
// @Summary Get a message by ID
// @Description Get message details by ID
// @Tags messages
// @Accept json
// @Produce json
// @Param id path string true "Message ID"
// @Success 200 {object} models.MessageSwagger
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /messages/{id} [get]
func (h *MessageHandler) GetMessage(c *gin.Context) {
	idParam := c.Param("id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid message ID format"})
		return
	}

	message, err := h.messageRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		h.logger.Error("Failed to get message", zap.Error(err), zap.String("id", id.String()))
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "message not found"})
		return
	}

	c.JSON(http.StatusOK, message)
}

// ListMessages godoc
// @Summary List messages with cursor pagination
// @Description Get a paginated list of all messages
// @Tags messages
// @Accept json
// @Produce json
// @Param cursor query string false "Pagination cursor"
// @Param limit query int false "Page size (max 100)" default(20)
// @Success 200 {object} models.MessageListResponseSwagger
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /messages [get]
func (h *MessageHandler) ListMessages(c *gin.Context) {
	var params models.PaginationParams
	if err := c.ShouldBindQuery(&params); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	// Set default limit if not provided
	if params.Limit <= 0 {
		params.Limit = 20
	} else if params.Limit > 100 {
		params.Limit = 100
	}

	response, err := h.messageRepo.List(c.Request.Context(), params)
	if err != nil {
		h.logger.Error("Failed to list messages", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, response)
}

// ListTenantMessages godoc
// @Summary List messages for a specific tenant
// @Description Get a paginated list of messages for a specific tenant
// @Tags messages
// @Accept json
// @Produce json
// @Param tenant_id path string true "Tenant ID"
// @Param cursor query string false "Pagination cursor"
// @Param limit query int false "Page size (max 100)" default(20)
// @Success 200 {object} models.MessageListResponseSwagger
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /tenants/{tenant_id}/messages [get]
func (h *MessageHandler) ListTenantMessages(c *gin.Context) {
	tenantIDParam := c.Param("id")
	tenantID, err := uuid.Parse(tenantIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid tenant ID format"})
		return
	}

	// Verify tenant exists
	_, err = h.tenantManager.GetTenant(c.Request.Context(), tenantID)
	if err != nil {
		h.logger.Error("Tenant not found", zap.Error(err), zap.String("tenant_id", tenantID.String()))
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "tenant not found"})
		return
	}

	var params models.PaginationParams
	if err := c.ShouldBindQuery(&params); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	// Set default limit if not provided
	if params.Limit <= 0 {
		params.Limit = 20
	} else if params.Limit > 100 {
		params.Limit = 100
	}

	response, err := h.messageRepo.ListByTenant(c.Request.Context(), tenantID, params)
	if err != nil {
		h.logger.Error("Failed to list tenant messages", zap.Error(err), zap.String("tenant_id", tenantID.String()))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, response)
}