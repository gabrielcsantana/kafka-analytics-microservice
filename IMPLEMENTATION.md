# Implementation Details

## Overview

This document provides detailed information about the implementation decisions, architecture, and features of the Real-Time Analytics Microservice.

## Architecture Decisions

### 1. Microservices Design

**Decision**: Separate producer and consumer into independent microservices.

**Rationale**:
- Clear separation of concerns
- Independent scaling (producer handles HTTP load, consumer handles processing load)
- Failure isolation (producer failure doesn't affect consumer)
- Easier to maintain and deploy

### 2. Aggregation Strategy

**Decision**: In-memory aggregation with periodic flushing to database.

**Rationale**:
- **Performance**: Reduces database write operations from N events to periodic batches
- **Efficiency**: Aggregation happens in memory before writing
- **Scalability**: Less database load means better overall throughput
- **Trade-off**: Potential data loss if service crashes before flush (acceptable for analytics use case)

**Flush triggers**:
- Time-based: Every 30 seconds (configurable via `FLUSH_INTERVAL_SEC`)
- Size-based: When batch reaches 100 messages (configurable via `BATCH_SIZE`)
- Shutdown: Final flush during graceful shutdown

### 3. Database Schema

**Decision**: Single table with aggregated metrics.

**Schema**:
```sql
CREATE TABLE user_activity_metrics (
    id SERIAL PRIMARY KEY,
    timestamp TIMESTAMP NOT NULL,
    unique_users INTEGER NOT NULL,
    page_view_count BIGINT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

**Rationale**:
- Meets requirement of "only one table"
- Optimized for analytical queries
- Time-series data structure
- Efficient storage (aggregates instead of individual events)
- Easy to query for trends and patterns

**Alternative considered**: Storing raw events (rejected due to higher storage requirements and slower aggregation queries)

### 4. Horizontal Scaling

**Decision**: Multiple consumer workers within the same service + support for multiple service instances.

**Implementation**:
- Configurable number of goroutines (workers) per consumer instance
- Kafka consumer group handles partition distribution across instances
- Shared in-memory aggregation state within a single instance

**Rationale**:
- **Worker goroutines**: Maximize CPU utilization within a single instance
- **Multiple instances**: True horizontal scaling across machines
- **Consumer groups**: Kafka automatically distributes partitions
- **Simplicity**: No need for external coordination (Redis, etcd)

**Scaling example**:
```bash
# Scale to 3 consumer instances
docker-compose up --scale consumer=3

# Each instance runs 3 workers (9 total workers)
```

### 5. Error Handling & Retry Strategy

**Producer**:
- Kafka client handles automatic retries (3 attempts)
- 10-second timeout for writes
- Returns 500 error to client on failure

**Consumer**:
- **Database connection**: 10 retries with exponential backoff (1-10 seconds)
- **Database insert**: 3 retries with linear backoff (1-3 seconds)
- **Message parsing**: Log error and commit offset (avoid poison messages)
- **Kafka read**: Retry on transient errors, continue on timeout

**Rationale**:
- Transient failures (network glitches) are retried automatically
- Permanent failures (bad data) don't block processing
- Exponential backoff prevents overwhelming failing services
- Commit on parse failure prevents infinite reprocessing

### 6. Graceful Shutdown

**Implementation**:
- Signal handlers for SIGTERM and SIGINT
- Context cancellation propagates to all goroutines
- Final flush of aggregated data
- Clean resource closure (Kafka, database connections)
- 5-second grace period for shutdown completion

**Rationale**:
- Prevents data loss during deployment or scaling
- Ensures offset commits are finalized
- Clean shutdown prevents resource leaks
- Required for production readiness

### 7. Technology Choices

**Kafka Client**: `segmentio/kafka-go`
- Pure Go implementation (no C dependencies)
- Simple API
- Good performance
- Active maintenance

**HTTP Framework**: `gin-gonic/gin`
- Fast and lightweight
- Good middleware ecosystem
- Easy request validation
- Popular in Go community

**Database Driver**: `lib/pq`
- Standard PostgreSQL driver
- Reliable and well-tested
- Compatible with `database/sql`

**Alternative considered**: Confluent's Kafka client (rejected due to C dependencies and complexity)

## Features Implemented

### Core Requirements

- ✅ **Kafka Producer**: REST endpoint publishes to `incoming.user_activity` topic
- ✅ **Kafka Consumer**: Reads from topic, transforms data
- ✅ **Aggregation**: Counts unique users and page_view activities
- ✅ **PostgreSQL Storage**: Single table with aggregated metrics
- ✅ **Docker**: Images for both producer and consumer

### Bonus Features

- ✅ **Horizontal Scaling**: Multiple consumer workers via goroutines
- ✅ **Retry Logic**: Well-defined retry strategies for all failure scenarios
- ✅ **Graceful Shutdown**: SIGTERM/SIGINT handlers for both services
- ✅ **Health Checks**: Endpoints and Docker health checks
- ✅ **Auto Topic Creation**: Kafka automatically creates topic if missing
- ✅ **Auto Schema Creation**: Consumer creates database table if missing

### Additional Features

- ✅ **Configuration**: Environment variables for all settings
- ✅ **Logging**: Structured logging for debugging and monitoring
- ✅ **Testing Script**: Easy way to generate test data
- ✅ **Comprehensive README**: Deployment and usage instructions
- ✅ **Makefile**: Common operations simplified

## Message Flow

```
1. HTTP Client → Producer REST API (/activity)
2. Producer → Kafka Topic (incoming.user_activity)
3. Kafka → Consumer (multiple workers)
4. Consumer → In-Memory Aggregation
5. Consumer → PostgreSQL (periodic flush)
```

## Data Processing Flow

```
UserActivity Event
    ↓
Parse JSON
    ↓
Extract user_id → Add to unique users set
Extract activity_type → Increment page_view counter (if applicable)
    ↓
Check flush conditions (batch size OR time interval)
    ↓
If flush needed:
    1. Calculate metrics (unique users count, page_view count)
    2. Insert into PostgreSQL with timestamp
    3. Reset aggregation state
```

## Performance Considerations

### Producer

- Non-blocking writes to Kafka
- Request timeout prevents hanging
- Kafka batching for throughput

### Consumer

- Multiple workers for parallelism
- Batch processing reduces database load
- In-memory aggregation is fast
- Consumer group enables horizontal scaling

### Database

- Single table reduces joins
- Indexed primary key for fast inserts
- Aggregated data reduces row count
- Timestamp allows efficient time-range queries

## Testing Strategy

### Manual Testing

```bash
# Start services
docker-compose up -d

# Send test events
./test-producer.sh 100

# Wait for aggregation (30 seconds or batch size)
sleep 35

# Verify metrics
make metrics
```

### Verification Checklist

- [ ] Producer accepts valid events (200 OK)
- [ ] Producer rejects invalid events (400 Bad Request)
- [ ] Consumer processes messages (check logs)
- [ ] Metrics appear in database
- [ ] Unique users count is correct
- [ ] Page view count is correct
- [ ] Graceful shutdown works (Ctrl+C)
- [ ] Services restart without errors

## Production Readiness

### Implemented

- ✅ Graceful shutdown
- ✅ Error handling and retries
- ✅ Health checks
- ✅ Configuration via environment
- ✅ Structured logging
- ✅ Docker containerization

### Would Add for Production

- [ ] Metrics and monitoring (Prometheus)
- [ ] Distributed tracing (Jaeger/Zipkin)
- [ ] API authentication/authorization
- [ ] Rate limiting
- [ ] Circuit breakers
- [ ] Dead letter queue for failed messages
- [ ] Database migrations (Flyway/Liquibase)
- [ ] Kubernetes manifests
- [ ] CI/CD pipeline
- [ ] Integration tests
- [ ] Load tests
- [ ] Documentation (OpenAPI/Swagger)

## Known Limitations

1. **Data Loss Risk**: In-memory aggregation can lose data if service crashes before flush
   - **Mitigation**: Acceptable for analytics; could add persistence layer if needed

2. **Single Point of Aggregation**: Each consumer instance aggregates independently
   - **Impact**: Multiple instances produce multiple metric rows
   - **Mitigation**: Acceptable; can sum in queries if needed

3. **No Exactly-Once Semantics**: At-least-once processing (duplicate messages possible)
   - **Impact**: Metrics might be slightly inflated
   - **Mitigation**: Acceptable for analytics; could add deduplication if needed

4. **No Schema Evolution**: Hard-coded data structures
   - **Mitigation**: Would add schema registry for production

## Future Enhancements

1. **Real-Time Dashboard**: Web UI showing live metrics
2. **Multiple Activity Types**: Support tracking different activity types
3. **Time-Window Aggregations**: Hourly/daily rollups
4. **Alerting**: Anomaly detection and notifications
5. **Data Retention**: Automatic archival of old data
6. **API for Querying**: REST API to fetch metrics
7. **Multi-Tenancy**: Support multiple customers/projects

## Conclusion

This implementation demonstrates:
- Clean microservices architecture
- Production-grade error handling
- Scalability through Kafka and multiple workers
- Operational excellence with graceful shutdown and health checks
- Docker-first deployment approach

The solution is production-ready for a non-critical analytics use case and can be enhanced with additional features as needed.
