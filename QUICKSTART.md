# Quick Start Guide

Get the system running in 5 minutes.

## Prerequisites

- Docker and Docker Compose installed
- Port 8080, 5432, 9092 available

## Step 1: Start Everything

```bash
docker-compose up --build -d
```

Wait ~30 seconds for all services to start.

## Step 2: Verify Services are Running

```bash
docker-compose ps
```

All services should show "Up" status.

## Step 3: Send Test Data

```bash
./test-producer.sh 50
```

This sends 50 user activity events.

## Step 4: View Results

Wait 30 seconds for aggregation, then check the database:

```bash
docker exec -it postgres psql -U postgres -d analytics -c "SELECT * FROM user_activity_metrics ORDER BY created_at DESC;"
```

You should see rows with:
- `unique_users`: Count of unique users
- `page_view_count`: Count of page_view activities

## Step 5: Try the REST API

Send a custom event:

```bash
curl -X POST http://localhost:8080/activity \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "user_123",
    "activity_type": "page_view",
    "timestamp": "2024-07-01T12:34:56Z",
    "metadata": {
      "page_url": "https://example.com/home",
      "referrer": "https://google.com"
    }
  }'
```

## Step 6: Watch Live Logs

```bash
docker-compose logs -f consumer
```

You'll see messages being processed in real-time.

## Clean Up

```bash
docker-compose down -v
```

## Common Issues

**Port already in use**: Stop other services using ports 8080, 5432, or 9092

**Services not starting**: Wait longer or check logs with `docker-compose logs`

**No data in database**: Wait 30 seconds for flush interval or send 100+ messages to trigger batch flush

## Next Steps

- Read [README.md](README.md) for detailed documentation
- Read [IMPLEMENTATION.md](IMPLEMENTATION.md) for architecture details
- Scale consumers: `docker-compose up --scale consumer=3`
- Monitor: `docker-compose logs -f`
