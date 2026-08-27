package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
)

type DNSRepository struct {
	db *sqlx.DB
}

func NewDNSRepository(db *sqlx.DB) *DNSRepository {
	return &DNSRepository{db: db}
}

// DNS Zone operations
func (r *DNSRepository) CreateZone(ctx context.Context, z *models.DNSZone) error {
	query := `
		INSERT INTO dns_zones (id, tenant_id, name, provider, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
		RETURNING created_at, updated_at`

	z.ID = uuid.New()
	return r.db.QueryRowContext(ctx, query,
		z.ID, z.TenantID, z.Name, z.Provider, z.Status,
	).Scan(&z.CreatedAt, &z.UpdatedAt)
}

func (r *DNSRepository) GetZoneByID(ctx context.Context, id uuid.UUID) (*models.DNSZone, error) {
	var z models.DNSZone
	query := `SELECT * FROM dns_zones WHERE id = $1`
	if err := r.db.GetContext(ctx, &z, query, id); err != nil {
		return nil, fmt.Errorf("DNS zone not found: %w", err)
	}
	return &z, nil
}

func (r *DNSRepository) GetZoneByName(ctx context.Context, name string) (*models.DNSZone, error) {
	var z models.DNSZone
	query := `SELECT * FROM dns_zones WHERE name = $1`
	if err := r.db.GetContext(ctx, &z, query, name); err != nil {
		return nil, fmt.Errorf("DNS zone not found: %w", err)
	}
	return &z, nil
}

func (r *DNSRepository) ListZonesByTenant(ctx context.Context, tenantID uuid.UUID) ([]models.DNSZone, error) {
	var zones []models.DNSZone
	query := `SELECT * FROM dns_zones WHERE tenant_id = $1 ORDER BY name`
	if err := r.db.SelectContext(ctx, &zones, query, tenantID); err != nil {
		return nil, err
	}
	return zones, nil
}

func (r *DNSRepository) UpdateZone(ctx context.Context, z *models.DNSZone) error {
	query := `UPDATE dns_zones SET provider = $2, status = $3, updated_at = NOW() WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, z.ID, z.Provider, z.Status)
	return err
}

func (r *DNSRepository) DeleteZone(ctx context.Context, id uuid.UUID) error {
	// Delete all records first
	_, err := r.db.ExecContext(ctx, `DELETE FROM dns_records WHERE zone_id = $1`, id)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `DELETE FROM dns_zones WHERE id = $1`, id)
	return err
}

// DNS Record operations
func (r *DNSRepository) CreateRecord(ctx context.Context, rec *models.DNSRecord) error {
	query := `
		INSERT INTO dns_records (id, zone_id, tenant_id, type, name, value, ttl, priority, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id`

	rec.ID = uuid.New()
	return r.db.QueryRowContext(ctx, query,
		rec.ID, rec.ZoneID, rec.TenantID, rec.Type, rec.Name,
		rec.Value, rec.TTL, rec.Priority, rec.Status,
	).Scan(&rec.ID)
}

func (r *DNSRepository) GetRecordByID(ctx context.Context, id uuid.UUID) (*models.DNSRecord, error) {
	var rec models.DNSRecord
	query := `SELECT * FROM dns_records WHERE id = $1`
	if err := r.db.GetContext(ctx, &rec, query, id); err != nil {
		return nil, fmt.Errorf("DNS record not found: %w", err)
	}
	return &rec, nil
}

func (r *DNSRepository) ListRecordsByZone(ctx context.Context, zoneID uuid.UUID) ([]models.DNSRecord, error) {
	var records []models.DNSRecord
	query := `SELECT * FROM dns_records WHERE zone_id = $1 ORDER BY type, name`
	if err := r.db.SelectContext(ctx, &records, query, zoneID); err != nil {
		return nil, err
	}
	return records, nil
}

func (r *DNSRepository) UpdateRecord(ctx context.Context, rec *models.DNSRecord) error {
	query := `UPDATE dns_records SET type = $2, name = $3, value = $4, ttl = $5, priority = $6, status = $7 WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query,
		rec.ID, rec.Type, rec.Name, rec.Value, rec.TTL, rec.Priority, rec.Status,
	)
	return err
}

func (r *DNSRepository) DeleteRecord(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM dns_records WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}
