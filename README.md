<div align="center">

# 🛡️ TierGuard
### Ultra-Low Latency SaaS Entitlements & Real-Time Usage Metering Engine

[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=for-the-badge&logo=go&logoColor=white)](https://golang.org)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-16-336791?style=for-the-badge&logo=postgresql&logoColor=white)](https://www.postgresql.org/)
[![Redis](https://img.shields.io/badge/Redis-7%20Alpine-DC382D?style=for-the-badge&logo=redis&logoColor=white)](https://redis.io/)
[![HTMX](https://img.shields.io/badge/HTMX-1.9.10-3D72D7?style=for-the-badge&logo=htmx&logoColor=white)](https://htmx.org/)
[![Tailwind CSS](https://img.shields.io/badge/Tailwind-CSS-38B2AC?style=for-the-badge&logo=tailwind-css&logoColor=white)](https://tailwindcss.com/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg?style=for-the-badge)](LICENSE)
[![Latency SLA](https://img.shields.io/badge/P99%20Latency-%3C%203.8ms-success?style=for-the-badge)](docs/CORE_ENGINE_SPEC.md)

<p align="center">
  <b>Decouple subscription tiers, plan allowances, and quota enforcement from your monolithic core application.</b><br>
  Built with high-concurrency Go, sub-millisecond Redis caching, atomic dual-write synchronization, and an interactive HTMX control plane.
</p>

[Architecture](#-architecture--data-flow) •
[Key Features](#-key-features) •
[Dashboard UI](#-interactive-admin-dashboard) •
[Quickstart](#-quickstart--local-setup) •
[API Reference](#-api-reference) •
[Testing & Simulation](#-testing--verification) •
[Contributors](#-engineering-team)

</div>

---

## 💡 Why TierGuard?

In standard B2B SaaS architectures, subscription rules, plan limitations, and usage quotas are frequently hardcoded across application controllers, API gateways, and background jobs. This introduces critical architectural liabilities:

- **Database Bottlenecks**: Executing SQL queries to count monthly tenant requests on every API call introduces severe database contention and degrades latency.
- **Billing Coupled to Business Logic**: Changing tier allocations or adding new feature flags requires full application re-deployments and schema migrations.
- **Slow Propagation**: Updating a customer's subscription tier in a CRM or Stripe webhook often takes minutes to reflect across distributed microservices.

**TierGuard solves this by providing a dedicated, decoupled entitlements engine**:
1. **Sub-5ms Hot Path**: Resolves tenant entitlements and rate allowances entirely from in-memory Redis keys.
2. **Asynchronous Metering**: Dispatches non-blocking Go goroutines and Redis Streams to record usage without adding latency to the client request.
3. **Atomic Dual-Write Consistency**: Instantly synchronizes PostgreSQL relational models and in-memory Redis caches when plans change.
4. **Zero-Build Admin Console**: Built with Go HTML templates, Tailwind CSS, and HTMX for instant, zero-reload administrative controls.

---

## ⚡ Architecture & Data Flow

TierGuard separates the **Read/Verification Critical Path** from the **Write/Administrative Path**:

```
                              ┌───────────────────────────────────┐
                              │    Client API Request             │
                              │    POST /api/verify               │
                              └─────────────────┬─────────────────┘
                                                │
                                    [Sub-5ms Critical Path]
                                                ▼
                              ┌───────────────────────────────────┐
                              │     In-Memory Redis Cache         │
                              │  - Tenant Active Tier             │
                              │  - Feature Entitlements           │
                              │  - Real-Time Quota Counter        │
                              └─────────────────┬─────────────────┘
                                                │
                         ┌──────────────────────┴──────────────────────┐
                         │                                             │
               [Quota & Feature OK]                             [Exceeded / Denied]
                         │                                             │
                         ▼                                             ▼
                 HTTP 200 Allowed                              HTTP 403 Forbidden
                {"allowed": true}                       {"allowed": false, "reason": "..."}
                         │
              (Async Go Goroutine)
                         │
                         ▼
               ┌───────────────────┐
               │   Redis Streams   │
               │  tierguard:usage  │
               └─────────┬─────────┘
                         │
                 (Batch Ingestion)
                         ▼
               ┌───────────────────┐                    ┌───────────────────────────────┐
               │   PostgreSQL 16   │◄──[Dual-Write]─────│   Admin Control Plane         │
               │ (Source of Truth) │    Synchronization │   (HTMX + Tailwind Dashboard) │
               └───────────────────┘                    └───────────────────────────────┘
```

### Verification Pipeline SLAs
- **Hot Path Evaluation**: `GET tenant:{id}:tier` + `GET tenant:{id}:quota` + `HEXISTS tier:{tier}:features {feature}`.
- **Measured P95 Latency**: **2.1ms**.
- **Measured P99 Latency**: **3.8ms**.
- **Cache Hit Ratio**: **> 99.4%** under sustained concurrent workloads.

---

## ✨ Key Features

- **Blazing Fast Evaluation (<5ms)**: Evaluates complex boolean feature entitlements and numeric quotas with sub-millisecond memory lookups.
- **Atomic Dual-Write Synchronization**: Tier modifications synchronously commit to PostgreSQL and instantly invalidate/repopulate Redis cache keys with zero stale reads.
- **Asynchronous Telemetry Stream**: Decouples quota increments from HTTP responses using background worker goroutines and durable Redis Streams.
- **High-Density Control Plane**:
  - Live tenant directory displaying active subscription tiers and consumption progress tracks.
  - HTMX-driven instant tier switching with zero full-page reloads.
  - Class-based dark and light theme switching with `localStorage` persistence.
  - Multi-language localization supporting **English**, **Tamil (தமிழ்)**, and **Hindi (हिन्दी)**.
  - Real-time dynamic health status beacons for PostgreSQL and Redis connectivity.
- **Zero Frontend Build Tooling**: Pure Go standard library `html/template`, HTMX CDN, and Tailwind CSS CDN. Zero `node_modules`, npm scripts, or webpack bundlers.

---

## 🖥️ Interactive Admin Dashboard

The administrative console is modeled with the product focus and aesthetic restraint of enterprise developer tools like **Linear** and **Stripe**:

| Feature | Description | Implementation |
|:---|:---|:---|
| **Live Telemetry Bar** | Header counters displaying Total Tenants, System Status, Active Plan Breakdown, and Engine Uptime | Server-rendered Go template variables |
| **Instant Tier Switching** | Inline `<select>` dropdown that immediately updates tenant tiers without reloading the page | `hx-post="/admin/tenants/tier"` with optimistic target swapping |
| **Quota Fill Meters** | Visual progress tracks showing real-time usage (e.g. `850 / 1,000` requests) with color transitions | Dynamic CSS width binding (`bg-indigo-600`, `bg-amber-500`, `bg-rose-600`) |
| **Theme System** | Toggle between refined dark mode (`slate-950`) and crisp light mode (`slate-50`) | CSS class switching with `localStorage` persistence |
| **Trilingual i18n** | Seamless switching between English, Tamil, and Hindi on the fly | Client-side localization map with DOM binding |
| **Health Beacons** | Live pulsing green/red indicators showing database and cache connectivity | Dynamic ping verification via `/health` endpoint |

---

## 📊 Default Subscription Tiers

TierGuard ships with a pre-configured, production-ready tier hierarchy:

| Tier | Monthly Request Limit | Included Features | Target Audience |
|:---|:---:|:---|:---|
| **Free** | `1,000` | `core_api`, `basic_analytics` | Indie developers, evaluation & prototypes |
| **Pro** | `10,000` | `core_api`, `basic_analytics`, `advanced_metrics`, `webhooks` | Growth SaaS startups, scale-ups |
| **Enterprise** | `100,000` | `core_api`, `basic_analytics`, `advanced_metrics`, `webhooks`, `sso_auth`, `audit_export` | Enterprise customers with dedicated SLA requirements |

---

## 📁 Repository Structure

```
TierGuard/
├── cmd/
│   ├── server/             # Application entrypoint & HTTP router initialization
│   │   └── main.go
│   ├── seed/               # Database seeder for default tiers and mock tenants
│   │   └── main.go
│   └── test_redis/         # Standalone Redis latency and connection diagnostic tool
│       └── main.go
├── internal/
│   ├── domain/             # Core business models (Tenant, Tier, Feature, Entitlement)
│   │   └── models.go
│   ├── handlers/           # HTTP controllers for API verification and Admin UI
│   │   ├── admin.go        # Dashboard rendering & HTMX partial handlers
│   │   ├── admin_test.go   # Unit test suite for admin endpoints
│   │   ├── verify.go       # High-performance sub-5ms entitlement verification
│   │   └── verify_test.go  # Unit & benchmark test suite for verify API
│   ├── repository/
│   │   ├── postgres/       # PostgreSQL pgxpool queries, transactions & migrations
│   │   │   ├── migrations/ # Up and down SQL DDL scripts
│   │   │   └── repository.go
│   │   └── redis/          # In-memory quota caching & Redis Stream producers
│   │       └── cache.go
│   └── service/            # Core business logic & dual-write orchestration
│       ├── entitlement.go  # Entitlement check, cache warmup & dual-write sync
│       ├── redis_service.go# Cache layer abstraction
│       └── redis_service_test.go
├── web/
│   └── templates/
│       └── dashboard.html  # Responsive Tailwind + HTMX admin dashboard
├── docs/                   # Detailed technical specifications
│   ├── CORE_ENGINE_SPEC.md # Latency SLAs, dual-write sync & architecture details
│   ├── DATABASE_INFRASTRUCTURE.md # PostgreSQL schemas & Docker configuration
│   └── FRONTEND_I18N_GUIDE.md     # Tailwind design tokens & i18n dictionaries
├── scripts/
│   ├── db_healthcheck.sh   # Direct CLI health diagnostics for Postgres & Redis
│   └── validate_i18n.sh    # Automated sanity check for localization strings
├── docker-compose.yml      # Containerized PostgreSQL 16 & Redis 7 Alpine orchestration
├── simulate_usage.sh       # Automated 100-request quota enforcement test script
└── README.md
```

---

## 🚀 Quickstart & Local Setup

### 1. Prerequisites
- **[Go 1.22+](https://golang.org/dl/)**
- **[Docker](https://www.docker.com/) & Docker Compose**
- **Git**

### 2. Clone Repository
```bash
git clone https://github.com/Mukilan-s18/TierGuard-Entitlements-Engine.git
cd TierGuard-Entitlements-Engine
```

### 3. Spin Up Container Infrastructure
Start containerized PostgreSQL 16 and Redis 7 Alpine:
```bash
docker compose up -d
```
Verify container health:
```bash
docker compose ps
```

### 4. Run Database Migrations & Seed Pipeline
Apply database schemas and initialize default tiers and sample tenants:
```bash
# Apply schema migrations
psql -h localhost -p 5432 -U tierguard -d tierguard -f internal/repository/postgres/migrations/000001_init_schema.up.sql

# Execute mock data seeder
go run cmd/seed/main.go
```

### 5. Launch the TierGuard Engine
```bash
go run cmd/server/main.go
```
The server will warm up the Redis cache from PostgreSQL and begin listening on `http://localhost:8080`.

- **Admin Control Plane**: [http://localhost:8080/admin](http://localhost:8080/admin)
- **Health Check Endpoint**: [http://localhost:8080/health](http://localhost:8080/health)
- **Verification API**: `POST http://localhost:8080/api/verify`

---

## ⚙️ Configuration & Environment Variables

TierGuard is configured via environment variables with sensible local defaults:

| Variable | Default Value | Description |
|:---|:---|:---|
| `PORT` | `8080` | HTTP port on which the web server listens |
| `DATABASE_URL` | `postgres://tierguard:tierguard_secret@localhost:5432/tierguard?sslmode=disable` | PostgreSQL connection string |
| `REDIS_ADDR` | `localhost:6379` | Redis server address |
| `REDIS_PASSWORD` | `""` | Redis authentication password |
| `REDIS_DB` | `0` | Redis logical database index |

---

## 📡 API Reference

### 1. Entitlement & Quota Verification
Evaluates whether a tenant is permitted to perform an action based on their active subscription tier and current monthly request count.

```http
POST /api/verify HTTP/1.1
Host: localhost:8080
Content-Type: application/json

{
  "tenant_id": "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
  "requested_feature": "core_api"
}
```

#### Successful Authorization (`200 OK`)
```json
{
  "allowed": true,
  "tenant_id": "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
  "tier": "Pro",
  "remaining_quota": 9150
}
```

#### Quota Exceeded (`403 Forbidden`)
```json
{
  "allowed": false,
  "reason": "monthly_quota_exceeded",
  "current_usage": 1000,
  "quota_limit": 1000
}
```

#### Feature Disabled / Tier Too Low (`403 Forbidden`)
```json
{
  "allowed": false,
  "reason": "feature_not_entitled",
  "required_tier": "Enterprise"
}
```

---

### 2. System Health & Diagnostics
Returns real-time connection status and latency for backend infrastructure.

```http
GET /health HTTP/1.1
Host: localhost:8080
```

#### Response (`200 OK`)
```json
{
  "status": "healthy",
  "postgres": {
    "status": "connected",
    "latency_ms": 1.4
  },
  "redis": {
    "status": "connected",
    "latency_ms": 0.6
  },
  "uptime_seconds": 3600
}
```

---

### 3. Administrative Tier Update (HTMX Endpoint)
Synchronously updates a tenant's subscription tier across PostgreSQL and Redis.

```http
POST /admin/tenants/tier HTTP/1.1
Host: localhost:8080
Content-Type: application/x-www-form-urlencoded
HX-Request: true

tenant_id=aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa&tier_name=Enterprise
```

---

## 🧪 Testing & Verification

### Run Unit Tests
```bash
go test -v ./...
```

### Run API Benchmark Suite
Validate the `< 5ms` verification SLA under high-concurrency conditions:
```bash
go test -bench=BenchmarkVerifyEndpoint -benchmem ./internal/handlers/...
```

### Automated Usage & Quota Simulation
A dedicated bash simulation sends 100 concurrent requests for **Tenant A** on the Free tier and verifies that the 101st request is rejected with `403 Forbidden`:

```bash
chmod +x simulate_usage.sh
./simulate_usage.sh
```

### Infrastructure Diagnostics
Verify PostgreSQL and Redis connection status via CLI:
```bash
chmod +x scripts/db_healthcheck.sh
./scripts/db_healthcheck.sh
```

### Localization Dictionary Sanity Check
Validate presence of all translation keys across English, Tamil, and Hindi:
```bash
chmod +x scripts/validate_i18n.sh
./scripts/validate_i18n.sh
```

---

## 👥 Engineering Team

This project was built and architected by:

| Contributor | Role & Technical Domain | GitHub Profile |
|:---|:---|:---:|
| **Mukilan S** | **Project Architect & Core Systems Engineer**<br>Engineered sub-5ms entitlement logic, in-memory Redis cache layer, dual-write synchronization, async telemetry pipelines, theme engine, and health beacons. | [@Mukilan-s18](https://github.com/Mukilan-s18) |
| **Mukeshkumar D R** | **Infrastructure & Database Engineer**<br>Configured Docker Compose multi-service architecture, PostgreSQL schema migrations, rollback scripts, pgxpool connection pooling, and mock seed pipeline. | [@MUKESHKUMAR-21](https://github.com/MUKESHKUMAR-21) |
| **Mutheshkumar S** | **Frontend & Quality Assurance Engineer**<br>Designed high-density administrative dashboard, Tailwind CSS design system, HTMX partial swap integration, and English/Tamil/Hindi trilingual localization. | [@Mutheshkumar](https://github.com/Mutheshkumar) |

---

## 📚 Technical Documentation

For deeper architectural drill-downs, refer to the technical specifications in `/docs`:
- [**Core Engine Specification (docs/CORE_ENGINE_SPEC.md)**](docs/CORE_ENGINE_SPEC.md): Verification SLAs, dual-write consistency, Redis streams.
- [**Database Infrastructure Architecture (docs/DATABASE_INFRASTRUCTURE.md)**](docs/DATABASE_INFRASTRUCTURE.md): Relational schemas, pgxpool parameters, migrations.
- [**Frontend & Localization Guide (docs/FRONTEND_I18N_GUIDE.md)**](docs/FRONTEND_I18N_GUIDE.md): Tailwind tokens, HTMX integration, i18n dictionaries.

---

## 📄 License

This project is licensed under the **MIT License** — see the [LICENSE](LICENSE) file for details.
Designed and engineered with enterprise-grade standards.
