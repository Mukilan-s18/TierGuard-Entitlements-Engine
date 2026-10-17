Project Context For AI Assistant
We are building an MVP for a SaaS entitlement and metering engine named TierGuard. The backend relies strictly on Go using the standard net/http library. The database is PostgreSQL. The cache is Redis. The frontend uses Go HTML templates injected with HTMX and Tailwind CSS via CDN. Do not generate React code. Do not write single page applications. Output clean, modular Go code separating the database repository layer from the HTTP handlers.

Technical Requirements Document
The system must resolve authorization checks in under five milliseconds. We achieve this by checking Redis first. The Go backend reads PostgreSQL only on startup or when a product manager updates a tier. It pushes that data into Redis.

The main verification endpoint must handle concurrent requests. Protect the Go maps and database connection pools using proper mutexes or connection limits. Use the pgx library for PostgreSQL. Use the go-redis library for caching.

Strict Database Schema
Write the Go structs and SQL migration scripts matching this exact structure.

Table tiers. Fields include id (UUID), name (VARCHAR), hierarchy_level (INT), max_monthly_requests (INT).
Table features. Fields include id (UUID), name (VARCHAR), required_tier_level (INT).
Table tenants. Fields include id (UUID), company_name (VARCHAR), tier_id (UUID foreign key).
Table usage_logs. Fields include id (UUID), tenant_id (UUID), request_count (INT), month (VARCHAR).

Core API Application Flow
The AI must generate one primary internal endpoint for the SaaS application to call.

Endpoint POST /api/verify.
The client sends a JSON payload containing tenant_id and requested_feature.
The Go handler queries Redis. It compares the tenant current tier against the feature required_tier_level. It checks if current usage is below max_monthly_requests.
It returns a 200 status with JSON {"allowed": true} if valid.
It returns a 403 status with JSON {"allowed": false, "reason": "quota_exceeded"} if invalid.
It fires a Go goroutine to increment the usage count asynchronously. Do not block the HTTP response waiting for the database update.

UI UX HTMX Design Brief
Generate a brutalist, high contrast dashboard using Tailwind CSS. Use a white background with slate gray borders.

The main view is a table listing all tenants. Each row shows the tenant name, their active tier, and a progress bar showing their usage quota.
Include a simple HTML form at the top. The form takes a tenant ID and a dropdown to select a new tier.
Use the hx-post attribute on the form to submit to /admin/update-tier. Use the hx-swap attribute to replace the tenant table instantly without a full page reload.