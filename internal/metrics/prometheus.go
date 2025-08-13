package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// Message processing metrics
	MessagesProcessedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "messages_processed_total",
			Help: "Total number of messages processed by tenant",
		},
		[]string{"tenant_id", "status"}, // status: success, failed, retry
	)

	MessagesProcessingDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "message_processing_duration_seconds",
			Help:    "Duration of message processing",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"tenant_id"},
	)

	// Queue metrics
	QueueDepth = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "queue_depth_messages",
			Help: "Current number of messages in queue",
		},
		[]string{"tenant_id", "queue_type"}, // queue_type: main, dlq
	)

	// Worker metrics
	ActiveWorkers = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "active_workers_total",
			Help: "Current number of active workers per tenant",
		},
		[]string{"tenant_id"},
	)

	WorkerUtilization = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "worker_utilization_ratio",
			Help: "Ratio of busy workers to total workers",
		},
		[]string{"tenant_id"},
	)

	// Tenant metrics
	ActiveTenants = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "active_tenants_total",
			Help: "Current number of active tenants",
		},
	)

	// RabbitMQ connection metrics
	RabbitMQConnections = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "rabbitmq_connections_active",
			Help: "Number of active RabbitMQ connections",
		},
		[]string{"status"}, // status: connected, disconnected
	)

	// Database metrics
	DatabaseConnections = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "database_connections_active",
			Help: "Number of active database connections",
		},
		[]string{"status"}, // status: active, idle, waiting
	)

	// API metrics
	APIRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "api_requests_total",
			Help: "Total number of API requests",
		},
		[]string{"method", "endpoint", "status_code"},
	)

	APIRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "api_request_duration_seconds",
			Help:    "Duration of API requests",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "endpoint"},
	)
)

// UpdateQueueDepth updates the queue depth metric for a tenant
func UpdateQueueDepth(tenantID, queueType string, depth float64) {
	QueueDepth.WithLabelValues(tenantID, queueType).Set(depth)
}

// UpdateActiveWorkers updates the active workers metric for a tenant
func UpdateActiveWorkers(tenantID string, count float64) {
	ActiveWorkers.WithLabelValues(tenantID).Set(count)
}

// UpdateWorkerUtilization updates worker utilization ratio
func UpdateWorkerUtilization(tenantID string, utilization float64) {
	WorkerUtilization.WithLabelValues(tenantID).Set(utilization)
}

// IncrementMessagesProcessed increments processed messages counter
func IncrementMessagesProcessed(tenantID, status string) {
	MessagesProcessedTotal.WithLabelValues(tenantID, status).Inc()
}

// RecordMessageProcessingDuration records message processing duration
func RecordMessageProcessingDuration(tenantID string, duration float64) {
	MessagesProcessingDuration.WithLabelValues(tenantID).Observe(duration)
}

// UpdateActiveTenants updates the total number of active tenants
func UpdateActiveTenants(count float64) {
	ActiveTenants.Set(count)
}

// UpdateRabbitMQConnections updates RabbitMQ connection status
func UpdateRabbitMQConnections(status string, count float64) {
	RabbitMQConnections.WithLabelValues(status).Set(count)
}

// IncrementAPIRequests increments API request counter
func IncrementAPIRequests(method, endpoint, statusCode string) {
	APIRequestsTotal.WithLabelValues(method, endpoint, statusCode).Inc()
}

// RecordAPIRequestDuration records API request duration
func RecordAPIRequestDuration(method, endpoint string, duration float64) {
	APIRequestDuration.WithLabelValues(method, endpoint).Observe(duration)
}