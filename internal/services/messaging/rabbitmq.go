package messaging

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

type RabbitMQManager struct {
	url        string
	connection *amqp.Connection
	logger     *zap.Logger
	mutex      sync.RWMutex
	channels   map[string]*amqp.Channel
	done       chan struct{}
}

type ChannelManager struct {
	channel  *amqp.Channel
	logger   *zap.Logger
	closed   bool
	mutex    sync.RWMutex
}

func NewRabbitMQManager(url string, logger *zap.Logger) *RabbitMQManager {
	return &RabbitMQManager{
		url:      url,
		logger:   logger,
		channels: make(map[string]*amqp.Channel),
		done:     make(chan struct{}),
	}
}

func (r *RabbitMQManager) Connect() error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.connection != nil && !r.connection.IsClosed() {
		return nil // Already connected
	}

	var conn *amqp.Connection
	var err error

	// Retry connection with exponential backoff
	for attempt := 1; attempt <= 5; attempt++ {
		conn, err = amqp.Dial(r.url)
		if err == nil {
			break
		}

		r.logger.Warn("Failed to connect to RabbitMQ, retrying...",
			zap.Int("attempt", attempt),
			zap.Error(err))

		if attempt < 5 {
			backoff := time.Duration(attempt) * time.Second
			time.Sleep(backoff)
		}
	}

	if err != nil {
		return fmt.Errorf("failed to connect to RabbitMQ after 5 attempts: %w", err)
	}

	r.connection = conn
	r.logger.Info("Successfully connected to RabbitMQ")

	// Start connection monitor
	go r.monitorConnection()

	return nil
}

func (r *RabbitMQManager) Close() error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	close(r.done)

	// Close all channels
	for _, ch := range r.channels {
		if ch != nil && !ch.IsClosed() {
			ch.Close()
		}
	}
	r.channels = make(map[string]*amqp.Channel)

	// Close connection
	if r.connection != nil && !r.connection.IsClosed() {
		if err := r.connection.Close(); err != nil {
			r.logger.Error("Error closing RabbitMQ connection", zap.Error(err))
			return err
		}
	}

	r.logger.Info("RabbitMQ connection closed")
	return nil
}

func (r *RabbitMQManager) GetChannel(channelID string) (*amqp.Channel, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.connection == nil || r.connection.IsClosed() {
		return nil, fmt.Errorf("RabbitMQ connection is not available")
	}

	// Check if channel exists and is still open
	if ch, exists := r.channels[channelID]; exists {
		if ch != nil && !ch.IsClosed() {
			return ch, nil
		}
		// Remove closed channel
		delete(r.channels, channelID)
	}

	// Create new channel
	ch, err := r.connection.Channel()
	if err != nil {
		return nil, fmt.Errorf("failed to create channel: %w", err)
	}

	r.channels[channelID] = ch
	return ch, nil
}

func (r *RabbitMQManager) CloseChannel(channelID string) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if ch, exists := r.channels[channelID]; exists {
		delete(r.channels, channelID)
		if ch != nil && !ch.IsClosed() {
			return ch.Close()
		}
	}
	return nil
}

func (r *RabbitMQManager) CreateQueue(queueName string) error {
	ch, err := r.GetChannel("queue_management")
	if err != nil {
		return fmt.Errorf("failed to get channel for queue creation: %w", err)
	}

	// Create dead letter queue first
	dlqName := queueName + ".dlq"
	_, err = ch.QueueDeclare(
		dlqName,   // name
		true,      // durable
		false,     // delete when unused
		false,     // exclusive
		false,     // no-wait
		nil,       // arguments
	)
	if err != nil {
		return fmt.Errorf("failed to declare dead letter queue %s: %w", dlqName, err)
	}

	// Create main queue with DLQ configuration
	args := amqp.Table{
		"x-dead-letter-exchange":    "",
		"x-dead-letter-routing-key": dlqName,
		"x-message-ttl":             30000, // 30 seconds TTL for failed messages
		"x-max-retries":             3,     // Maximum retry attempts
	}

	_, err = ch.QueueDeclare(
		queueName, // name
		true,      // durable
		false,     // delete when unused
		false,     // exclusive
		false,     // no-wait
		args,      // arguments with DLQ config
	)
	if err != nil {
		return fmt.Errorf("failed to declare queue %s: %w", queueName, err)
	}

	r.logger.Info("Queue and DLQ created successfully", 
		zap.String("queue", queueName),
		zap.String("dlq", dlqName))
	return nil
}

func (r *RabbitMQManager) DeleteQueue(queueName string) error {
	ch, err := r.GetChannel("queue_management")
	if err != nil {
		return fmt.Errorf("failed to get channel for queue deletion: %w", err)
	}

	// Delete main queue
	_, err = ch.QueueDelete(queueName, false, false, false)
	if err != nil {
		return fmt.Errorf("failed to delete queue %s: %w", queueName, err)
	}

	// Delete dead letter queue
	dlqName := queueName + ".dlq"
	_, err = ch.QueueDelete(dlqName, false, false, false)
	if err != nil {
		r.logger.Warn("Failed to delete dead letter queue", zap.String("dlq", dlqName), zap.Error(err))
		// Continue even if DLQ deletion fails
	}

	r.logger.Info("Queue and DLQ deleted successfully", 
		zap.String("queue", queueName),
		zap.String("dlq", dlqName))
	return nil
}

func (r *RabbitMQManager) PublishMessage(queueName string, message []byte) error {
	return r.PublishMessageWithRetry(queueName, message, 0)
}

func (r *RabbitMQManager) PublishMessageWithRetry(queueName string, message []byte, retryCount int) error {
	ch, err := r.GetChannel("publisher")
	if err != nil {
		return fmt.Errorf("failed to get channel for publishing: %w", err)
	}

	headers := amqp.Table{
		"x-retry-count": retryCount,
		"x-original-queue": queueName,
	}

	err = ch.PublishWithContext(
		context.Background(),
		"",        // exchange
		queueName, // routing key
		false,     // mandatory
		false,     // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			Body:         message,
			DeliveryMode: amqp.Persistent, // Make message persistent
			MessageId:    uuid.New().String(),
			Timestamp:    time.Now(),
			Headers:      headers,
		})

	if err != nil {
		return fmt.Errorf("failed to publish message to queue %s: %w", queueName, err)
	}

	if retryCount > 0 {
		r.logger.Info("Message republished with retry", 
			zap.String("queue", queueName),
			zap.Int("retry_count", retryCount))
	} else {
		r.logger.Debug("Message published successfully", zap.String("queue", queueName))
	}
	return nil
}

func (r *RabbitMQManager) RetryFailedMessage(dlqName string, originalQueueName string, message []byte, retryCount int) error {
	if retryCount >= 3 {
		r.logger.Error("Maximum retry attempts reached, message will remain in DLQ",
			zap.String("original_queue", originalQueueName),
			zap.String("dlq", dlqName),
			zap.Int("retry_count", retryCount))
		return fmt.Errorf("maximum retry attempts reached")
	}

	// Calculate exponential backoff delay
	backoffDelay := time.Duration(1<<retryCount) * time.Second // 1s, 2s, 4s

	r.logger.Info("Retrying failed message after delay",
		zap.String("original_queue", originalQueueName),
		zap.Duration("delay", backoffDelay),
		zap.Int("retry_count", retryCount+1))

	// Wait for backoff period
	time.Sleep(backoffDelay)

	// Republish to original queue with incremented retry count
	return r.PublishMessageWithRetry(originalQueueName, message, retryCount+1)
}

func (r *RabbitMQManager) monitorConnection() {
	closeChan := make(chan *amqp.Error)
	if r.connection != nil {
		r.connection.NotifyClose(closeChan)
	}

	select {
	case <-r.done:
		return
	case err := <-closeChan:
		if err != nil {
			r.logger.Error("RabbitMQ connection lost", zap.Error(err))
			// Attempt to reconnect
			r.attemptReconnect()
		}
	}
}

func (r *RabbitMQManager) attemptReconnect() {
	r.logger.Info("Attempting to reconnect to RabbitMQ...")
	
	// Clear existing connection
	r.mutex.Lock()
	r.connection = nil
	// Clear all channels as they are now invalid
	for id := range r.channels {
		delete(r.channels, id)
	}
	r.mutex.Unlock()

	// Retry connection
	for {
		select {
		case <-r.done:
			return
		default:
			if err := r.Connect(); err != nil {
				r.logger.Error("Failed to reconnect to RabbitMQ", zap.Error(err))
				time.Sleep(5 * time.Second)
				continue
			}
			r.logger.Info("Successfully reconnected to RabbitMQ")
			return
		}
	}
}

func (r *RabbitMQManager) IsConnected() bool {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	
	return r.connection != nil && !r.connection.IsClosed()
}

func GetTenantQueueName(tenantID uuid.UUID) string {
	return fmt.Sprintf("tenant_%s_queue", tenantID.String())
}