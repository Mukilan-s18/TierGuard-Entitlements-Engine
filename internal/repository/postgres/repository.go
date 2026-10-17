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
