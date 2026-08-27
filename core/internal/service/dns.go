package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
)

type DNSService struct {
	dnsRepo *repository.DNSRepository
	logger  *zap.Logger
}

func NewDNSService(dnsRepo *repository.DNSRepository, logger *zap.Logger) *DNSService {
	return &DNSService{
		dnsRepo: dnsRepo,
		logger:  logger,
	}
}

// CreateZone creates a new DNS zone
func (s *DNSService) CreateZone(ctx context.Context, name, provider, tenantID string) (*models.DNSZone, error) {
	tenantUUID, err := uuid.Parse(tenantID)
	if err != nil {
		return nil, fmt.Errorf("invalid tenant ID: %w", err)
	}

	zone := &models.DNSZone{
		ID:       uuid.New(),
		TenantID: tenantUUID,
		Name:     name,
		Provider: provider,
		Status:   "active",
	}

	if err := s.dnsRepo.CreateZone(ctx, zone); err != nil {
		return nil, fmt.Errorf("failed to create DNS zone: %w", err)
	}

	s.logger.Info("DNS zone created",
		zap.String("id", zone.ID.String()),
		zap.String("name", zone.Name),
		zap.String("provider", zone.Provider),
	)

	return zone, nil
}

// GetZone gets a DNS zone by ID
func (s *DNSService) GetZone(ctx context.Context, id, tenantID string) (*models.DNSZone, error) {
	zoneUUID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("invalid zone ID: %w", err)
	}

	zone, err := s.dnsRepo.GetZoneByID(ctx, zoneUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to get DNS zone: %w", err)
	}

	// Verify tenant ownership
	tenantUUID, err := uuid.Parse(tenantID)
	if err != nil {
		return nil, fmt.Errorf("invalid tenant ID: %w", err)
	}

	if zone.TenantID != tenantUUID {
		return nil, fmt.Errorf("zone not found")
	}

	return zone, nil
}

// ListZones lists all DNS zones for a tenant
func (s *DNSService) ListZones(ctx context.Context, tenantID string) ([]models.DNSZone, error) {
	tenantUUID, err := uuid.Parse(tenantID)
	if err != nil {
		return nil, fmt.Errorf("invalid tenant ID: %w", err)
	}

	return s.dnsRepo.ListZonesByTenant(ctx, tenantUUID)
}

// UpdateZone updates a DNS zone
func (s *DNSService) UpdateZone(ctx context.Context, id, tenantID, provider, status string) (*models.DNSZone, error) {
	zone, err := s.GetZone(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}

	if provider != "" {
		zone.Provider = provider
	}
	if status != "" {
		zone.Status = status
	}

	if err := s.dnsRepo.UpdateZone(ctx, zone); err != nil {
		return nil, fmt.Errorf("failed to update DNS zone: %w", err)
	}

	s.logger.Info("DNS zone updated",
		zap.String("id", zone.ID.String()),
		zap.String("name", zone.Name),
	)

	return zone, nil
}

// DeleteZone deletes a DNS zone
func (s *DNSService) DeleteZone(ctx context.Context, id, tenantID string) error {
	zone, err := s.GetZone(ctx, id, tenantID)
	if err != nil {
		return err
	}

	if err := s.dnsRepo.DeleteZone(ctx, zone.ID); err != nil {
		return fmt.Errorf("failed to delete DNS zone: %w", err)
	}

	s.logger.Info("DNS zone deleted",
		zap.String("id", zone.ID.String()),
		zap.String("name", zone.Name),
	)

	return nil
}

// CreateRecord creates a new DNS record
func (s *DNSService) CreateRecord(ctx context.Context, zoneID, tenantID, recordType, name, value string, ttl int, priority *int) (*models.DNSRecord, error) {
	zoneUUID, err := uuid.Parse(zoneID)
	if err != nil {
		return nil, fmt.Errorf("invalid zone ID: %w", err)
	}

	tenantUUID, err := uuid.Parse(tenantID)
	if err != nil {
		return nil, fmt.Errorf("invalid tenant ID: %w", err)
	}

	// Verify zone exists and belongs to tenant
	zone, err := s.dnsRepo.GetZoneByID(ctx, zoneUUID)
	if err != nil {
		return nil, fmt.Errorf("zone not found: %w", err)
	}

	if zone.TenantID != tenantUUID {
		return nil, fmt.Errorf("zone not found")
	}

	record := &models.DNSRecord{
		ID:       uuid.New(),
		ZoneID:   zoneUUID,
		TenantID: tenantUUID,
		Type:     recordType,
		Name:     name,
		Value:    value,
		TTL:      ttl,
		Priority: priority,
		Status:   "active",
	}

	if err := s.dnsRepo.CreateRecord(ctx, record); err != nil {
		return nil, fmt.Errorf("failed to create DNS record: %w", err)
	}

	s.logger.Info("DNS record created",
		zap.String("id", record.ID.String()),
		zap.String("type", record.Type),
		zap.String("name", record.Name),
	)

	return record, nil
}

// GetRecord gets a DNS record by ID
func (s *DNSService) GetRecord(ctx context.Context, id, tenantID string) (*models.DNSRecord, error) {
	recordUUID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("invalid record ID: %w", err)
	}

	record, err := s.dnsRepo.GetRecordByID(ctx, recordUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to get DNS record: %w", err)
	}

	// Verify tenant ownership
	tenantUUID, err := uuid.Parse(tenantID)
	if err != nil {
		return nil, fmt.Errorf("invalid tenant ID: %w", err)
	}

	if record.TenantID != tenantUUID {
		return nil, fmt.Errorf("record not found")
	}

	return record, nil
}

// ListRecords lists all DNS records for a zone
func (s *DNSService) ListRecords(ctx context.Context, zoneID, tenantID string) ([]models.DNSRecord, error) {
	zoneUUID, err := uuid.Parse(zoneID)
	if err != nil {
		return nil, fmt.Errorf("invalid zone ID: %w", err)
	}

	tenantUUID, err := uuid.Parse(tenantID)
	if err != nil {
		return nil, fmt.Errorf("invalid tenant ID: %w", err)
	}

	// Verify zone exists and belongs to tenant
	zone, err := s.dnsRepo.GetZoneByID(ctx, zoneUUID)
	if err != nil {
		return nil, fmt.Errorf("zone not found: %w", err)
	}

	if zone.TenantID != tenantUUID {
		return nil, fmt.Errorf("zone not found")
	}

	return s.dnsRepo.ListRecordsByZone(ctx, zoneUUID)
}

// UpdateRecord updates a DNS record
func (s *DNSService) UpdateRecord(ctx context.Context, id, tenantID, recordType, name, value string, ttl int, priority *int, status string) (*models.DNSRecord, error) {
	record, err := s.GetRecord(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}

	if recordType != "" {
		record.Type = recordType
	}
	if name != "" {
		record.Name = name
	}
	if value != "" {
		record.Value = value
	}
	if ttl > 0 {
		record.TTL = ttl
	}
	if priority != nil {
		record.Priority = priority
	}
	if status != "" {
		record.Status = status
	}

	if err := s.dnsRepo.UpdateRecord(ctx, record); err != nil {
		return nil, fmt.Errorf("failed to update DNS record: %w", err)
	}

	s.logger.Info("DNS record updated",
		zap.String("id", record.ID.String()),
		zap.String("type", record.Type),
		zap.String("name", record.Name),
	)

	return record, nil
}

// DeleteRecord deletes a DNS record
func (s *DNSService) DeleteRecord(ctx context.Context, id, tenantID string) error {
	record, err := s.GetRecord(ctx, id, tenantID)
	if err != nil {
		return err
	}

	if err := s.dnsRepo.DeleteRecord(ctx, record.ID); err != nil {
		return fmt.Errorf("failed to delete DNS record: %w", err)
	}

	s.logger.Info("DNS record deleted",
		zap.String("id", record.ID.String()),
		zap.String("type", record.Type),
		zap.String("name", record.Name),
	)

	return nil
}
