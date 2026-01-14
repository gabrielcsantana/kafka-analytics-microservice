# Real-Time Analytics Microservice

A real-time analytics microservice built with Golang and Kafka that processes user activity data, performs aggregations, and stores results in PostgreSQL.

## Architecture

The system consists of two main microservices:

1. **Producer Service**: REST API that receives user activity events and publishes them to Kafka
2. **Consumer Service**: Consumes events from Kafka, aggregates data (unique users and page_view counts), and stores results in PostgreSQL

### Components

- **Kafka**: Message broker for event streaming
- **Zookeeper**: Required for Kafka coordination
- **PostgreSQL**: Database for storing aggregated metrics
- **Producer**: Go microservice with REST API
- **Consumer**: Go microservice with horizontal scaling support

## Features

- ✅ REST API endpoint for publishing user activity events
- ✅ Real-time event processing with Kafka
- ✅ Data aggregation (unique users, page_view counts)
- ✅ PostgreSQL storage with automatic schema creation
- ✅ Horizontal scaling with multiple consumer workers
- ✅ Retry logic with exponential backoff
- ✅ Graceful shutdown on SIGTERM/SIGINT
- ✅ Docker containerization
- ✅ Health check endpoints

## Prerequisites

- Docker and Docker Compose
- (Optional) Go 1.21+ for local development

## Quick Start

### 1. Clone the repository

```bash
git clone <repository-url>
cd moveo-ai
```

### 2. Start all services

```bash
docker-compose up --build
```

This will start:
- Zookeeper (port 2181)
- Kafka (ports 9092, 9093)
- PostgreSQL (port 5432)
- Producer service (port 8080)
- Consumer service

### 3. Wait for services to be ready

The services have health checks configured. Wait until all services are healthy:

```bash
docker-compose ps
```

## Usage

### Send user activity events

```bash
curl -X POST http://localhost:8080/activity \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "12345",
    "activity_type": "page_view",
    "timestamp": "2024-07-01T12:34:56Z",
    "metadata": {
      "page_url": "https://example.com/home",
      "referrer": "https://google.com"
    }
  }'
```

### Check producer health

```bash
curl http://localhost:8080/health
```

### View aggregated metrics in PostgreSQL

```bash
docker exec -it postgres psql -U postgres -d analytics -c "SELECT * FROM user_activity_metrics ORDER BY created_at DESC LIMIT 10;"
```

## Configuration

### Producer Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `KAFKA_BROKERS` | `localhost:9092` | Kafka broker address |
| `KAFKA_TOPIC` | `incoming.user_activity` | Kafka topic name |
| `SERVER_PORT` | `8080` | HTTP server port |

### Consumer Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `KAFKA_BROKERS` | `localhost:9092` | Kafka broker address |
| `KAFKA_TOPIC` | `incoming.user_activity` | Kafka topic name |
| `KAFKA_GROUP_ID` | `user-activity-consumer-group` | Kafka consumer group ID |
| `NUM_WORKERS` | `3` | Number of consumer workers (for horizontal scaling) |
| `FLUSH_INTERVAL_SEC` | `30` | Interval to flush aggregated data (seconds) |
| `BATCH_SIZE` | `100` | Batch size before flushing to database |
| `DB_HOST` | `localhost` | PostgreSQL host |
| `DB_PORT` | `5432` | PostgreSQL port |
| `DB_USER` | `postgres` | PostgreSQL user |
| `DB_PASSWORD` | `postgres` | PostgreSQL password |
| `DB_NAME` | `analytics` | PostgreSQL database name |

## Database Schema

The consumer automatically creates the following table:

```sql
CREATE TABLE user_activity_metrics (
    id SERIAL PRIMARY KEY,
    timestamp TIMESTAMP NOT NULL,
    unique_users INTEGER NOT NULL,
    page_view_count BIGINT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

## Scaling

### Horizontal Scaling

The consumer supports horizontal scaling in two ways:

1. **Multiple workers within the same service** (configured via `NUM_WORKERS`)
2. **Multiple consumer instances** (scale the Docker service)

To scale the consumer service:

```bash
docker-compose up --scale consumer=3
```

## Testing

### Unit Tests

The project includes comprehensive unit tests for both services. See [TESTING.md](TESTING.md) for detailed testing documentation.

```bash
# Run all unit tests
make test-unit

# Run producer tests only
make test-producer-unit

# Run consumer tests only
make test-consumer-unit

# Run with coverage
cd producer && go test -v -cover ./...
cd consumer && go test -v -cover ./...
```

**Test Coverage:**
- ✅ JSON marshaling/unmarshaling
- ✅ Activity processing and aggregation
- ✅ Database operations with retry logic
- ✅ HTTP endpoint validation
- ✅ Concurrent access safety
- ✅ Environment variable parsing
- ✅ Error handling

### Integration Tests

Use the provided script to generate sample user activity:

```bash
# Send 100 random user activities
make test

# Or use the script directly
./test-producer.sh 100
```

## Monitoring

### View logs

```bash
# Producer logs
docker-compose logs -f producer

# Consumer logs
docker-compose logs -f consumer

# Kafka logs
docker-compose logs -f kafka
```

### Check Kafka topics

```bash
docker exec -it kafka kafka-topics --bootstrap-server localhost:9092 --list
```

### Check consumer group status

```bash
docker exec -it kafka kafka-consumer-groups --bootstrap-server localhost:9092 --group user-activity-consumer-group --describe
```

## Graceful Shutdown

Both services handle SIGTERM and SIGINT signals gracefully:

- **Producer**: Stops accepting new requests, completes in-flight requests, closes Kafka connection
- **Consumer**: Stops consuming messages, flushes pending aggregations to database, commits offsets, closes connections

To test graceful shutdown:

```bash
docker-compose stop producer consumer
```

## Development

### Local development without Docker

1. Start Kafka and PostgreSQL:

```bash
docker-compose up zookeeper kafka postgres
```

2. Run producer:

```bash
cd producer
go mod download
go run main.go
```

3. Run consumer:

```bash
cd consumer
go mod download
go run main.go
```

## Error Handling

### Retry Strategy

- **Producer**: Retries message publishing up to 3 times with Kafka client built-in retry
- **Consumer**:
  - Database connection: 10 retries with exponential backoff
  - Database insert: 3 retries with 1-3 second delays
  - Failed message parsing: Commits offset to avoid reprocessing

### Failure Scenarios

1. **Kafka unavailable**: Producer returns 500 error, consumer retries connection
2. **Database unavailable**: Consumer retries with backoff, logs errors
3. **Invalid message format**: Consumer logs error and commits offset
4. **Duplicate messages**: Idempotency handled by aggregation logic

## Architecture Decisions

### Why batch aggregation?

Instead of writing individual events to the database, we aggregate in-memory and flush periodically or when a batch size is reached. This:
- Reduces database write load
- Improves performance
- Provides natural aggregation boundaries

### Why single table?

The challenge requires "only one table", so we store time-series aggregated metrics rather than individual events. This design is optimized for:
- Analytical queries (counting users and page views)
- Reduced storage footprint
- Fast aggregation

### Why multiple workers?

Multiple consumer workers (goroutines) within the same process allow:
- Horizontal scaling without complex coordination
- Better CPU utilization
- Shared in-memory aggregation state

## Troubleshooting

### Services not starting

```bash
# Check service status
docker-compose ps

# Check logs
docker-compose logs

# Restart services
docker-compose restart
```

### Connection errors

Ensure all services are healthy before sending requests:

```bash
docker-compose ps
```

### Database connection issues

```bash
# Check PostgreSQL logs
docker-compose logs postgres

# Verify database exists
docker exec -it postgres psql -U postgres -l
```

## Clean Up

```bash
# Stop and remove containers
docker-compose down

# Remove volumes (deletes data)
docker-compose down -v

# Remove images
docker-compose down --rmi all
```

## License

MIT

## Contact

For questions or issues, please contact the development team.
