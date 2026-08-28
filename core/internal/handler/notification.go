package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/middleware"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/notify"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/service"
)

type NotificationHandler struct {
	service *service.NotificationService
	logger  *zap.Logger
}

func NewNotificationHandler(service *service.NotificationService, logger *zap.Logger) *NotificationHandler {
	return &NotificationHandler{
		service: service,
		logger:  logger,
	}
}

// Notifications
func (h *NotificationHandler) Create(c *gin.Context) {
	var req models.CreateNotificationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	notification, err := h.service.Create(c.Request.Context(), tenantID, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, notification)
}

func (h *NotificationHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid notification id"})
		return
	}

	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	notification, err := h.service.GetByID(c.Request.Context(), tenantID, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, notification)
}

func (h *NotificationHandler) List(c *gin.Context) {
	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	var userID *uuid.UUID
	if userIDStr := c.Query("user_id"); userIDStr != "" {
		uid, err := uuid.Parse(userIDStr)
		if err == nil {
			userID = &uid
		}
	}

	var isRead *bool
	if isReadStr := c.Query("is_read"); isReadStr != "" {
		read := isReadStr == "true"
		isRead = &read
	}

	notifications, total, err := h.service.List(c.Request.Context(), tenantID, userID, isRead, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"notifications": notifications,
		"total":         total,
		"limit":         limit,
		"offset":        offset,
	})
}

func (h *NotificationHandler) MarkAsRead(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid notification id"})
		return
	}

	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	if err := h.service.MarkAsRead(c.Request.Context(), tenantID, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "notification marked as read"})
}

func (h *NotificationHandler) MarkAllAsRead(c *gin.Context) {
	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	userID := c.MustGet("user_id").(uuid.UUID)

	if err := h.service.MarkAllAsRead(c.Request.Context(), tenantID, userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "all notifications marked as read"})
}

func (h *NotificationHandler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid notification id"})
		return
	}

	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	if err := h.service.Delete(c.Request.Context(), tenantID, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "notification deleted"})
}

func (h *NotificationHandler) CleanupOld(c *gin.Context) {
	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	days, _ := strconv.Atoi(c.DefaultQuery("days", "30"))

	deleted, err := h.service.CleanupOld(c.Request.Context(), tenantID, days)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "old notifications cleaned up",
		"deleted": deleted,
	})
}

// Templates
func (h *NotificationHandler) CreateTemplate(c *gin.Context) {
	var req models.CreateNotificationTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	template, err := h.service.CreateTemplate(c.Request.Context(), tenantID, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, template)
}

func (h *NotificationHandler) GetTemplate(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid template id"})
		return
	}

	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	template, err := h.service.GetTemplateByID(c.Request.Context(), tenantID, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, template)
}

func (h *NotificationHandler) ListTemplates(c *gin.Context) {
	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	templates, err := h.service.ListTemplates(c.Request.Context(), tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"templates": templates})
}

func (h *NotificationHandler) UpdateTemplate(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid template id"})
		return
	}

	var req models.UpdateNotificationTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	template, err := h.service.UpdateTemplate(c.Request.Context(), tenantID, id, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, template)
}

func (h *NotificationHandler) DeleteTemplate(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid template id"})
		return
	}

	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	if err := h.service.DeleteTemplate(c.Request.Context(), tenantID, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "template deleted"})
}

// Channels
func (h *NotificationHandler) CreateChannel(c *gin.Context) {
	var req models.CreateNotificationChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	channel, err := h.service.CreateChannel(c.Request.Context(), tenantID, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, channel)
}

func (h *NotificationHandler) GetChannel(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid channel id"})
		return
	}

	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	channel, err := h.service.GetChannelByID(c.Request.Context(), tenantID, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, channel)
}

func (h *NotificationHandler) ListChannels(c *gin.Context) {
	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	channels, err := h.service.ListChannels(c.Request.Context(), tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"channels": channels})
}

func (h *NotificationHandler) UpdateChannel(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid channel id"})
		return
	}

	var req models.UpdateNotificationChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	channel, err := h.service.UpdateChannel(c.Request.Context(), tenantID, id, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, channel)
}

func (h *NotificationHandler) DeleteChannel(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid channel id"})
		return
	}

	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	if err := h.service.DeleteChannel(c.Request.Context(), tenantID, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "channel deleted"})
}

// Preferences
func (h *NotificationHandler) GetPreferences(c *gin.Context) {
	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	userID := c.MustGet("user_id").(uuid.UUID)

	preferences, err := h.service.GetPreferences(c.Request.Context(), tenantID, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"preferences": preferences})
}

func (h *NotificationHandler) SetPreference(c *gin.Context) {
	var req models.UpdateNotificationPreferenceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	userID := c.MustGet("user_id").(uuid.UUID)

	if err := h.service.SetPreference(c.Request.Context(), tenantID, userID, req.Type, req.Channel, req.Enabled); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "preference updated"})
}

// ============================================================
// ALERT DELIVERY ENDPOINTS
//
// Mounted by RegisterNotifyRoutes below. router.go needs exactly one line for
// all of it; see the comment on that function.
// ============================================================

// RegisterNotifyRoutes mounts the alert delivery endpoints on an
// authenticated group.
//
// It is the single entry point, so internal/handler/router.go needs one line
// inside its `protected` block and nothing else:
//
//	RegisterNotifyRoutes(protected, r.notificationHandler)
//
// It creates its own /notifications subgroup carrying the same permission
// check as the existing notification routes, so the two sets are guarded
// identically and neither can drift.
//
// The reason this exists as a function rather than as lines in router.go: a
// route that is never mounted is a feature that passes its tests and does
// nothing, which is the failure this project has already paid for once.
func RegisterNotifyRoutes(rg *gin.RouterGroup, h *NotificationHandler) {
	// There is deliberately no nil check on h. Registration takes method
	// values, never calls, so a nil handler produces exactly the same route
	// table as a live one - which is what lets router_test.go assert this
	// mount against the real NewRouter, where the database-backed handlers are
	// nil. A guard here would make the routes vanish from that table and hide
	// the very regression the test exists to catch.
	group := rg.Group("/notifications", middleware.RequirePermission("notifications"))
	{
		// Ingress: a monitoring check reports that a threshold is crossed, or
		// that it has cleared. Returns as soon as the outbox rows are written.
		group.POST("/alerts", h.PublishAlert)
		// What deduplication currently believes, so "why did I not get a
		// message" has an answer that is not a database client.
		group.GET("/alerts/state", h.ListAlertStates)

		// Prove a channel works before an incident does it for you.
		group.POST("/channels/:id/test", h.TestChannel)
		// What this panel can actually send to.
		group.GET("/channel-types", h.ListChannelTypes)

		// The outbox, including ?status=dead_letter - the list of alerts that
		// reached nobody.
		group.GET("/deliveries", h.ListDeliveries)
		group.GET("/deliveries/:id", h.GetDelivery)
		group.POST("/deliveries/:id/retry", h.RetryDelivery)
	}
}

// PublishAlertRequest is the body of POST /notifications/alerts.
type PublishAlertRequest struct {
	DedupKey   string            `json:"dedup_key" binding:"required"`
	Kind       string            `json:"kind" binding:"required"`
	Severity   string            `json:"severity"`
	ServerID   string            `json:"server_id"`
	ServerName string            `json:"server_name"`
	Resource   string            `json:"resource"`
	Metric     string            `json:"metric"`
	Value      float64           `json:"value"`
	Threshold  float64           `json:"threshold"`
	Condition  string            `json:"condition"`
	Unit       string            `json:"unit"`
	Summary    string            `json:"summary"`
	PanelPath  string            `json:"panel_path"`
	Extra      map[string]string `json:"extra"`
}

// PublishAlert handles POST /notifications/alerts.
//
// It answers 202 rather than 200 even when a message was queued, because
// nothing has been sent yet - the dispatcher does that. A response that said
// 200 OK would be claiming a delivery this endpoint cannot know about.
func (h *NotificationHandler) PublishAlert(c *gin.Context) {
	var req PublishAlertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	alert := notify.Alert{
		DedupKey:   req.DedupKey,
		Kind:       notify.EventKind(req.Kind),
		Severity:   notify.Severity(req.Severity),
		ServerID:   req.ServerID,
		ServerName: req.ServerName,
		Resource:   req.Resource,
		Metric:     req.Metric,
		Value:      req.Value,
		Threshold:  req.Threshold,
		Condition:  req.Condition,
		Unit:       req.Unit,
		Summary:    req.Summary,
		PanelPath:  req.PanelPath,
		Extra:      req.Extra,
	}

	result, err := h.service.Notify(c.Request.Context(), tenantID, alert)
	if err != nil {
		switch {
		case errors.Is(err, notify.ErrInvalidAlert):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, service.ErrDeliveryNotConfigured):
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		default:
			h.logger.Error("Could not queue an alert", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusAccepted, result)
}

// ListAlertStates handles GET /notifications/alerts/state.
func (h *NotificationHandler) ListAlertStates(c *gin.Context) {
	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))

	states, err := h.service.ListAlertStates(c.Request.Context(), tenantID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"states": states, "total": len(states)})
}

// TestChannel handles POST /notifications/channels/:id/test.
//
// It sends synchronously and returns the real failure. A test that queued a
// message and answered "accepted" would tell an operator nothing about whether
// the channel works, which is the only question they asked.
func (h *NotificationHandler) TestChannel(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid channel id"})
		return
	}

	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	if err := h.service.TestChannel(c.Request.Context(), tenantID, id, c.Query("server_name")); err != nil {
		if errors.Is(err, service.ErrDeliveryNotConfigured) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
			return
		}
		// The error has already been scrubbed by the service. It is returned
		// verbatim because "authentication failed" is exactly what the
		// operator needs, and it is the whole point of a test endpoint.
		c.JSON(http.StatusBadGateway, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "test notification sent",
	})
}

// ListChannelTypes handles GET /notifications/channel-types.
func (h *NotificationHandler) ListChannelTypes(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"types": h.service.SupportedChannelTypes()})
}

// ListDeliveries handles GET /notifications/deliveries.
func (h *NotificationHandler) ListDeliveries(c *gin.Context) {
	tenantID := c.MustGet("tenant_id").(uuid.UUID)

	filter := notify.DeliveryFilter{
		Status:   c.Query("status"),
		DedupKey: c.Query("dedup_key"),
	}
	filter.Limit, _ = strconv.Atoi(c.DefaultQuery("limit", "50"))
	filter.Offset, _ = strconv.Atoi(c.DefaultQuery("offset", "0"))

	if raw := c.Query("channel_id"); raw != "" {
		channelID, err := uuid.Parse(raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid channel_id"})
			return
		}
		filter.ChannelID = &channelID
	}

	deliveries, total, err := h.service.ListDeliveries(c.Request.Context(), tenantID, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"deliveries": deliveries,
		"total":      total,
		"limit":      filter.Limit,
		"offset":     filter.Offset,
	})
}

// GetDelivery handles GET /notifications/deliveries/:id.
func (h *NotificationHandler) GetDelivery(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid delivery id"})
		return
	}

	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	delivery, err := h.service.GetDelivery(c.Request.Context(), tenantID, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, delivery)
}

// RetryDelivery handles POST /notifications/deliveries/:id/retry.
func (h *NotificationHandler) RetryDelivery(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid delivery id"})
		return
	}

	tenantID := c.MustGet("tenant_id").(uuid.UUID)
	if err := h.service.RetryDelivery(c.Request.Context(), tenantID, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "no dead-lettered delivery with that id; only a dead letter can be retried",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"message": "delivery queued for another attempt"})
}
