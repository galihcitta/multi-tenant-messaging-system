# Multi-Tenant Messaging System Technical Task

## Overview
Build a Go application using RabbitMQ and PostgreSQL that handles multi-tenant messaging with dynamic consumer management, partitioned data storage, and configurable concurrency. The system must include APIs, automated tests, and graceful shutdown.

## Requirements

### 1. Auto-Spawn Tenant Consumer
- **Implementation**:
    - Create `TenantManager` service to track active tenants and their RabbitMQ consumers.
    - When a tenant is created via `POST /tenants`:
        - Create dedicated RabbitMQ queue `tenant_{id}_queue`.
        - Spawn consumer goroutine listening to the queue.
        - Store consumer control channel in `TenantManager`.
    - Use RabbitMQ's `channel.Consume()` with unique consumer tags.

### 2. Auto-Stop Tenant Consumer
- **Implementation**:
    - On `DELETE /tenants/{id}`:
        - Send shutdown signal via consumer's control channel.
        - Close RabbitMQ channel and delete queue.
        - Remove tenant from `TenantManager`.

### 3. Partitioned Message Table
- **Database Schema**:
  ```sql
  CREATE TABLE messages (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    payload JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW()
  ) PARTITION BY LIST (tenant_id);
  ```

### 4. Configurable Transmitter Concurrency
- **API Endpoint**:
    - PUT /tenants/{id}/config/concurrency
    ```json
      { "workers": 5 }
    ```
- **Implementation**:
    - Use worker pool pattern with buffered channel.
    - Atomic variable to track worker count.

### 5. Graceful Shutdown
- **Implementation**:
    - Application must process ongoing transaction before stop.

### 6. Cursor Pagination API
- **API Endpoint**:
    - GET /messages?cursor=123
    ```json
      { 
        "data": [],
        "next_cursor": "456"
      }
    ```
  
### 7. Swagger Documentation
- Generate OpenAPI spec with swag init.

### 8. Integration Tests
- **Test Setup**:
    - Example:
    ```go
        pool, err := dockertest.NewPool("")
        resource, _ := pool.Run("postgres", "13", [...])
        
        // Run migrations
        // Execute tests
        defer pool.Purge(resource)
    ``` 
- **Test Cases**:
    - Tenant creation/destruction lifecycle
    - Message publishing/consumption
    - Concurrency config updates

### 9. Configuration Management
- **Structure**:
  ```yaml
    rabbitmq:
      url: amqp://user:pass@localhost:5672/
    database:
      url: postgres://user:pass@localhost:5432/app
    workers: 3  # Default worker
  ```

### Additional Considerations
- Retry Logic: Dead-letter queues for failed messages
- Monitoring: Prometheus metrics for queue depth and worker activity
- Security: JWT authentication for tenant-specific operations
  

  