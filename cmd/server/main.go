//	@title			Multi-Tenant Messaging System API
//	@version		1.0
//	@description	A Go application using RabbitMQ and PostgreSQL that handles multi-tenant messaging with dynamic consumer management, partitioned data storage, and configurable concurrency.
//	@termsOfService	http://swagger.io/terms/

//	@contact.name	API Support
//	@contact.url	http://www.swagger.io/support
//	@contact.email	support@example.com

//	@license.name	MIT
//	@license.url	http://opensource.org/licenses/MIT

//	@host		localhost:8080
//	@BasePath	/api/v1

//	@securityDefinitions.apikey	BearerAuth
//	@in							header
//	@name						Authorization
//	@description				Type "Bearer" followed by a space and JWT token.

package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/galihcitta/multi-tenant-messaging-system/internal/api"
	"github.com/galihcitta/multi-tenant-messaging-system/internal/config"
	"github.com/galihcitta/multi-tenant-messaging-system/internal/repository"
	"github.com/galihcitta/multi-tenant-messaging-system/internal/services/messaging"
	"github.com/galihcitta/multi-tenant-messaging-system/internal/services/tenant"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize logger
	logger, err := initLogger(cfg.Logging.Level)
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer logger.Sync()

	logger.Info("Starting multi-tenant messaging system")

	// Initialize database
	db, err := repository.NewDatabase(cfg.Database.URL, logger)
	if err != nil {
		logger.Fatal("Failed to initialize database", zap.Error(err))
	}
	defer db.Close()

	// Initialize repositories
	tenantRepo := repository.NewTenantRepository(db.Pool(), logger)
	messageRepo := repository.NewMessageRepository(db.Pool(), logger)

	// Initialize RabbitMQ
	rabbitMQ := messaging.NewRabbitMQManager(cfg.RabbitMQ.URL, logger)
	if err := rabbitMQ.Connect(); err != nil {
		logger.Fatal("Failed to connect to RabbitMQ", zap.Error(err))
	}
	defer rabbitMQ.Close()

	// Initialize tenant manager
	tenantManager := tenant.NewManager(tenantRepo, messageRepo, rabbitMQ, cfg.Workers, logger)

	// Restore consumers for existing tenants
	if err := tenantManager.RestoreConsumers(context.Background()); err != nil {
		logger.Error("Failed to restore some consumers", zap.Error(err))
	}

	// Initialize API server
	server := api.NewServer(cfg, tenantManager, messageRepo, logger)
	server.SetupRoutes()

	// Start HTTP server
	httpServer := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler: server.GetRouter(),
	}

	go func() {
		logger.Info("Server starting", zap.String("address", httpServer.Addr))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Failed to start server", zap.Error(err))
		}
	}()

	// Wait for interrupt signal for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server...")

	// Create shutdown context
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.GracefulShutdownTimeout)
	defer cancel()

	// Shutdown tenant manager first (stops all consumers)
	logger.Info("Shutting down tenant manager...")
	if err := tenantManager.Shutdown(); err != nil {
		logger.Error("Error shutting down tenant manager", zap.Error(err))
	}

	// Shutdown HTTP server
	logger.Info("Shutting down HTTP server...")
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Fatal("Server forced to shutdown", zap.Error(err))
	}

	logger.Info("Server exited gracefully")
}

func initLogger(level string) (*zap.Logger, error) {
	var zapConfig zap.Config
	
	if gin.Mode() == gin.DebugMode {
		zapConfig = zap.NewDevelopmentConfig()
	} else {
		zapConfig = zap.NewProductionConfig()
	}
	
	switch level {
	case "debug":
		zapConfig.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	case "info":
		zapConfig.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	case "warn":
		zapConfig.Level = zap.NewAtomicLevelAt(zap.WarnLevel)
	case "error":
		zapConfig.Level = zap.NewAtomicLevelAt(zap.ErrorLevel)
	}

	return zapConfig.Build()
}