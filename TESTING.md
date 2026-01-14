# Testing Guide

This document describes how to run tests for the Real-Time Analytics Microservice.

## Test Types

The project includes two types of tests:

1. **Unit Tests**: Test individual functions and components in isolation
2. **Integration Tests**: Test the full system end-to-end (via `test-producer.sh`)

## Prerequisites

- Go 1.21+ installed for unit tests
- Docker and Docker Compose for integration tests

## Running Unit Tests

### Install Dependencies

First, download the test dependencies:

```bash
# For producer
cd producer
go mod tidy

# For consumer
cd consumer
go mod tidy
```

### Run All Unit Tests

```bash
# From project root
make test-unit
```

Or run tests manually:

```bash
# Producer tests
cd producer
go test -v -cover ./...

# Consumer tests
cd consumer
go test -v -cover ./...
```

### Run Specific Service Tests

```bash
# Producer only
make test-producer-unit

# Consumer only
make test-consumer-unit
```

### Run Tests with Coverage Report

```bash
# Producer
cd producer
go test -v -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Consumer
cd consumer
go test -v -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Run Benchmarks

```bash
# Producer benchmarks
cd producer
go test -bench=. -benchmem

# Consumer benchmarks
cd consumer
go test -bench=. -benchmem
```

## Running Integration Tests

Integration tests require the full system to be running:

```bash
# Start all services
docker-compose up -d

# Wait for services to be ready (check health)
docker-compose ps

# Run integration test
make test

# Or run directly
./test-producer.sh 100
```

## Test Coverage

### Producer Tests (`producer/main_test.go`)

- ✅ JSON marshaling/unmarshaling of UserActivity
- ✅ Producer initialization (NewProducer)
- ✅ Message publishing logic (PublishActivity)
- ✅ Health endpoint
- ✅ Activity endpoint validation
- ✅ Required field validation
- ✅ Invalid JSON handling
- ✅ Environment variable parsing (getEnv)
- ✅ Context timeout handling
- ✅ Producer cleanup (Close)
- ✅ Benchmarks for JSON marshaling

### Consumer Tests (`consumer/main_test.go`)

- ✅ JSON unmarshaling of UserActivity
- ✅ Aggregator initialization (NewAggregator)
- ✅ Activity processing (ProcessActivity)
- ✅ Unique user tracking
- ✅ Page view counting
- ✅ Mixed activity type handling
- ✅ Batch size threshold (ShouldFlush)
- ✅ Database flush operations (Flush)
- ✅ Empty data handling
- ✅ Database retry logic on errors
- ✅ Successful retry after failures
- ✅ Thread-safe concurrent access
- ✅ Consumer initialization (NewConsumer)
- ✅ Consumer cleanup (Close)
- ✅ Environment variable parsing (getEnv, getEnvAsInt)
- ✅ Database table creation SQL
- ✅ Database insert operations
- ✅ Benchmarks for processing and unmarshaling

## Test Dependencies

The tests use the following libraries:

- **testify/assert**: Assertion library for cleaner test assertions
- **testify/require**: Assertion library that stops test execution on failure
- **go-sqlmock**: Mock library for database testing (consumer only)

These dependencies are automatically downloaded when running `go mod tidy`.

## Continuous Integration

To run tests in CI/CD pipelines:

```yaml
# Example GitHub Actions workflow
- name: Run unit tests
  run: |
    cd producer && go test -v -cover ./...
    cd ../consumer && go test -v -cover ./...

- name: Run integration tests
  run: |
    docker-compose up -d
    sleep 10  # Wait for services
    make test
    docker-compose down
```

## Troubleshooting

### "go: command not found"

Install Go 1.21 or later from https://go.dev/dl/

### "cannot find package"

Run `go mod tidy` in the service directory to download dependencies.

### "connection refused" errors in tests

Unit tests should not connect to actual Kafka or databases. These errors suggest the test is not properly mocked. Review the test setup.

### Integration test failures

Ensure all services are healthy before running integration tests:

```bash
docker-compose ps
docker-compose logs
```

## Writing New Tests

### Unit Test Template

```go
func TestFeatureName(t *testing.T) {
    // Arrange
    // Set up test data and mocks

    // Act
    // Execute the function being tested

    // Assert
    assert.Equal(t, expected, actual)
    require.NoError(t, err)
}
```

### Table-Driven Test Template

```go
func TestFeatureName(t *testing.T) {
    tests := []struct {
        name     string
        input    interface{}
        expected interface{}
    }{
        {
            name:     "test case 1",
            input:    ...,
            expected: ...,
        },
        // More test cases...
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := FunctionUnderTest(tt.input)
            assert.Equal(t, tt.expected, result)
        })
    }
}
```

## Best Practices

1. **Test one thing at a time**: Each test should verify a single behavior
2. **Use descriptive test names**: Names should clearly indicate what is being tested
3. **Follow AAA pattern**: Arrange, Act, Assert
4. **Mock external dependencies**: Don't connect to real Kafka or databases in unit tests
5. **Test edge cases**: Include tests for error conditions and boundary values
6. **Keep tests fast**: Unit tests should run in milliseconds
7. **Test concurrency**: Include tests for thread-safe code
8. **Write benchmarks**: For performance-critical code

## Additional Resources

- [Go Testing Package](https://pkg.go.dev/testing)
- [Testify Documentation](https://github.com/stretchr/testify)
- [go-sqlmock Documentation](https://github.com/DATA-DOG/go-sqlmock)
