package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"tierguard/internal/domain"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(ctx context.Context, connString string) (*Repository, error) {
	config, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, fmt.Errorf("failed to parse postgres config: %w", err)
	}

	config.MaxConns = 25
	config.MinConns = 5
	config.MaxConnLifetime = 30 * time.Minute
	config.MaxConnIdleTime = 5 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping postgres: %w", err)
	}

	return &Repository{pool: pool}, nil
}

func (r *Repository) Close() {
	if r.pool != nil {
		r.pool.Close()
	}
}

// Ping checks if the PostgreSQL database connection is alive.
func (r *Repository) Ping(ctx context.Context) error {
	if r.pool == nil {
		return fmt.Errorf("postgres pool is not initialized")
	}
	return r.pool.Ping(ctx)
}

// CreateTier inserts or updates a tier.
func (r *Repository) UpsertTier(ctx context.Context, tier domain.Tier) (domain.Tier, error) {
	if tier.ID == uuid.Nil {
		tier.ID = uuid.New()
	}
	query := `
		INSERT INTO tiers (id, name, hierarchy_level, max_monthly_requests)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (name) DO UPDATE 
		SET hierarchy_level = EXCLUDED.hierarchy_level,
		    max_monthly_requests = EXCLUDED.max_monthly_requests
		RETURNING id, name, hierarchy_level, max_monthly_requests
	`
	var t domain.Tier
	err := r.pool.QueryRow(ctx, query, tier.ID, tier.Name, tier.HierarchyLevel, tier.MaxMonthlyRequests).
		Scan(&t.ID, &t.Name, &t.HierarchyLevel, &t.MaxMonthlyRequests)
	return t, err
}

func (r *Repository) GetTierByName(ctx context.Context, name string) (*domain.Tier, error) {
	query := `SELECT id, name, hierarchy_level, max_monthly_requests FROM tiers WHERE name = $1`
	var t domain.Tier
	err := r.pool.QueryRow(ctx, query, name).Scan(&t.ID, &t.Name, &t.HierarchyLevel, &t.MaxMonthlyRequests)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &t, nil
}

func (r *Repository) GetAllTiers(ctx context.Context) ([]domain.Tier, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, name, hierarchy_level, max_monthly_requests FROM tiers ORDER BY hierarchy_level ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tiers []domain.Tier
	for rows.Next() {
		var t domain.Tier
		if err := rows.Scan(&t.ID, &t.Name, &t.HierarchyLevel, &t.MaxMonthlyRequests); err != nil {
			return nil, err
		}
		tiers = append(tiers, t)
	}
	return tiers, rows.Err()
}

// UpsertFeature inserts or updates a feature capability.
func (r *Repository) UpsertFeature(ctx context.Context, feature domain.Feature) (domain.Feature, error) {
	if feature.ID == uuid.Nil {
		feature.ID = uuid.New()
	}
	query := `
		INSERT INTO features (id, name, required_tier_level)
		VALUES ($1, $2, $3)
		ON CONFLICT (name) DO UPDATE 
		SET required_tier_level = EXCLUDED.required_tier_level
		RETURNING id, name, required_tier_level
	`
	var f domain.Feature
	err := r.pool.QueryRow(ctx, query, feature.ID, feature.Name, feature.RequiredTierLevel).
		Scan(&f.ID, &f.Name, &f.RequiredTierLevel)
	return f, err
}

func (r *Repository) GetAllFeatures(ctx context.Context) ([]domain.Feature, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, name, required_tier_level FROM features ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var features []domain.Feature
	for rows.Next() {
		var f domain.Feature
		if err := rows.Scan(&f.ID, &f.Name, &f.RequiredTierLevel); err != nil {
			return nil, err
		}
		features = append(features, f)
	}
	return features, rows.Err()
}

// CreateTenant inserts a new tenant.
func (r *Repository) CreateTenant(ctx context.Context, tenant domain.Tenant) (domain.Tenant, error) {
	if tenant.ID == uuid.Nil {
		tenant.ID = uuid.New()
	}
	query := `
		INSERT INTO tenants (id, company_name, tier_id)
		VALUES ($1, $2, $3)
		RETURNING id, company_name, tier_id
	`
	var t domain.Tenant
	err := r.pool.QueryRow(ctx, query, tenant.ID, tenant.CompanyName, tenant.TierID).
		Scan(&t.ID, &t.CompanyName, &t.TierID)
	return t, err
}

func (r *Repository) GetAllTenants(ctx context.Context) ([]domain.Tenant, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, company_name, tier_id FROM tenants ORDER BY company_name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tenants []domain.Tenant
	for rows.Next() {
		var t domain.Tenant
		if err := rows.Scan(&t.ID, &t.CompanyName, &t.TierID); err != nil {
			return nil, err
		}
		tenants = append(tenants, t)
	}
	return tenants, rows.Err()
}

func (r *Repository) GetTenantWithTier(ctx context.Context, tenantID uuid.UUID) (*domain.Tenant, *domain.Tier, error) {
	query := `
		SELECT t.id, t.company_name, t.tier_id, tr.id, tr.name, tr.hierarchy_level, tr.max_monthly_requests
		FROM tenants t
		JOIN tiers tr ON t.tier_id = tr.id
		WHERE t.id = $1
	`
	var tenant domain.Tenant
	var tier domain.Tier
	err := r.pool.QueryRow(ctx, query, tenantID).Scan(
		&tenant.ID, &tenant.CompanyName, &tenant.TierID,
		&tier.ID, &tier.Name, &tier.HierarchyLevel, &tier.MaxMonthlyRequests,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	return &tenant, &tier, nil
}

func (r *Repository) UpdateTenantTier(ctx context.Context, tenantID, tierID uuid.UUID) error {
	cmdTag, err := r.pool.Exec(ctx, `UPDATE tenants SET tier_id = $1 WHERE id = $2`, tierID, tenantID)
	if err != nil {
		return err
	}
	if cmdTag.RowsAffected() == 0 {
		return fmt.Errorf("tenant not found: %s", tenantID)
	}
	return nil
}

func (r *Repository) UpdateTenant(ctx context.Context, tenantID uuid.UUID, companyName string, tierID uuid.UUID) error {
	cmdTag, err := r.pool.Exec(ctx, `UPDATE tenants SET company_name = $1, tier_id = $2 WHERE id = $3`, companyName, tierID, tenantID)
	if err != nil {
		return err
	}
	if cmdTag.RowsAffected() == 0 {
		return fmt.Errorf("tenant not found: %s", tenantID)
	}
	return nil
}

func (r *Repository) IncrementUsage(ctx context.Context, tenantID uuid.UUID, month string, count int) error {
	query := `
		INSERT INTO usage_logs (id, tenant_id, request_count, month)
		VALUES (uuid_generate_v4(), $1, $2, $3)
		ON CONFLICT (tenant_id, month)
		DO UPDATE SET request_count = usage_logs.request_count + EXCLUDED.request_count
	`
	_, err := r.pool.Exec(ctx, query, tenantID, count, month)
	return err
}

func (r *Repository) SetUsage(ctx context.Context, tenantID uuid.UUID, month string, count int) error {
	query := `
		INSERT INTO usage_logs (id, tenant_id, request_count, month)
		VALUES (uuid_generate_v4(), $1, $2, $3)
		ON CONFLICT (tenant_id, month)
		DO UPDATE SET request_count = EXCLUDED.request_count
	`
	_, err := r.pool.Exec(ctx, query, tenantID, count, month)
	return err
}

func (r *Repository) GetUsage(ctx context.Context, tenantID uuid.UUID, month string) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `SELECT COALESCE(request_count, 0) FROM usage_logs WHERE tenant_id = $1 AND month = $2`, tenantID, month).Scan(&count)
	if err != nil {
		if err == pgx.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}
	return count, nil
}

func (r *Repository) GetUsageLogsByTenant(ctx context.Context, tenantID uuid.UUID) ([]domain.UsageLog, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, tenant_id, request_count, month FROM usage_logs WHERE tenant_id = $1 ORDER BY month DESC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []domain.UsageLog
	for rows.Next() {
		var ul domain.UsageLog
		if err := rows.Scan(&ul.ID, &ul.TenantID, &ul.RequestCount, &ul.Month); err != nil {
			return nil, err
		}
		logs = append(logs, ul)
	}
	return logs, rows.Err()
}

func (r *Repository) GetDashboardView(ctx context.Context, month string) ([]domain.TenantDashboardView, error) {
	query := `
		SELECT 
			t.id, 
			t.company_name, 
			tr.name, 
			tr.hierarchy_level, 
			tr.max_monthly_requests,
			COALESCE(u.request_count, 0) as current_usage
		FROM tenants t
		JOIN tiers tr ON t.tier_id = tr.id
		LEFT JOIN usage_logs u ON t.id = u.tenant_id AND u.month = $1
		ORDER BY CASE WHEN t.company_name = 'Acme Corp' THEN 0 ELSE 1 END, tr.hierarchy_level DESC, t.company_name ASC
	`
	rows, err := r.pool.Query(ctx, query, month)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []domain.TenantDashboardView
	for rows.Next() {
		var v domain.TenantDashboardView
		if err := rows.Scan(
			&v.TenantID,
			&v.CompanyName,
			&v.TierName,
			&v.HierarchyLevel,
			&v.MaxMonthlyRequests,
			&v.CurrentUsage,
		); err != nil {
			return nil, err
		}
		if v.MaxMonthlyRequests > 0 {
			v.UsagePercentage = (float64(v.CurrentUsage) / float64(v.MaxMonthlyRequests)) * 100.0
			if v.UsagePercentage > 100.0 {
				v.UsagePercentage = 100.0
			}
		}
		list = append(list, v)
	}
	return list, rows.Err()
}
