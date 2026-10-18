#!/usr/bin/env bash
# Database and Redis Health Check Utility
# Contributed by: MUKESHKUMAR-21 (Mukeshkumar)

set -e

echo "=== TierGuard Infrastructure Diagnostics ==="

# Check PostgreSQL
if command -v pg_isready &> /dev/null; then
    echo -n "Checking PostgreSQL connection... "
    if pg_isready -h localhost -p 5432 -U tierguard > /dev/null 2>&1; then
        echo "✅ Connected (Port 5432)"
    else
        echo "⚠️ PostgreSQL is unreachable at localhost:5432"
    fi
else
    echo "ℹ️ pg_isready command not found; skipping direct CLI check."
fi

# Check Redis
if command -v redis-cli &> /dev/null; then
    echo -n "Checking Redis instance... "
    if [ "$(redis-cli -h localhost -p 6379 ping 2>/dev/null)" = "PONG" ]; then
        echo "✅ Connected (PONG on Port 6379)"
    else
        echo "⚠️ Redis is unreachable at localhost:6379"
    fi
else
    echo "ℹ️ redis-cli command not found; skipping direct CLI check."
fi

echo "Infrastructure diagnostics complete."
