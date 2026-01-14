#!/bin/bash

# Script to generate sample user activity data
# Usage: ./test-producer.sh [num_events]

NUM_EVENTS=${1:-50}
PRODUCER_URL=${PRODUCER_URL:-http://localhost:8080}

echo "Sending $NUM_EVENTS user activity events to $PRODUCER_URL/activity"

for i in $(seq 1 $NUM_EVENTS); do
  USER_ID=$((1 + RANDOM % 100))

  # Randomly choose activity type (80% page_view, 20% other)
  if [ $((RANDOM % 10)) -lt 8 ]; then
    ACTIVITY_TYPE="page_view"
  else
    ACTIVITY_TYPE="click"
  fi

  TIMESTAMP=$(date -u +%Y-%m-%dT%H:%M:%SZ)

  curl -X POST "$PRODUCER_URL/activity" \
    -H "Content-Type: application/json" \
    -s -w "\n" \
    -d "{
      \"user_id\": \"user_${USER_ID}\",
      \"activity_type\": \"${ACTIVITY_TYPE}\",
      \"timestamp\": \"${TIMESTAMP}\",
      \"metadata\": {
        \"page_url\": \"https://example.com/page${i}\",
        \"referrer\": \"https://google.com\"
      }
    }" | grep -q "successfully" && echo "✓ Event $i sent" || echo "✗ Event $i failed"

  # Small delay to avoid overwhelming the system
  sleep 0.05
done

echo ""
echo "Completed sending $NUM_EVENTS events"
echo "Check metrics with: docker exec -it postgres psql -U postgres -d analytics -c 'SELECT * FROM user_activity_metrics ORDER BY created_at DESC;'"
