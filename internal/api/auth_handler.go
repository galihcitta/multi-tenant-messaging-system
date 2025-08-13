package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/galihcitta/multi-tenant-messaging-system/internal/middleware"
)

type AuthHandler struct {
	logger *zap.Logger
}

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	TenantID string `json:"tenant_id,omitempty"`
}

type LoginResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

func NewAuthHandler(logger *zap.Logger) *AuthHandler {
	return &AuthHandler{
		logger: logger,
	}
}

// Login godoc
// @Summary Authenticate user and get JWT tokens
// @Description Authenticate user credentials and return access and refresh tokens
// @Tags auth
// @Accept json
// @Produce json
// @Param login body LoginRequest true "Login credentials"
// @Success 200 {object} LoginResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Router /auth/login [post]
func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	// In a real application, you would validate credentials against a database
	// For demo purposes, we'll use hardcoded credentials
	var userID, role string
	
	switch req.Username {
	case "admin":
		if req.Password != "admin123" {
			c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "Invalid credentials"})
			return
		}
		userID = "admin-user"
		role = "admin"
	case "user":
		if req.Password != "user123" {
			c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "Invalid credentials"})
			return
		}
		userID = "regular-user"
		role = "user"
		if req.TenantID == "" {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Tenant ID required for regular users"})
			return
		}
	default:
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "Invalid credentials"})
		return
	}

	// Generate tokens
	accessToken, err := middleware.GenerateToken(userID, req.TenantID, role)
	if err != nil {
		h.logger.Error("Failed to generate access token", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to generate token"})
		return
	}

	refreshToken, err := middleware.GenerateRefreshToken(userID)
	if err != nil {
		h.logger.Error("Failed to generate refresh token", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to generate refresh token"})
		return
	}

	response := LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    86400, // 24 hours in seconds
	}

	h.logger.Info("User logged in successfully", 
		zap.String("user_id", userID), 
		zap.String("role", role),
		zap.String("tenant_id", req.TenantID))

	c.JSON(http.StatusOK, response)
}

// RefreshToken godoc
// @Summary Refresh access token
// @Description Use refresh token to get a new access token
// @Tags auth
// @Accept json
// @Produce json
// @Param refresh body RefreshTokenRequest true "Refresh token"
// @Success 200 {object} LoginResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Router /auth/refresh [post]
func (h *AuthHandler) RefreshToken(c *gin.Context) {
	var req RefreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	// Validate refresh token
	claims, err := middleware.ValidateToken(req.RefreshToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "Invalid refresh token"})
		return
	}

	// Check if it's actually a refresh token
	if claims.Role != "refresh" {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "Invalid refresh token"})
		return
	}

	// For demo purposes, we'll determine the user's role and tenant
	// In a real application, this would come from a database
	var role, tenantID string
	switch claims.UserID {
	case "admin-user":
		role = "admin"
	case "regular-user":
		role = "user"
		// In a real app, you'd fetch this from database
		tenantID = claims.TenantID
	default:
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "Unknown user"})
		return
	}

	// Generate new access token
	accessToken, err := middleware.GenerateToken(claims.UserID, tenantID, role)
	if err != nil {
		h.logger.Error("Failed to generate new access token", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to generate token"})
		return
	}

	// Generate new refresh token
	newRefreshToken, err := middleware.GenerateRefreshToken(claims.UserID)
	if err != nil {
		h.logger.Error("Failed to generate new refresh token", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to generate refresh token"})
		return
	}

	response := LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    86400, // 24 hours in seconds
	}

	h.logger.Info("Token refreshed successfully", zap.String("user_id", claims.UserID))
	c.JSON(http.StatusOK, response)
}

// Logout godoc
// @Summary Logout user
// @Description Logout user (client should discard tokens)
// @Tags auth
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]string
// @Router /auth/logout [post]
func (h *AuthHandler) Logout(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if exists {
		h.logger.Info("User logged out", zap.Any("user_id", userID))
	}

	// In a real implementation, you might want to blacklist the token
	// For now, we just return success (client should discard tokens)
	c.JSON(http.StatusOK, gin.H{
		"message": "Logged out successfully",
	})
}