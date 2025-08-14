# Multi-Tenant Messaging System

**Author**: Galih Citta Surya Prasetya

A Go application using RabbitMQ and PostgreSQL that handles multi-tenant messaging with dynamic consumer management, partitioned data storage, and configurable concurrency.

## Features

- **Auto-Spawn Tenant Consumers**: Automatically creates dedicated RabbitMQ queues and consumers when tenants are created
- **Auto-Stop Tenant Consumers**: Gracefully shuts down consumers and cleans up queues when tenants are deleted
- **Partitioned Message Storage**: Uses PostgreSQL table partitioning for efficient data isolation by tenant
- **Configurable Worker Concurrency**: Adjustable worker pools per tenant for optimal performance
- **Graceful Shutdown**: Processes ongoing transactions before stopping
- **Cursor Pagination**: Efficient pagination for message listing APIs
- **Swagger Documentation**: Auto-generated API documentation
- **Health Monitoring**: Built-in health checks and metrics

## Quick Start

### Using Docker Compose (Recommended)

1. **Start the services**:
   ```bash
   docker-compose up -d
   ```

2. **Check service health**:
   ```bash
   curl http://localhost:8080/health
   ```

3. **Access Swagger UI**:
   Open http://localhost:8080/swagger/index.html in your browser

4. **Access RabbitMQ Management**:
   Open http://localhost:15672 (user: `user`, password: `pass`)

### Manual Setup

1. **Prerequisites**:
   - Go 1.23+
   - PostgreSQL 13+
   - RabbitMQ 3.12+

2. **Database Setup**:
   ```bash
   # Run migrations
   migrate -path migrations -database "postgres://user:pass@localhost:5432/messaging_system?sslmode=disable" up
   ```

3. **Start the application**:
   ```bash
   go run cmd/server/main.go
   ```

## API Endpoints

### Authentication

- **POST /auth/login** - Authenticate and get JWT tokens
- **POST /auth/refresh** - Refresh access token using refresh token
- **POST /auth/logout** - Logout (requires authentication)

**Demo Credentials**:
- Admin: `username: admin`, `password: admin123`
- User: `username: user`, `password: user123` (requires `tenant_id`)

### Tenant Management

- **POST /api/v1/tenants** - Create a new tenant
- **GET /api/v1/tenants** - List all tenants
- **GET /api/v1/tenants/{id}** - Get tenant details
- **DELETE /api/v1/tenants/{id}** - Delete a tenant
- **PUT /api/v1/tenants/{id}/config/concurrency** - Update worker concurrency
- **GET /api/v1/tenants/stats** - Get tenant statistics (admin only when auth enabled)

### Message Management

- **POST /api/v1/messages** - Create and publish a message
- **GET /api/v1/messages** - List messages with cursor pagination
- **GET /api/v1/messages/{id}** - Get message details
- **GET /api/v1/tenants/{tenant_id}/messages** - List messages for a specific tenant

## Configuration

Configuration can be provided via `config.yaml` file or environment variables:

```yaml
rabbitmq:
  url: amqp://user:pass@localhost:5672/

database:
  url: postgres://user:pass@localhost:5432/messaging_system

server:
  host: localhost
  port: 8080

workers: 3  # Default worker count per tenant

logging:
  level: info
  format: json

auth:
  jwt_secret: your-secret-key
  token_expiry: 24h
  refresh_expiry: 168h
  require_auth: false
  allow_registration: false

metrics:
  enabled: true
  path: /metrics
  port: 8080
  update_interval: 10s

graceful_shutdown_timeout: 30s
```

### Environment Variables

- `RABBITMQ_URL` - RabbitMQ connection URL
- `DATABASE_URL` - PostgreSQL connection URL
- `SERVER_HOST` - Server bind host
- `SERVER_PORT` - Server bind port
- `WORKERS` - Default worker count per tenant
- `LOGGING_LEVEL` - Log level (debug, info, warn, error)
- `AUTH_JWT_SECRET` - JWT signing secret
- `AUTH_REQUIRE_AUTH` - Enable/disable authentication
- `METRICS_ENABLED` - Enable/disable Prometheus metrics
- `GRACEFUL_SHUTDOWN_TIMEOUT` - Shutdown timeout duration

## Usage Examples

### Authentication

#### Login (Admin User)
```bash
curl -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "username": "admin",
    "password": "admin123"
  }'
```

#### Login (Regular User)
```bash
curl -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "username": "user",
    "password": "user123",
    "tenant_id": "your-tenant-id"
  }'
```

#### Refresh Token
```bash
curl -X POST http://localhost:8080/auth/refresh \
  -H "Content-Type: application/json" \
  -d '{
    "refresh_token": "your-refresh-token"
  }'
```

#### Using Bearer Token for Authenticated Requests
```bash
curl -X GET http://localhost:8080/api/v1/tenants \
  -H "Authorization: Bearer your-access-token"
```

### Creating a Tenant

```bash
curl -X POST http://localhost:8080/api/v1/tenants \
  -H "Content-Type: application/json" \
  -d '{
    "name": "example-tenant",
    "workers": 5
  }'
```

### Publishing a Message

```bash
curl -X POST http://localhost:8080/api/v1/messages \
  -H "Content-Type: application/json" \
  -d '{
    "tenant_id": "your-tenant-id",
    "payload": {"key": "value", "message": "Hello World!"}
  }'
```

### Listing Messages with Pagination

```bash
# First page
curl "http://localhost:8080/api/v1/messages?limit=10"

# Next page using cursor
curl "http://localhost:8080/api/v1/messages?limit=10&cursor=<next_cursor>"
```

### Updating Tenant Concurrency

```bash
curl -X PUT http://localhost:8080/api/v1/tenants/{tenant-id}/config/concurrency \
  -H "Content-Type: application/json" \
  -d '{"workers": 10}'
```

## Architecture

### Components

- **TenantManager**: Manages tenant lifecycle and consumer spawning/stopping
- **WorkerPool**: Handles message consumption with configurable concurrency
- **RabbitMQManager**: Manages RabbitMQ connections, channels, and queues
- **Repository Layer**: Database operations with partitioned table support
- **API Layer**: RESTful endpoints with Swagger documentation

### Database Schema

- **tenants**: Stores tenant metadata and worker configuration
- **messages**: Partitioned table storing messages by tenant_id
- Each tenant gets its own partition: `messages_{tenant_id_with_underscores}` (UUIDs converted to underscore format)

### Message Flow

1. Client creates a tenant via POST /tenants
2. System creates a tenant record and dedicated RabbitMQ queue
3. Worker pool spawns consumers for the tenant's queue
4. Client publishes messages to the tenant's queue
5. Workers consume messages and store them in the partitioned table
6. Client can retrieve messages via pagination APIs

## Development

### Running Tests

```bash
# Unit tests only
go test -short ./...

# All tests including integration tests (requires Docker)
go test ./...

# Integration tests with verbose output and timeout
go test -v ./tests/... -timeout 10m

# Skip integration tests if Docker is not available
SKIP_INTEGRATION_TESTS=true go test ./...
```

**Note**: Integration tests use `dockertest` to automatically spin up PostgreSQL and RabbitMQ containers. Docker must be running and accessible.

### Building

```bash
# Build binary
go build -o bin/server cmd/server/main.go

# Build Docker image
docker build -t multi-tenant-messaging-system .
```

### Generating Swagger Documentation

```bash
# Install swag
go install github.com/swaggo/swag/cmd/swag@latest

# Generate docs
swag init -g cmd/server/main.go
```

## Monitoring

### Health Checks

- **Application**: `GET /health`
- **Database**: Connection pool monitoring
- **RabbitMQ**: Connection and channel health

### Prometheus Metrics

The application exposes Prometheus metrics at `GET /metrics`:

```bash
# Access metrics endpoint
curl http://localhost:8080/metrics

# Filter specific metrics
curl -s http://localhost:8080/metrics | grep "messages_processed\|active_tenants"
```

**Available Metrics**:
- `messages_processed_total` - Total messages processed by tenant
- `message_processing_duration_seconds` - Message processing time histogram
- `queue_size` - Current queue size per tenant
- `active_workers` - Number of active workers per tenant
- `worker_pool_size` - Total worker pool size per tenant
- `active_tenants` - Number of active tenants
- `rabbitmq_connected` - RabbitMQ connection status (0/1)
- `db_connections_active` - Active database connections
- `api_requests_total` - Total API requests by method and endpoint
- `api_request_duration_seconds` - API request duration histogram

**Example Queries**:
```bash
# Check tenant activity
curl -s http://localhost:8080/metrics | grep "active_tenants\|worker_pool_size"

# Monitor message processing
curl -s http://localhost:8080/metrics | grep "messages_processed_total"

# Check API performance
curl -s http://localhost:8080/metrics | grep "api_request_duration_seconds"
```

## Production Considerations

### Security

- Use strong authentication for RabbitMQ and PostgreSQL
- Implement JWT authentication for API endpoints
- Use TLS/SSL for all connections
- Set up proper firewall rules

### Performance

- Monitor and adjust worker concurrency per tenant
- Set up connection pooling limits
- Use read replicas for heavy read workloads
- Implement message batching for high-throughput scenarios

### Reliability

- Set up RabbitMQ clustering for high availability
- Use PostgreSQL replication
- Implement proper backup strategies
- Set up monitoring and alerting

### Scaling

- Horizontally scale by running multiple application instances
- Use PostgreSQL partitioning for time-based data archival
- Implement message routing for multi-region deployments

## Troubleshooting

### Common Issues

**PostgreSQL Connection Issues**:
```bash
# Check if PostgreSQL is running
docker-compose ps postgres

# View PostgreSQL logs
docker-compose logs postgres

# Test connection manually
psql "postgres://user:pass@localhost:5432/messaging_system"
```

**RabbitMQ Connection Issues**:
```bash
# Check RabbitMQ status
docker-compose ps rabbitmq

# Access RabbitMQ management UI
open http://localhost:15672

# View RabbitMQ logs
docker-compose logs rabbitmq
```

**Integration Test Failures**:
```bash
# Ensure Docker is running
docker info

# Clean up containers before retesting
docker system prune -f

# Run tests with verbose output for debugging
go test -v ./tests/... -timeout 15m
```

**Application Logs**:
```bash
# View application logs in Docker
docker-compose logs app

# Follow live logs
docker-compose logs -f app

# Check specific log levels
docker-compose logs app | grep "ERROR\|WARN"
```

### Debug Mode

Set `LOGGING_LEVEL=debug` for detailed logging:
```bash
LOGGING_LEVEL=debug go run cmd/server/main.go
```

### Performance Debugging

```bash
# Check system metrics
curl http://localhost:8080/metrics | grep -E "(active_|queue_|processing_)"

# Monitor tenant activity
curl http://localhost:8080/api/v1/tenants/stats

# Check application health
curl http://localhost:8080/health
```

## License

MIT License