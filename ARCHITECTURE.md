# Architecture Documentation

This document describes the architecture and code organization of the Real-Time Analytics Microservice.

## Project Structure

```
moveo-ai/
├── producer/               # Producer service
│   ├── main.go            # Application entry point and HTTP server setup
│   ├── models.go          # Data models (UserActivity, Metadata)
│   ├── producer.go        # Kafka producer logic
│   ├── utils.go           # Utility functions (env helpers)
│   ├── main_test.go       # Unit tests
│   ├── go.mod             # Go module dependencies
│   └── Dockerfile         # Docker build configuration
│
├── consumer/              # Consumer service
│   ├── main.go           # Application entry point
│   ├── models.go         # Data models (UserActivity, Metadata)
│   ├── consumer.go       # Kafka consumer logic
│   ├── aggregator.go     # Metrics aggregation logic
│   ├── db.go             # Database initialization and schema
│   ├── utils.go          # Utility functions (env helpers)
│   ├── main_test.go      # Unit tests
│   ├── go.mod            # Go module dependencies
│   └── Dockerfile        # Docker build configuration
│
├── docker-compose.yml    # Multi-container orchestration
├── Makefile              # Build and test automation
├── README.md             # User documentation
├── TESTING.md            # Testing guide
├── ARCHITECTURE.md       # This file
├── IMPLEMENTATION.md     # Implementation details
└── QUICKSTART.md         # Quick start guide
```

## Code Organization Principles

### Separation of Concerns

Each file has a single, well-defined responsibility:

1. **main.go**: Application bootstrap and coordination
   - Configuration loading
   - Component initialization
   - Graceful shutdown handling

2. **models.go**: Data structures
   - Request/response types
   - Domain models
   - JSON serialization tags

3. **producer.go / consumer.go**: Core business logic
   - Message publishing/consuming
   - Worker management
   - Error handling

4. **aggregator.go**: Data processing
   - In-memory aggregation
   - Batch processing
   - Thread-safe operations

5. **db.go**: Database operations
   - Connection management
   - Schema initialization
   - Retry logic

6. **utils.go**: Helper functions
   - Environment variable parsing
   - Common utilities

7. **main_test.go**: Unit tests
   - Function-level testing
   - Mock implementations
   - Benchmarks

### File Naming Conventions

- `main.go`: Entry point (required by Go)
- `models.go`: Data structures and types
- `{component}.go`: Core functionality for specific components
- `{component}_test.go`: Tests for the corresponding component
- `utils.go`: Shared utility functions
- `db.go`: Database-related code

## Producer Service Architecture

### Components

```
┌─────────────┐
│   main.go   │  Entry point, HTTP server setup
└──────┬──────┘
       │
       ├─────► ┌──────────────┐
       │       │  models.go   │  UserActivity, Metadata
       │       └──────────────┘
       │
       ├─────► ┌──────────────┐
       │       │ producer.go  │  Kafka publishing logic
       │       └──────────────┘
       │
       └─────► ┌──────────────┐
               │  utils.go    │  Environment helpers
               └──────────────┘
```

### Request Flow

1. HTTP POST `/activity` receives user activity
2. Gin framework validates JSON (via struct tags)
3. Producer marshals activity to JSON
4. Kafka writer publishes to topic
5. Response returned to client

### Key Features

- **Validation**: Automatic via Gin binding tags
- **Timeout**: 10-second context timeout for Kafka writes
- **Retry**: 3 attempts with Kafka client built-in retry
- **Health Check**: `/health` endpoint for monitoring

## Consumer Service Architecture

### Components

```
┌─────────────┐
│   main.go   │  Entry point, coordination
└──────┬──────┘
       │
       ├─────► ┌──────────────┐
       │       │  models.go   │  UserActivity, Metadata
       │       └──────────────┘
       │
       ├─────► ┌──────────────┐
       │       │ consumer.go  │  Kafka consumption, workers
       │       └──────────────┘
       │
       ├─────► ┌───────────────┐
       │       │ aggregator.go │  Metrics aggregation
       │       └───────────────┘
       │
       ├─────► ┌──────────────┐
       │       │    db.go     │  Database operations
       │       └──────────────┘
       │
       └─────► ┌──────────────┐
               │  utils.go    │  Environment helpers
               └──────────────┘
```

### Data Flow

1. Multiple workers fetch messages from Kafka
2. Each worker unmarshals JSON to UserActivity
3. Aggregator processes activity (thread-safe)
4. Periodic flush or batch size triggers DB write
5. Metrics stored in PostgreSQL

### Aggregation Strategy

- **In-Memory Aggregation**: Reduces DB write load
- **Unique Users**: Tracked using map (deduplicated)
- **Page Views**: Counter for page_view activity types
- **Flush Triggers**:
  - Batch size reached (default: 100 messages)
  - Time interval elapsed (default: 30 seconds)

### Concurrency Model

- **Worker Pool**: N goroutines consume messages
- **Shared Aggregator**: Protected by mutex
- **Periodic Flusher**: Separate goroutine for time-based flush
- **Graceful Shutdown**: Context cancellation signals all workers

## Design Patterns

### Producer Patterns

1. **Dependency Injection**: Producer receives Kafka writer configuration
2. **Context Propagation**: Timeout handling via context
3. **Error Wrapping**: Errors preserve context through the stack

### Consumer Patterns

1. **Worker Pool**: Horizontal scaling within a single process
2. **Producer-Consumer**: Kafka messages → Aggregator → Database
3. **Mutex Protection**: Thread-safe shared state access
4. **Retry with Backoff**: Exponential backoff for DB failures

### Testing Patterns

1. **Table-Driven Tests**: Multiple test cases in loops
2. **Mocking**: go-sqlmock for database operations
3. **Isolation**: Tests don't require external services
4. **Benchmarking**: Performance testing for critical paths

## Scalability Considerations

### Horizontal Scaling

**Producer**:
- Stateless design allows multiple instances
- Load balancer distributes traffic
- Kafka handles partitioning

**Consumer**:
- Multiple workers per instance (goroutines)
- Multiple instances via Docker Compose
- Kafka consumer groups coordinate partitions

### Vertical Scaling

- Increase `NUM_WORKERS` for more parallel processing
- Adjust `BATCH_SIZE` for throughput tuning
- Configure Kafka reader `MaxBytes` for larger batches

### Performance Tuning

1. **Batch Processing**: Reduces DB operations
2. **Connection Pooling**: SQL database connections
3. **Async Operations**: Non-blocking goroutines
4. **Kafka Partitioning**: Parallel message processing

## Error Handling Strategy

### Producer

- **Validation Errors**: Return 400 Bad Request
- **Kafka Errors**: Return 500, log for investigation
- **Timeout**: Context deadline prevents hanging requests

### Consumer

- **Parse Errors**: Log and commit offset (skip bad messages)
- **DB Errors**: Retry 3 times with exponential backoff
- **Connection Errors**: Retry 10 times for DB connection
- **Context Cancellation**: Clean shutdown with final flush

## Configuration Management

All configuration via environment variables:

```go
// Producer
KAFKA_BROKERS    string  "localhost:9092"
KAFKA_TOPIC      string  "incoming.user_activity"
SERVER_PORT      string  "8080"

// Consumer
KAFKA_BROKERS       string  "localhost:9092"
KAFKA_TOPIC         string  "incoming.user_activity"
KAFKA_GROUP_ID      string  "user-activity-consumer-group"
NUM_WORKERS         int     3
FLUSH_INTERVAL_SEC  int     30
BATCH_SIZE          int     100
DB_HOST            string  "localhost"
DB_PORT            string  "5432"
DB_USER            string  "postgres"
DB_PASSWORD        string  "postgres"
DB_NAME            string  "analytics"
```

## Best Practices Applied

1. ✅ **Single Responsibility**: Each file has one clear purpose
2. ✅ **DRY**: Common utilities extracted (utils.go)
3. ✅ **Testability**: Dependency injection, mockable interfaces
4. ✅ **Error Handling**: Errors are logged, wrapped, and handled appropriately
5. ✅ **Graceful Shutdown**: SIGTERM/SIGINT handled with cleanup
6. ✅ **Concurrency Safety**: Mutexes protect shared state
7. ✅ **Configuration**: Externalized via environment variables
8. ✅ **Documentation**: Comprehensive comments and docs
9. ✅ **Testing**: Unit tests with good coverage
10. ✅ **Observability**: Structured logging throughout

## Future Enhancements

Potential improvements while maintaining current architecture:

1. **Metrics**: Add Prometheus metrics for monitoring
2. **Tracing**: Implement distributed tracing (OpenTelemetry)
3. **Schema Validation**: JSON Schema validation for events
4. **Dead Letter Queue**: Handle persistently failing messages
5. **Circuit Breaker**: Protect against cascading failures
6. **Rate Limiting**: Prevent producer overload
7. **Compression**: Enable Kafka message compression
8. **Partitioning Strategy**: Custom partition keys for better distribution
9. **Health Checks**: More comprehensive health endpoints
10. **Configuration Management**: Centralized config service (e.g., Consul)

## References

- [Go Project Layout](https://github.com/golang-standards/project-layout)
- [Effective Go](https://go.dev/doc/effective_go)
- [Kafka Best Practices](https://kafka.apache.org/documentation/#design)
- [PostgreSQL Performance](https://www.postgresql.org/docs/current/performance-tips.html)
