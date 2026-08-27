package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/service"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
)

type DNSHandler struct {
	dnsService *service.DNSService
	logger     *zap.Logger
}

func NewDNSHandler(dnsService *service.DNSService, logger *zap.Logger) *DNSHandler {
	return &DNSHandler{
		dnsService: dnsService,
		logger:     logger,
	}
}

// CreateZone creates a new DNS zone
func (h *DNSHandler) CreateZone(c *gin.Context) {
	var req struct {
		Name     string `json:"name" binding:"required"`
		Provider string `json:"provider" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ValidationError(c, map[string]string{"name": "Name is required", "provider": "Provider is required"})
		return
	}

	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Error(c, http.StatusBadRequest, "Tenant ID is required")
		return
	}

	zone, err := h.dnsService.CreateZone(c.Request.Context(), req.Name, req.Provider, tenantID)
	if err != nil {
		h.logger.Error("Failed to create DNS zone", zap.Error(err))
		utils.Error(c, http.StatusInternalServerError, "Failed to create DNS zone")
		return
	}

	utils.Success(c, http.StatusCreated, zone)
}

// GetZone gets a DNS zone by ID
func (h *DNSHandler) GetZone(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		utils.Error(c, http.StatusBadRequest, "Zone ID is required")
		return
	}

	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Error(c, http.StatusBadRequest, "Tenant ID is required")
		return
	}

	zone, err := h.dnsService.GetZone(c.Request.Context(), id, tenantID)
	if err != nil {
		h.logger.Error("Failed to get DNS zone", zap.Error(err))
		utils.Error(c, http.StatusNotFound, "DNS zone not found")
		return
	}

	utils.Success(c, http.StatusOK, zone)
}

// ListZones lists all DNS zones for a tenant
func (h *DNSHandler) ListZones(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Error(c, http.StatusBadRequest, "Tenant ID is required")
		return
	}

	zones, err := h.dnsService.ListZones(c.Request.Context(), tenantID)
	if err != nil {
		h.logger.Error("Failed to list DNS zones", zap.Error(err))
		utils.Error(c, http.StatusInternalServerError, "Failed to list DNS zones")
		return
	}

	utils.Success(c, http.StatusOK, zones)
}

// UpdateZone updates a DNS zone
func (h *DNSHandler) UpdateZone(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		utils.Error(c, http.StatusBadRequest, "Zone ID is required")
		return
	}

	var req struct {
		Provider string `json:"provider"`
		Status   string `json:"status"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, http.StatusBadRequest, "Invalid request body")
		return
	}

	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Error(c, http.StatusBadRequest, "Tenant ID is required")
		return
	}

	zone, err := h.dnsService.UpdateZone(c.Request.Context(), id, tenantID, req.Provider, req.Status)
	if err != nil {
		h.logger.Error("Failed to update DNS zone", zap.Error(err))
		utils.Error(c, http.StatusInternalServerError, "Failed to update DNS zone")
		return
	}

	utils.Success(c, http.StatusOK, zone)
}

// DeleteZone deletes a DNS zone
func (h *DNSHandler) DeleteZone(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		utils.Error(c, http.StatusBadRequest, "Zone ID is required")
		return
	}

	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Error(c, http.StatusBadRequest, "Tenant ID is required")
		return
	}

	if err := h.dnsService.DeleteZone(c.Request.Context(), id, tenantID); err != nil {
		h.logger.Error("Failed to delete DNS zone", zap.Error(err))
		utils.Error(c, http.StatusInternalServerError, "Failed to delete DNS zone")
		return
	}

	utils.Success(c, http.StatusOK, gin.H{"message": "DNS zone deleted successfully"})
}

// CreateRecord creates a new DNS record
func (h *DNSHandler) CreateRecord(c *gin.Context) {
	zoneID := c.Param("zoneId")
	if zoneID == "" {
		utils.Error(c, http.StatusBadRequest, "Zone ID is required")
		return
	}

	var req struct {
		Type     string `json:"type" binding:"required"`
		Name     string `json:"name" binding:"required"`
		Value    string `json:"value" binding:"required"`
		TTL      int    `json:"ttl"`
		Priority *int   `json:"priority"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, http.StatusBadRequest, "Invalid request body")
		return
	}

	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Error(c, http.StatusBadRequest, "Tenant ID is required")
		return
	}

	if req.TTL == 0 {
		req.TTL = 3600
	}

	record, err := h.dnsService.CreateRecord(c.Request.Context(), zoneID, tenantID, req.Type, req.Name, req.Value, req.TTL, req.Priority)
	if err != nil {
		h.logger.Error("Failed to create DNS record", zap.Error(err))
		utils.Error(c, http.StatusInternalServerError, "Failed to create DNS record")
		return
	}

	utils.Success(c, http.StatusCreated, record)
}

// GetRecord gets a DNS record by ID
func (h *DNSHandler) GetRecord(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		utils.Error(c, http.StatusBadRequest, "Record ID is required")
		return
	}

	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Error(c, http.StatusBadRequest, "Tenant ID is required")
		return
	}

	record, err := h.dnsService.GetRecord(c.Request.Context(), id, tenantID)
	if err != nil {
		h.logger.Error("Failed to get DNS record", zap.Error(err))
		utils.Error(c, http.StatusNotFound, "DNS record not found")
		return
	}

	utils.Success(c, http.StatusOK, record)
}

// ListRecords lists all DNS records for a zone
func (h *DNSHandler) ListRecords(c *gin.Context) {
	zoneID := c.Param("zoneId")
	if zoneID == "" {
		utils.Error(c, http.StatusBadRequest, "Zone ID is required")
		return
	}

	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Error(c, http.StatusBadRequest, "Tenant ID is required")
		return
	}

	records, err := h.dnsService.ListRecords(c.Request.Context(), zoneID, tenantID)
	if err != nil {
		h.logger.Error("Failed to list DNS records", zap.Error(err))
		utils.Error(c, http.StatusInternalServerError, "Failed to list DNS records")
		return
	}

	utils.Success(c, http.StatusOK, records)
}

// UpdateRecord updates a DNS record
func (h *DNSHandler) UpdateRecord(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		utils.Error(c, http.StatusBadRequest, "Record ID is required")
		return
	}

	var req struct {
		Type     string `json:"type"`
		Name     string `json:"name"`
		Value    string `json:"value"`
		TTL      int    `json:"ttl"`
		Priority *int   `json:"priority"`
		Status   string `json:"status"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, http.StatusBadRequest, "Invalid request body")
		return
	}

	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Error(c, http.StatusBadRequest, "Tenant ID is required")
		return
	}

	record, err := h.dnsService.UpdateRecord(c.Request.Context(), id, tenantID, req.Type, req.Name, req.Value, req.TTL, req.Priority, req.Status)
	if err != nil {
		h.logger.Error("Failed to update DNS record", zap.Error(err))
		utils.Error(c, http.StatusInternalServerError, "Failed to update DNS record")
		return
	}

	utils.Success(c, http.StatusOK, record)
}

// DeleteRecord deletes a DNS record
func (h *DNSHandler) DeleteRecord(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		utils.Error(c, http.StatusBadRequest, "Record ID is required")
		return
	}

	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Error(c, http.StatusBadRequest, "Tenant ID is required")
		return
	}

	if err := h.dnsService.DeleteRecord(c.Request.Context(), id, tenantID); err != nil {
		h.logger.Error("Failed to delete DNS record", zap.Error(err))
		utils.Error(c, http.StatusInternalServerError, "Failed to delete DNS record")
		return
	}

	utils.Success(c, http.StatusOK, gin.H{"message": "DNS record deleted successfully"})
}
