package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"

	"github.com/galihcitta/multi-tenant-messaging-system/internal/models"
	"github.com/galihcitta/multi-tenant-messaging-system/internal/repository"
)

type WorkerPool struct {
	tenantID        uuid.UUID
	queueName       string
	workerCount     int32
	messageRepo     *repository.MessageRepository
	rabbitMQ        *RabbitMQManager
	logger          *zap.Logger
	ctx             context.Context
	cancel          context.CancelFunc
	wg              sync.WaitGroup
	jobs            chan []byte
	stopChan        chan struct{}
	workerCountMux  sync.RWMutex
}

type MessageHandler interface {
	HandleMessage(ctx context.Context, message *models.Message) error
}

type DefaultMessageHandler struct {
	messageRepo *repository.MessageRepository
	logger      *zap.Logger
}

func NewDefaultMessageHandler(messageRepo *repository.MessageRepository, logger *zap.Logger) *DefaultMessageHandler {
	return &DefaultMessageHandler{
		messageRepo: messageRepo,
		logger:      logger,
	}
}

func (h *DefaultMessageHandler) HandleMessage(ctx context.Context, message *models.Message) error {
	// Store the message in the database
	if err := h.messageRepo.Create(ctx, message); err != nil {
		h.logger.Error("Failed to store message", zap.Error(err), zap.String("message_id", message.ID.String()))
		return fmt.Errorf("failed to store message: %w", err)
	}

	h.logger.Info("Message processed successfully",
		zap.String("message_id", message.ID.String()),
		zap.String("tenant_id", message.TenantID.String()))

	return nil
}

func NewWorkerPool(tenantID uuid.UUID, workerCount int, messageRepo *repository.MessageRepository, rabbitMQ *RabbitMQManager, logger *zap.Logger) *WorkerPool {
	ctx, cancel := context.WithCancel(context.Background())
	queueName := GetTenantQueueName(tenantID)
	
	return &WorkerPool{
		tenantID:    tenantID,
		queueName:   queueName,
		workerCount: int32(workerCount),
		messageRepo: messageRepo,
		rabbitMQ:    rabbitMQ,
		logger:      logger,
		ctx:         ctx,
		cancel:      cancel,
		jobs:        make(chan []byte, workerCount*2), // Buffer to prevent blocking
		stopChan:    make(chan struct{}),
	}
}

func (wp *WorkerPool) Start() error {
	// Create the queue if it doesn't exist
	if err := wp.rabbitMQ.CreateQueue(wp.queueName); err != nil {
		return fmt.Errorf("failed to create queue: %w", err)
	}

	// Start consumer goroutine
	wp.wg.Add(1)
	go wp.consumer()

	// Start worker goroutines
	currentWorkerCount := atomic.LoadInt32(&wp.workerCount)
	for i := int32(0); i < currentWorkerCount; i++ {
		wp.wg.Add(1)
		go wp.worker(int(i))
	}

	wp.logger.Info("Worker pool started",
		zap.String("tenant_id", wp.tenantID.String()),
		zap.String("queue", wp.queueName),
		zap.Int32("workers", currentWorkerCount))

	return nil
}

func (wp *WorkerPool) Stop() error {
	wp.logger.Info("Stopping worker pool", zap.String("tenant_id", wp.tenantID.String()))

	// Signal stop
	close(wp.stopChan)
	wp.cancel()

	// Close jobs channel
	close(wp.jobs)

	// Wait for all goroutines to finish
	wp.wg.Wait()

	// Delete the queue
	if err := wp.rabbitMQ.DeleteQueue(wp.queueName); err != nil {
		wp.logger.Error("Failed to delete queue", zap.Error(err), zap.String("queue", wp.queueName))
	}

	wp.logger.Info("Worker pool stopped", zap.String("tenant_id", wp.tenantID.String()))
	return nil
}

func (wp *WorkerPool) UpdateWorkerCount(newCount int) {
	if newCount <= 0 {
		return
	}

	wp.workerCountMux.Lock()
	defer wp.workerCountMux.Unlock()

	oldCount := atomic.LoadInt32(&wp.workerCount)
	atomic.StoreInt32(&wp.workerCount, int32(newCount))

	wp.logger.Info("Worker count updated",
		zap.String("tenant_id", wp.tenantID.String()),
		zap.Int32("old_count", oldCount),
		zap.Int("new_count", newCount))

	// Note: In a production system, you might want to gracefully add/remove workers
	// For now, the change will take effect for new messages
}

func (wp *WorkerPool) consumer() {
	defer wp.wg.Done()

	channelID := fmt.Sprintf("consumer_%s", wp.tenantID.String())
	ch, err := wp.rabbitMQ.GetChannel(channelID)
	if err != nil {
		wp.logger.Error("Failed to get channel for consumer", zap.Error(err))
		return
	}
	defer wp.rabbitMQ.CloseChannel(channelID)

	// Set QoS to limit number of unacknowledged messages
	err = ch.Qos(int(atomic.LoadInt32(&wp.workerCount)), 0, false)
	if err != nil {
		wp.logger.Error("Failed to set QoS", zap.Error(err))
		return
	}

	var msgs <-chan amqp.Delivery
	msgs, err = ch.Consume(
		wp.queueName,              // queue
		fmt.Sprintf("consumer_%s", wp.tenantID.String()), // consumer tag
		false, // auto-ack
		false, // exclusive
		false, // no-local
		false, // no-wait
		nil,   // args
	)
	if err != nil {
		wp.logger.Error("Failed to start consuming", zap.Error(err))
		return
	}

	for {
		select {
		case <-wp.stopChan:
			return
		case <-wp.ctx.Done():
			return
		case msg, ok := <-msgs:
			if !ok {
				wp.logger.Warn("Message channel closed")
				return
			}

			// Send job to worker pool
			select {
			case wp.jobs <- msg.Body:
				// Acknowledge the message after sending to worker
				msg.Ack(false)
			case <-wp.stopChan:
				msg.Nack(false, true) // Reject and requeue
				return
			case <-wp.ctx.Done():
				msg.Nack(false, true) // Reject and requeue
				return
			case <-time.After(5 * time.Second):
				// Timeout - reject and requeue the message
				wp.logger.Warn("Worker pool full, rejecting message")
				msg.Nack(false, true)
			}
		}
	}
}

func (wp *WorkerPool) worker(workerID int) {
	defer wp.wg.Done()

	handler := NewDefaultMessageHandler(wp.messageRepo, wp.logger)

	wp.logger.Debug("Worker started", zap.Int("worker_id", workerID), zap.String("tenant_id", wp.tenantID.String()))

	for {
		select {
		case <-wp.ctx.Done():
			wp.logger.Debug("Worker stopping", zap.Int("worker_id", workerID))
			return
		case job, ok := <-wp.jobs:
			if !ok {
				wp.logger.Debug("Jobs channel closed, worker stopping", zap.Int("worker_id", workerID))
				return
			}

			// Process the message
			if err := wp.processMessage(handler, job); err != nil {
				wp.logger.Error("Failed to process message",
					zap.Error(err),
					zap.Int("worker_id", workerID),
					zap.String("tenant_id", wp.tenantID.String()))
			}
		}
	}
}

func (wp *WorkerPool) processMessage(handler MessageHandler, messageData []byte) error {
	// Parse the message
	var message models.Message
	if err := json.Unmarshal(messageData, &message); err != nil {
		return fmt.Errorf("failed to unmarshal message: %w", err)
	}

	// Set tenant ID if not set
	if message.TenantID == uuid.Nil {
		message.TenantID = wp.tenantID
	}

	// Set ID and timestamp if not set
	if message.ID == uuid.Nil {
		message.ID = uuid.New()
	}
	if message.CreatedAt.IsZero() {
		message.CreatedAt = time.Now()
	}

	// Process the message
	return handler.HandleMessage(wp.ctx, &message)
}

func (wp *WorkerPool) GetWorkerCount() int {
	return int(atomic.LoadInt32(&wp.workerCount))
}

func (wp *WorkerPool) GetQueueName() string {
	return wp.queueName
}