# TierGuard - SaaS Entitlements & Metering Engine

[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go)](https://golang.org)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-16-336791?style=flat&logo=postgresql)](https://www.postgresql.org/)
[![Redis](https://img.shields.io/badge/Redis-7-DC382D?style=flat&logo=redis)](https://redis.io/)
[![HTMX](https://img.shields.io/badge/HTMX-1.9.12-3D72D7?style=flat)](https://htmx.org/)
[![Tailwind CSS](https://img.shields.io/badge/Tailwind-CSS-38B2AC?style=flat&logo=tailwind-css)](https://tailwindcss.com/)

TierGuard is an ultra-low latency (<5ms) SaaS entitlements and usage metering engine designed to decouple subscription and quota enforcement logic from core application codebases.

---

## ⚡ Core Architecture

```
                  ┌────────────────────────┐
                  │   Incoming Request     │
                  │   POST /api/verify     │
                  └───────────┬────────────┘
                              │
                    [Sub-5ms Memory Path]
                              ▼
                  ┌────────────────────────┐
                  │      Redis Cache       │
                  │  Tenant Quotas & Tiers │
                  └───────────┬────────────┘
                              │
               ┌──────────────┴──────────────┐
       [Allowed]                             [Forbidden]
               │                                     │
               ▼                                     ▼
        HTTP 200 OK                          HTTP 403 Forbidden
     {"allowed": true}                 {"allowed": false, "reason": "..."}
               │
      (Async Goroutine)
               ▼
     ┌───────────────────┐
     │   Redis Streams   │
     │   Usage Metering  │
     └───────────────────┘
```

- **Sub-5ms Authorization**: All entitlement checks (`POST /api/verify`) query in-memory Redis keys directly, bypassing the relational database in the critical hot path.
- **Asynchronous Usage Metering**: Valid verification requests dispatch non-blocking Go goroutines that append usage events to Redis Streams.
- **Relational Source of Truth**: PostgreSQL stores canonical tenant organizations, tier definitions, feature requirements, and monthly usage logs.
- **Real-Time Synchronized Admin Console**: Built with Go standard `html/template`, HTMX, and Tailwind CSS. Product managers can instantly update tenant subscription tiers with zero page reload, synchronizing PostgreSQL and Redis concurrently.

---

## 🚀 Key Features

- **Blazing Fast Verification**: Resolves tier hierarchy and quota constraints in under 5ms.
- **Instant Tier Updates**: Dynamic HTMX updates propagate across PostgreSQL and Redis simultaneously.
- **Modern Enterprise Admin Dashboard**:
  - High-density telemetry data table with visual quota progress tracks.
  - Full-viewport dark and light theme switching.
  - Multi-language localization (English, தமிழ், हिन्दी).
  - Dynamic live connection status lights for PostgreSQL and Redis (green when connected, red when disconnected).
- **Zero Frontend Bundlers**: Built purely with Go HTML templates, HTMX, and Tailwind CSS. No React or npm build steps required.

---

## 🛠️ Tech Stack

- **Backend**: Go (`net/http`, `pgx/v5`, `go-redis/v9`, `google/uuid`)
- **Database**: PostgreSQL 16
- **Cache & Streaming**: Redis 7
- **Frontend**: Go `html/template`, HTMX 1.9, Tailwind CSS

---

## 📦 Getting Started

### 1. Prerequisites
- [Docker](https://www.docker.com/) & Docker Compose
- [Go 1.22+](https://golang.org/dl/)

### 2. Start PostgreSQL & Redis
```bash
docker compose up -d
```

### 3. Run Database Migrations & Seed Mock Data
```bash
# Apply schema migrations
psql -h localhost -U postgres -d tierguard -f migrations/001_init_schema.sql

# Seed default tiers (Free: 1,000, Pro: 50,000, Enterprise: 1,000,000) and mock tenants
go run cmd/seed/main.go
```

### 4. Start the TierGuard Server
```bash
go run cmd/server/main.go
```
The server will warm up the Redis cache from PostgreSQL and listen on `http://localhost:8080`.

- **Admin Dashboard**: `http://localhost:8080/admin`
- **Health Check**: `http://localhost:8080/health`
- **Verification API**: `POST http://localhost:8080/api/verify`

---

## 📡 API Reference

### Entitlement Verification
Checks whether a tenant has access to a feature and has remaining quota for the billing period.

```http
POST /api/verify
Content-Type: application/json

{
  "tenant_id": "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
  "requested_feature": "sso_auth"
}
```

#### Response (Allowed)
```json
{
  "allowed": true
}
```

#### Response (Quota Exceeded)
```json
{
  "allowed": false,
  "reason": "quota_exceeded"
}
```

#### Response (Tier Too Low)
```json
{
  "allowed": false,
  "reason": "feature_not_entitled"
}
```

---

## 🧪 Testing Usage Simulation

A built-in script simulates concurrent requests and verifies quota enforcement:

```bash
chmod +x simulate_usage.sh
./simulate_usage.sh
```

---

## 📄 License

MIT License. Designed and built with enterprise standards.
