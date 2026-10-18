# TierGuard Database & Infrastructure Architecture

## Overview
This document specifies the database schemas, migration strategy, connection pooling, and containerized infrastructure supporting **TierGuard**'s high-throughput entitlement engine.

---

## 1. Containerized Infrastructure
TierGuard uses Docker Compose for orchestrating database and cache dependencies in development and staging environments.

### Services Defined
- **PostgreSQL 16 Alpine (`postgres`)**:
  - Image: `postgres:16-alpine`
  - Port: `5432:5432`
  - Volume: `pg_data` mounted to `/var/lib/postgresql/data`
  - Health check: `pg_isready -U tierguard`
- **Redis 7 Alpine (`redis`)**:
  - Image: `redis:7-alpine`
  - Port: `6379:6379`
  - Storage: Append-Only File (`appendonly yes`) enabled for durable persistence
  - Health check: `redis-cli ping`

```bash
# Start container dependencies
docker compose up -d

# Verify health status
docker compose ps
```

---

## 2. PostgreSQL Schema & Migrations

### Migration Files
- `internal/repository/postgres/migrations/000001_init_schema.up.sql`
- `internal/repository/postgres/migrations/000001_init_schema.down.sql`

### Relational Entity Model
```
┌─────────────────┐       ┌─────────────────┐       ┌─────────────────┐
│     tenants     │──────<│     tiers       │>──────│ tier_features   │
│  (id, name, ..) │       │(id, name, quota)│       │ (tier_id, feat) │
└─────────────────┘       └─────────────────┘       └─────────────────┘
         │
         ∨
┌─────────────────┐
│   usage_logs    │
│(tenant_id, time)│
└─────────────────┘
```

### Table Definitions
1. **`tiers`**: Defines subscription tiers (`Free`, `Pro`, `Enterprise`) with request limits and rate restrictions.
2. **`tenants`**: Represents customer organizations with an assigned active tier and unique API credentials.
3. **`tier_features`**: Maps subscription tiers to enabled feature flags (e.g., `analytics_export`, `audit_logs`).
4. **`usage_logs`**: Time-series request metering log for reconciliation and monthly quota resets.

---

## 3. Connection Pooling Architecture
The Go application connects to PostgreSQL using `pgxpool` (`github.com/jackc/pgx/v5/pgxpool`):
- **Max Connections**: 25 connections.
- **Min Idle Connections**: 5 connections.
- **Max Conn Lifetime**: 1 hour.
- **Max Idle Time**: 30 minutes.
- **Health Check Ping**: Monitored via `Ping(ctx)` on every health check cycle.

---

## 4. Seeding Pipeline
A dedicated seed tool (`cmd/seed/main.go`) initializes the environment with preconfigured tiers and realistic mock tenant profiles:

```bash
# Execute seeding pipeline
go run cmd/seed/main.go
```
- **Free Tier**: 1,000 requests/month, basic API access.
- **Pro Tier**: 10,000 requests/month, advanced analytics.
- **Enterprise Tier**: 100,000 requests/month, unlimited features.
