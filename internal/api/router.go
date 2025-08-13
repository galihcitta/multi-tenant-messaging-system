package api

import (
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	ginSwagger "github.com/swaggo/gin-swagger"
	swaggerFiles "github.com/swaggo/files"
	"go.uber.org/zap"

	"github.com/galihcitta/multi-tenant-messaging-system/internal/config"
	"github.com/galihcitta/multi-tenant-messaging-system/internal/middleware"
	"github.com/galihcitta/multi-tenant-messaging-system/internal/repository"
	"github.com/galihcitta/multi-tenant-messaging-system/internal/services/tenant"
	_ "github.com/galihcitta/multi-tenant-messaging-system/docs"
)

type Server struct {
	router         *gin.Engine
	config         *config.Config
	tenantHandler  *TenantHandler
	messageHandler *MessageHandler
	authHandler    *AuthHandler
	logger         *zap.Logger
}

func NewServer(
	cfg *config.Config,
	tenantManager *tenant.Manager,
	messageRepo *repository.MessageRepository,
	logger *zap.Logger,
) *Server {
	router := gin.New()
	
	// Middleware
	router.Use(gin.Logger())
	router.Use(gin.Recovery())
	router.Use(CORSMiddleware())
	
	// Add Prometheus metrics middleware if enabled
	if cfg.Metrics.Enabled {
		router.Use(middleware.PrometheusMiddleware())
	}
	
	tenantHandler := NewTenantHandlerWithManager(tenantManager, logger)
	messageHandler := NewMessageHandler(messageRepo, tenantManager, logger)
	authHandler := NewAuthHandler(logger)
	
	return &Server{
		router:         router,
		config:         cfg,
		tenantHandler:  tenantHandler,
		messageHandler: messageHandler,
		authHandler:    authHandler,
		logger:         logger,
	}
}

func (s *Server) SetupRoutes() {
	// Health check
	s.router.GET("/health", s.healthCheck)
	
	// Prometheus metrics endpoint (if enabled)
	if s.config.Metrics.Enabled {
		s.router.GET(s.config.Metrics.Path, gin.WrapH(promhttp.Handler()))
	}
	
	// Swagger documentation
	s.router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	
	// Authentication routes (no auth required)
	auth := s.router.Group("/auth")
	{
		auth.POST("/login", s.authHandler.Login)
		auth.POST("/refresh", s.authHandler.RefreshToken)
		auth.POST("/logout", middleware.JWTAuthMiddleware(), s.authHandler.Logout)
	}
	
	// API v1 routes
	v1 := s.router.Group("/api/v1")
	
	// Apply JWT middleware if authentication is required
	if s.config.Auth.RequireAuth {
		v1.Use(middleware.JWTAuthMiddleware())
	}
	
	{
		// Tenant routes
		tenants := v1.Group("/tenants")
		{
			tenants.POST("", s.tenantHandler.CreateTenant)
			tenants.GET("", s.tenantHandler.GetAllTenants)
			
			// Stats endpoint with conditional admin middleware
			if s.config.Auth.RequireAuth {
				tenants.GET("/stats", middleware.AdminOnlyMiddleware(), s.tenantHandler.GetTenantStats)
			} else {
				tenants.GET("/stats", s.tenantHandler.GetTenantStats)
			}
			
			// Tenant-specific routes with tenant authorization
			tenantAuth := tenants.Group("")
			if s.config.Auth.RequireAuth {
				tenantAuth.Use(middleware.TenantAuthMiddleware())
			}
			{
				tenantAuth.GET("/:id", s.tenantHandler.GetTenant)
				tenantAuth.DELETE("/:id", s.tenantHandler.DeleteTenant)
				tenantAuth.PUT("/:id/config/concurrency", s.tenantHandler.UpdateTenantConcurrency)
				tenantAuth.GET("/:id/messages", s.messageHandler.ListTenantMessages)
			}
		}
		
		// Message routes
		messages := v1.Group("/messages")
		{
			messages.POST("", s.messageHandler.CreateMessage)
			messages.GET("", s.messageHandler.ListMessages)
			messages.GET("/:id", s.messageHandler.GetMessage)
		}
	}
}

func (s *Server) GetRouter() *gin.Engine {
	return s.router
}

func (s *Server) healthCheck(c *gin.Context) {
	c.JSON(200, gin.H{
		"status":  "ok",
		"service": "multi-tenant-messaging-system",
	})
}

func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Header("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}