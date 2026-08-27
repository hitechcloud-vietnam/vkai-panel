package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/service"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
)

type SecurityHandler struct {
	securityService *service.SecurityService
	logger          *zap.Logger
}

func NewSecurityHandler(securityService *service.SecurityService, logger *zap.Logger) *SecurityHandler {
	return &SecurityHandler{
		securityService: securityService,
		logger:          logger,
	}
}

// CreateScan creates a new security scan
func (h *SecurityHandler) CreateScan(c *gin.Context) {
	var req models.CreateSecurityScanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.SendValidationError(c, map[string]string{
			"server_id": "Server ID is required",
			"scan_type": "Scan type is required",
		})
		return
	}

	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Error(c, http.StatusBadRequest, "Tenant ID is required")
		return
	}

	scan, err := h.securityService.CreateScan(c.Request.Context(), &req, tenantID)
	if err != nil {
		h.logger.Error("Failed to create security scan", zap.Error(err))
		utils.Error(c, http.StatusInternalServerError, "Failed to create security scan")
		return
	}

	utils.Created(c, scan)
}

// GetScan gets a security scan by ID
func (h *SecurityHandler) GetScan(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		utils.Error(c, http.StatusBadRequest, "Scan ID is required")
		return
	}

	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Error(c, http.StatusBadRequest, "Tenant ID is required")
		return
	}

	scan, err := h.securityService.GetScan(c.Request.Context(), id, tenantID)
	if err != nil {
		h.logger.Error("Failed to get security scan", zap.Error(err))
		utils.Error(c, http.StatusNotFound, "Security scan not found")
		return
	}

	utils.Success(c, scan)
}

// ListScans lists all security scans for a tenant
func (h *SecurityHandler) ListScans(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Error(c, http.StatusBadRequest, "Tenant ID is required")
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))

	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}

	scans, total, err := h.securityService.ListScans(c.Request.Context(), tenantID, page, perPage)
	if err != nil {
		h.logger.Error("Failed to list security scans", zap.Error(err))
		utils.Error(c, http.StatusInternalServerError, "Failed to list security scans")
		return
	}

	utils.Paginated(c, scans, int64(total), page, perPage)
}

// DeleteScan deletes a security scan
func (h *SecurityHandler) DeleteScan(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		utils.Error(c, http.StatusBadRequest, "Scan ID is required")
		return
	}

	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Error(c, http.StatusBadRequest, "Tenant ID is required")
		return
	}

	if err := h.securityService.DeleteScan(c.Request.Context(), id, tenantID); err != nil {
		h.logger.Error("Failed to delete security scan", zap.Error(err))
		utils.Error(c, http.StatusInternalServerError, "Failed to delete security scan")
		return
	}

	utils.Success(c, gin.H{"message": "Security scan deleted successfully"})
}

// GetVulnerability gets a vulnerability by ID
func (h *SecurityHandler) GetVulnerability(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		utils.Error(c, http.StatusBadRequest, "Vulnerability ID is required")
		return
	}

	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Error(c, http.StatusBadRequest, "Tenant ID is required")
		return
	}

	vuln, err := h.securityService.GetVulnerability(c.Request.Context(), id, tenantID)
	if err != nil {
		h.logger.Error("Failed to get vulnerability", zap.Error(err))
		utils.Error(c, http.StatusNotFound, "Vulnerability not found")
		return
	}

	utils.Success(c, vuln)
}

// ListVulnerabilitiesByScan lists all vulnerabilities for a scan
func (h *SecurityHandler) ListVulnerabilitiesByScan(c *gin.Context) {
	scanID := c.Param("scanId")
	if scanID == "" {
		utils.Error(c, http.StatusBadRequest, "Scan ID is required")
		return
	}

	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Error(c, http.StatusBadRequest, "Tenant ID is required")
		return
	}

	vulns, err := h.securityService.ListVulnerabilitiesByScan(c.Request.Context(), scanID, tenantID)
	if err != nil {
		h.logger.Error("Failed to list vulnerabilities", zap.Error(err))
		utils.Error(c, http.StatusInternalServerError, "Failed to list vulnerabilities")
		return
	}

	utils.Success(c, vulns)
}

// ListVulnerabilitiesByTenant lists all vulnerabilities for a tenant
func (h *SecurityHandler) ListVulnerabilitiesByTenant(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Error(c, http.StatusBadRequest, "Tenant ID is required")
		return
	}

	severity := c.Query("severity")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))

	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}

	vulns, total, err := h.securityService.ListVulnerabilitiesByTenant(c.Request.Context(), tenantID, severity, page, perPage)
	if err != nil {
		h.logger.Error("Failed to list vulnerabilities", zap.Error(err))
		utils.Error(c, http.StatusInternalServerError, "Failed to list vulnerabilities")
		return
	}

	utils.Paginated(c, vulns, int64(total), page, perPage)
}

// UpdateVulnerability updates a vulnerability
func (h *SecurityHandler) UpdateVulnerability(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		utils.Error(c, http.StatusBadRequest, "Vulnerability ID is required")
		return
	}

	var req models.UpdateSecurityVulnerabilityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.SendValidationError(c, map[string]string{"status": "Status is required"})
		return
	}

	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Error(c, http.StatusBadRequest, "Tenant ID is required")
		return
	}

	vuln, err := h.securityService.UpdateVulnerability(c.Request.Context(), id, tenantID, &req)
	if err != nil {
		h.logger.Error("Failed to update vulnerability", zap.Error(err))
		utils.Error(c, http.StatusInternalServerError, "Failed to update vulnerability")
		return
	}

	utils.Success(c, vuln)
}

// DeleteVulnerability deletes a vulnerability
func (h *SecurityHandler) DeleteVulnerability(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		utils.Error(c, http.StatusBadRequest, "Vulnerability ID is required")
		return
	}

	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Error(c, http.StatusBadRequest, "Tenant ID is required")
		return
	}

	if err := h.securityService.DeleteVulnerability(c.Request.Context(), id, tenantID); err != nil {
		h.logger.Error("Failed to delete vulnerability", zap.Error(err))
		utils.Error(c, http.StatusInternalServerError, "Failed to delete vulnerability")
		return
	}

	utils.Success(c, gin.H{"message": "Vulnerability deleted successfully"})
}

// ListChecksByScan lists all checks for a scan
func (h *SecurityHandler) ListChecksByScan(c *gin.Context) {
	scanID := c.Param("scanId")
	if scanID == "" {
		utils.Error(c, http.StatusBadRequest, "Scan ID is required")
		return
	}

	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Error(c, http.StatusBadRequest, "Tenant ID is required")
		return
	}

	checks, err := h.securityService.ListChecksByScan(c.Request.Context(), scanID, tenantID)
	if err != nil {
		h.logger.Error("Failed to list security checks", zap.Error(err))
		utils.Error(c, http.StatusInternalServerError, "Failed to list security checks")
		return
	}

	utils.Success(c, checks)
}

// CreatePolicy creates a new security policy
func (h *SecurityHandler) CreatePolicy(c *gin.Context) {
	var req models.CreateSecurityPolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.SendValidationError(c, map[string]string{
			"name":     "Name is required",
			"category": "Category is required",
			"rules":    "Rules are required",
		})
		return
	}

	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Error(c, http.StatusBadRequest, "Tenant ID is required")
		return
	}

	policy, err := h.securityService.CreatePolicy(c.Request.Context(), &req, tenantID)
	if err != nil {
		h.logger.Error("Failed to create security policy", zap.Error(err))
		utils.Error(c, http.StatusInternalServerError, "Failed to create security policy")
		return
	}

	utils.Created(c, policy)
}

// GetPolicy gets a security policy by ID
func (h *SecurityHandler) GetPolicy(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		utils.Error(c, http.StatusBadRequest, "Policy ID is required")
		return
	}

	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Error(c, http.StatusBadRequest, "Tenant ID is required")
		return
	}

	policy, err := h.securityService.GetPolicy(c.Request.Context(), id, tenantID)
	if err != nil {
		h.logger.Error("Failed to get security policy", zap.Error(err))
		utils.Error(c, http.StatusNotFound, "Security policy not found")
		return
	}

	utils.Success(c, policy)
}

// ListPolicies lists all security policies for a tenant
func (h *SecurityHandler) ListPolicies(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Error(c, http.StatusBadRequest, "Tenant ID is required")
		return
	}

	policies, err := h.securityService.ListPolicies(c.Request.Context(), tenantID)
	if err != nil {
		h.logger.Error("Failed to list security policies", zap.Error(err))
		utils.Error(c, http.StatusInternalServerError, "Failed to list security policies")
		return
	}

	utils.Success(c, policies)
}

// UpdatePolicy updates a security policy
func (h *SecurityHandler) UpdatePolicy(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		utils.Error(c, http.StatusBadRequest, "Policy ID is required")
		return
	}

	var req models.UpdateSecurityPolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, http.StatusBadRequest, "Invalid request body")
		return
	}

	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Error(c, http.StatusBadRequest, "Tenant ID is required")
		return
	}

	policy, err := h.securityService.UpdatePolicy(c.Request.Context(), id, tenantID, &req)
	if err != nil {
		h.logger.Error("Failed to update security policy", zap.Error(err))
		utils.Error(c, http.StatusInternalServerError, "Failed to update security policy")
		return
	}

	utils.Success(c, policy)
}

// DeletePolicy deletes a security policy
func (h *SecurityHandler) DeletePolicy(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		utils.Error(c, http.StatusBadRequest, "Policy ID is required")
		return
	}

	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		utils.Error(c, http.StatusBadRequest, "Tenant ID is required")
		return
	}

	if err := h.securityService.DeletePolicy(c.Request.Context(), id, tenantID); err != nil {
		h.logger.Error("Failed to delete security policy", zap.Error(err))
		utils.Error(c, http.StatusInternalServerError, "Failed to delete security policy")
		return
	}

	utils.Success(c, gin.H{"message": "Security policy deleted successfully"})
}
