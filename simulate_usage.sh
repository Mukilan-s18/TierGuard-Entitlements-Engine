#!/usr/bin/env bash
set -e

ENDPOINT="http://localhost:8080/api/verify"
TENANT_ID="aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
FEATURE="export_reports"

echo "=========================================================="
echo "Simulating 100 requests for Tenant A ($TENANT_ID)..."
echo "Feature: $FEATURE (Requires Pro tier)"
echo "=========================================================="

success_count=0
fail_count=0

for i in $(seq 1 100); do
  STATUS=$(curl -s -o /tmp/verify_res.json -w "%{http_code}" -X POST "$ENDPOINT" \
    -H "Content-Type: application/json" \
    -d "{\"tenant_id\": \"$TENANT_ID\", \"requested_feature\": \"$FEATURE\"}")
  
  if [ "$STATUS" -eq 200 ]; then
    success_count=$((success_count + 1))
    if [ $((i % 20)) -eq 0 ] || [ "$i" -eq 1 ] || [ "$i" -eq 100 ]; then
      echo "  Request #$i: HTTP $STATUS (Allowed) [Body: $(cat /tmp/verify_res.json)]"
    fi
  else
    fail_count=$((fail_count + 1))
    echo "  Request #$i FAILED with HTTP $STATUS: $(cat /tmp/verify_res.json)"
  fi
done

echo ""
echo "Summary of 100 test requests:"
echo "  Successful (HTTP 200): $success_count / 100"
echo "  Failed:                $fail_count / 100"
echo ""

# Give background async streaming goroutines a brief moment to finish updating Redis
sleep 0.1

echo "=========================================================="
echo "Sending 101st request to verify quota limit enforcement..."
echo "=========================================================="

STATUS_101=$(curl -s -o /tmp/verify_101.json -w "%{http_code}" -X POST "$ENDPOINT" \
  -H "Content-Type: application/json" \
  -d "{\"tenant_id\": \"$TENANT_ID\", \"requested_feature\": \"$FEATURE\"}")

RESPONSE_BODY=$(cat /tmp/verify_101.json)
echo "  Request #101: HTTP $STATUS_101"
echo "  Response Payload: $RESPONSE_BODY"

if [ "$STATUS_101" -eq 403 ]; then
  echo ""
  echo "SUCCESS: 101st request was correctly rejected with HTTP 403 Forbidden."
  exit 0
else
  echo ""
  echo "ERROR: Expected HTTP 403, but received HTTP $STATUS_101"
  exit 1
fi
