package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/notify"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
)

type NotificationService struct {
	repo   *repository.NotificationRepository
	logger *zap.Logger

	// The delivery half. It is nil until AttachDelivery is called, which is
	// how a caller that only wants the in-panel inbox avoids paying for it -
	// and why every delivery entry point checks for nil and says so rather
	// than panicking.
	registry     *notify.Registry
	dispatcher   *notify.Dispatcher
	deliveryOpts DeliveryOptions
}

func NewNotificationService(repo *repository.NotificationRepository, logger *zap.Logger) *NotificationService {
	return &NotificationService{
		repo:   repo,
		logger: logger,
	}
}

// Notifications
func (s *NotificationService) Create(ctx context.Context, tenantID uuid.UUID, req *models.CreateNotificationRequest) (*models.Notification, error) {
	notification := &models.Notification{
		TenantID: tenantID,
		UserID:   req.UserID,
		Type:     req.Type,
		Title:    req.Title,
		Message:  req.Message,
		Details:  req.Details,
		IsRead:   false,
	}
	if err := s.repo.Create(ctx, notification); err != nil {
		return nil, err
	}
	return notification, nil
}

func (s *NotificationService) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*models.Notification, error) {
	return s.repo.GetByID(ctx, tenantID, id)
}

func (s *NotificationService) List(ctx context.Context, tenantID uuid.UUID, userID *uuid.UUID, isRead *bool, limit, offset int) ([]models.Notification, int, error) {
	return s.repo.List(ctx, tenantID, userID, isRead, limit, offset)
}

func (s *NotificationService) MarkAsRead(ctx context.Context, tenantID, id uuid.UUID) error {
	return s.repo.MarkAsRead(ctx, tenantID, id)
}

func (s *NotificationService) MarkAllAsRead(ctx context.Context, tenantID, userID uuid.UUID) error {
	return s.repo.MarkAllAsRead(ctx, tenantID, userID)
}

func (s *NotificationService) Delete(ctx context.Context, tenantID, id uuid.UUID) error {
	return s.repo.Delete(ctx, tenantID, id)
}

func (s *NotificationService) CleanupOld(ctx context.Context, tenantID uuid.UUID, days int) (int64, error) {
	return s.repo.DeleteOld(ctx, tenantID, days)
}

// Templates
func (s *NotificationService) CreateTemplate(ctx context.Context, tenantID uuid.UUID, req *models.CreateNotificationTemplateRequest) (*models.NotificationTemplate, error) {
	template := &models.NotificationTemplate{
		TenantID: tenantID,
		Name:     req.Name,
		Type:     req.Type,
		Subject:  req.Subject,
		Body:     req.Body,
		IsActive: true,
	}
	if err := s.repo.CreateTemplate(ctx, template); err != nil {
		return nil, err
	}
	return template, nil
}

func (s *NotificationService) GetTemplateByID(ctx context.Context, tenantID, id uuid.UUID) (*models.NotificationTemplate, error) {
	return s.repo.GetTemplateByID(ctx, tenantID, id)
}

func (s *NotificationService) ListTemplates(ctx context.Context, tenantID uuid.UUID) ([]models.NotificationTemplate, error) {
	return s.repo.ListTemplates(ctx, tenantID)
}

func (s *NotificationService) UpdateTemplate(ctx context.Context, tenantID, id uuid.UUID, req *models.UpdateNotificationTemplateRequest) (*models.NotificationTemplate, error) {
	return s.repo.UpdateTemplate(ctx, tenantID, id, req)
}

func (s *NotificationService) DeleteTemplate(ctx context.Context, tenantID, id uuid.UUID) error {
	return s.repo.DeleteTemplate(ctx, tenantID, id)
}

// Channels
func (s *NotificationService) CreateChannel(ctx context.Context, tenantID uuid.UUID, req *models.CreateNotificationChannelRequest) (*models.NotificationChannel, error) {
	channel := &models.NotificationChannel{
		TenantID: tenantID,
		Name:     req.Name,
		Type:     req.Type,
		Config:   req.Config,
		IsActive: true,
	}

	// Reject a channel this panel cannot send to at creation time. Accepting
	// it would give an operator a channel that looks configured and is only
	// discovered to be inert when an alert fails to arrive.
	if s.registry != nil && !s.registry.Supports(channel.Type) {
		return nil, fmt.Errorf("unsupported notification channel type %q (supported: %s)",
			channel.Type, strings.Join(s.registry.Types(), ", "))
	}
	// Build the sender once, purely to validate the config, and throw it away.
	// The alternative is learning that "host" was misspelled during an outage.
	if s.registry != nil {
		if _, err := s.registry.Build(channel.Type, notify.Config(channel.Config)); err != nil {
			return nil, notify.ScrubberForConfig(channel.Config).ScrubError(err)
		}
	}

	if err := s.repo.CreateChannel(ctx, channel); err != nil {
		return nil, err
	}
	return redactChannel(channel), nil
}

func (s *NotificationService) GetChannelByID(ctx context.Context, tenantID, id uuid.UUID) (*models.NotificationChannel, error) {
	channel, err := s.repo.GetChannelByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	return redactChannel(channel), nil
}

func (s *NotificationService) ListChannels(ctx context.Context, tenantID uuid.UUID) ([]models.NotificationChannel, error) {
	channels, err := s.repo.ListChannels(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	for i := range channels {
		redactChannel(&channels[i])
	}
	return channels, nil
}

func (s *NotificationService) UpdateChannel(ctx context.Context, tenantID, id uuid.UUID, req *models.UpdateNotificationChannelRequest) (*models.NotificationChannel, error) {
	channel, err := s.repo.UpdateChannel(ctx, tenantID, id, req)
	if err != nil {
		return nil, err
	}
	return redactChannel(channel), nil
}

func (s *NotificationService) DeleteChannel(ctx context.Context, tenantID, id uuid.UUID) error {
	return s.repo.DeleteChannel(ctx, tenantID, id)
}

// Preferences
func (s *NotificationService) GetPreferences(ctx context.Context, tenantID, userID uuid.UUID) ([]models.NotificationPreference, error) {
	return s.repo.GetPreferences(ctx, tenantID, userID)
}

func (s *NotificationService) SetPreference(ctx context.Context, tenantID, userID uuid.UUID, prefType, channel string, enabled bool) error {
	return s.repo.SetPreference(ctx, tenantID, userID, prefType, channel, enabled)
}

// ============================================================
// ALERT DELIVERY
//
// The path an alert takes from a monitoring check to a human:
//
//   Notify -> deduplicate -> render -> write one outbox row per channel
//                                          |
//   Dispatcher (background, RunDispatcher) -+-> senders -> retry -> dead letter
//
// Nothing below sends anything. Notify returns as soon as the rows are
// written, which is what keeps alerting off the request path of whatever was
// recording the metric.
// ============================================================

// AttachDelivery gives the service everything it needs to deliver alerts.
//
// It is a separate call rather than a constructor argument so that
// NewNotificationService keeps the signature every existing caller uses. A
// service without it still serves the in-panel notification inbox; it refuses
// alert delivery with a clear error rather than pretending to send.
func (s *NotificationService) AttachDelivery(registry *notify.Registry, opts DeliveryOptions) {
	if registry == nil {
		registry = notify.NewRegistry(notify.Deps{})
	}
	s.registry = registry
	s.deliveryOpts = opts.withDefaults()
	s.dispatcher = notify.NewDispatcher(s.repo, registry, s.logger, notify.DispatcherOptions{
		Interval:     s.deliveryOpts.Interval,
		BaseBackoff:  s.deliveryOpts.BaseBackoff,
		MaxBackoff:   s.deliveryOpts.MaxBackoff,
		SendTimeout:  s.deliveryOpts.SendTimeout,
		OnDeadLetter: s.recordDeadLetter,
	})
}

// DeliveryOptions configure alert delivery.
type DeliveryOptions struct {
	// PanelBaseURL is the panel's externally reachable base URL, used to build
	// the link in every message. Empty falls back to VKAI_PANEL_URL, and then
	// to a relative path.
	PanelBaseURL string
	// QuietPeriod is the default silence between repeat messages for one
	// incident. A caller can override it per alert.
	QuietPeriod time.Duration
	// MaxAttempts is the attempt budget per delivery before a dead letter.
	MaxAttempts int
	// Interval, BaseBackoff, MaxBackoff and SendTimeout are passed to the
	// dispatcher; zero means its default.
	Interval    time.Duration
	BaseBackoff time.Duration
	MaxBackoff  time.Duration
	SendTimeout time.Duration
	// Location renders timestamps in the operator's timezone. Nil means UTC.
	Location *time.Location
}

// withDefaults fills in what the caller left out.
func (o DeliveryOptions) withDefaults() DeliveryOptions {
	if o.PanelBaseURL == "" {
		o.PanelBaseURL = strings.TrimRight(strings.TrimSpace(os.Getenv("VKAI_PANEL_URL")), "/")
	}
	if o.QuietPeriod == 0 {
		o.QuietPeriod = notify.DefaultQuietPeriod
	}
	if o.MaxAttempts <= 0 {
		o.MaxAttempts = notify.DefaultMaxAttempts
	}
	return o
}

// ErrDeliveryNotConfigured is returned when alert delivery is asked for on a
// service that was never given a sender registry. It names the missing wiring
// rather than failing as "nil pointer", because the last audit found four
// features that were built and never connected.
var ErrDeliveryNotConfigured = errors.New(
	"notification delivery is not configured: AttachDelivery was never called on this service")

// NotifyResult reports what one call to Notify did, per channel and overall.
// It is returned to the API so an operator pressing a button, or a test,
// can see that a message was actually queued.
type NotifyResult struct {
	// Notified is false when deduplication suppressed the alert.
	Notified bool `json:"notified"`
	// Reason names the deduplication rule that decided.
	Reason string `json:"reason"`
	// Kind is the event that was rendered, when one was.
	Kind notify.EventKind `json:"kind"`
	// SuppressedUntil is set when Notified is false because of a quiet period.
	SuppressedUntil *time.Time `json:"suppressed_until,omitempty"`
	// DeliveryIDs are the outbox rows written, one per channel.
	DeliveryIDs []uuid.UUID `json:"delivery_ids,omitempty"`
	// Channels is how many active channels the alert fanned out to.
	Channels int `json:"channels"`
	// Occurrences is how many checks this incident has spanned.
	Occurrences int `json:"occurrences"`
}

// Notify is the entry point for a monitoring alert.
//
// It deduplicates, renders, and writes one outbox row per active channel. It
// does not send: the dispatcher does, in the background. A caller on a request
// path can call this and return.
func (s *NotificationService) Notify(ctx context.Context, tenantID uuid.UUID, alert notify.Alert) (*NotifyResult, error) {
	if s.registry == nil {
		return nil, ErrDeliveryNotConfigured
	}
	if err := alert.Validate(); err != nil {
		return nil, err
	}

	now := time.Now()
	alert.Normalize(now)

	quietPeriod := s.deliveryOpts.QuietPeriod
	if alert.Kind == notify.KindTest {
		// A test is never deduplicated. An operator pressing "test" twice
		// expects two messages, and silence would read as a broken channel.
		quietPeriod = 0
	}

	result := &NotifyResult{Kind: alert.Kind}

	if alert.Kind == notify.KindTest {
		result.Notified = true
		result.Reason = notify.ReasonTest
		result.Occurrences = 1
	} else {
		decision, err := s.repo.ObserveAlert(ctx, tenantID, alert.DedupKey, notify.Observation{
			Kind:        alert.Kind,
			At:          alert.OccurredAt,
			QuietPeriod: quietPeriod,
			Value:       alert.Value,
			Threshold:   alert.Threshold,
		})
		if err != nil {
			return nil, err
		}

		result.Notified = decision.Notify
		result.Reason = decision.Reason
		result.Kind = decision.Kind
		result.Occurrences = decision.State.Occurrences
		if !decision.SuppressedUntil.IsZero() {
			suppressed := decision.SuppressedUntil
			result.SuppressedUntil = &suppressed
		}

		if !decision.Notify {
			s.logger.Debug("Alert suppressed",
				zap.String("tenant_id", tenantID.String()),
				zap.String("dedup_key", alert.DedupKey),
				zap.String("reason", decision.Reason),
			)
			return result, nil
		}

		alert.Kind = decision.Kind
		alert.Occurrences = decision.State.Occurrences
		alert.FiringSince = decision.State.FirstSeenAt
	}

	// Fan out to every active channel for the tenant.
	//
	// notification_preferences is deliberately not consulted here, and that is
	// a decision rather than an oversight. Its rows are keyed
	// (tenant_id, user_id, type, channel): they say which in-panel
	// notification types a named person wants to see. A monitoring alert is
	// addressed to nobody in particular - it is addressed to whoever is
	// on call - so there is no user_id to look the preference up by, and
	// picking one would mean an alert going quiet because the person whose row
	// happened to be chosen had muted that type. Per-channel routing rules for
	// alerts belong on notification_channels, where an operator can see them,
	// not in a per-user preference table.
	channels, err := s.repo.ListActiveChannels(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if len(channels) == 0 {
		// Nothing to send to is worth a log line: an operator who configured
		// alerts but no channel has a monitoring system that cannot reach
		// them, and nothing else would ever say so.
		s.logger.Warn("Alert fired but this tenant has no active notification channel",
			zap.String("tenant_id", tenantID.String()),
			zap.String("dedup_key", alert.DedupKey),
			zap.String("subject", alert.Summary),
		)
		return result, nil
	}

	templates, err := s.templatesFor(ctx, tenantID)
	if err != nil {
		// A template that cannot be read must not cost an alert; the built-in
		// set is always usable.
		s.logger.Warn("Could not read notification templates, using the built-in ones",
			zap.String("tenant_id", tenantID.String()), zap.Error(err))
		templates = notify.DefaultTemplates()
	}

	message, renderErr := notify.Render(templates, alert, notify.RenderOptions{
		PanelBaseURL: s.deliveryOpts.PanelBaseURL,
		QuietPeriod:  quietPeriod,
		Location:     s.deliveryOpts.Location,
	})
	if renderErr != nil {
		s.logger.Warn("Notification template problem",
			zap.String("tenant_id", tenantID.String()), zap.Error(renderErr))
	}

	for _, channel := range channels {
		if !s.registry.Supports(channel.Type) {
			s.logger.Warn("Notification channel has a type this panel cannot send to",
				zap.String("channel", channel.Name),
				zap.String("channel_type", channel.Type),
				zap.Strings("supported", s.registry.Types()),
			)
			continue
		}
		id, err := s.repo.EnqueueDelivery(ctx, notify.EnqueueRequest{
			TenantID:    tenantID,
			ChannelID:   channel.ID,
			DedupKey:    alert.DedupKey,
			Kind:        alert.Kind,
			Subject:     message.Subject,
			Body:        message.Body,
			Alert:       message.Alert,
			MaxAttempts: s.deliveryOpts.MaxAttempts,
		})
		if err != nil {
			s.logger.Error("Could not queue an alert for delivery",
				zap.String("channel", channel.Name),
				zap.String("channel_type", channel.Type),
				zap.Error(err))
			continue
		}
		result.DeliveryIDs = append(result.DeliveryIDs, id)
	}
	result.Channels = len(result.DeliveryIDs)

	s.logger.Info("Alert queued for delivery",
		zap.String("tenant_id", tenantID.String()),
		zap.String("dedup_key", alert.DedupKey),
		zap.String("event", string(alert.Kind)),
		zap.String("severity", string(alert.Severity)),
		zap.Int("channels", result.Channels),
		zap.String("reason", result.Reason),
	)
	return result, nil
}

// TestChannel sends a test message over one channel, synchronously.
//
// It is deliberately not queued. An operator pressing "test" needs the actual
// error - "authentication failed", "no such host" - in the response, not a
// delivery id and a suggestion to check the logs. A channel that is only ever
// exercised during an incident is a channel nobody knows is broken.
func (s *NotificationService) TestChannel(ctx context.Context, tenantID, channelID uuid.UUID, serverName string) error {
	if s.registry == nil {
		return ErrDeliveryNotConfigured
	}

	channel, err := s.repo.GetChannelByID(ctx, tenantID, channelID)
	if err != nil {
		return err
	}

	sender, err := s.registry.Build(channel.Type, notify.Config(channel.Config))
	if err != nil {
		// Build errors are configuration errors; they already name the missing
		// setting. Scrub anyway: a factory may quote a value back.
		return notify.ScrubberForConfig(channel.Config).ScrubError(err)
	}

	if serverName == "" {
		serverName = "this panel"
	}
	alert := notify.Alert{
		Kind:       notify.KindTest,
		Severity:   notify.SeverityInfo,
		ServerName: serverName,
		Resource:   "notification channel",
		Summary:    fmt.Sprintf("Test message for the %q channel.", channel.Name),
		PanelPath:  "/settings/notifications",
		OccurredAt: time.Now(),
	}
	alert.Normalize(time.Now())

	templates, err := s.templatesFor(ctx, tenantID)
	if err != nil {
		templates = notify.DefaultTemplates()
	}
	message, renderErr := notify.Render(templates, alert, notify.RenderOptions{
		PanelBaseURL: s.deliveryOpts.PanelBaseURL,
		Location:     s.deliveryOpts.Location,
	})
	if renderErr != nil {
		s.logger.Warn("Notification template problem during a channel test", zap.Error(renderErr))
	}

	timeout := s.deliveryOpts.SendTimeout
	if timeout <= 0 {
		timeout = notify.DefaultSendTimeout
	}
	sendCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	scrub := notify.ScrubberForConfig(channel.Config)
	if err := sender.Send(sendCtx, message); err != nil {
		scrubbed := scrub.ScrubError(err)
		s.logger.Warn("Notification channel test failed",
			zap.String("channel", channel.Name),
			zap.String("channel_type", channel.Type),
			zap.Error(scrubbed),
		)
		return scrubbed
	}

	s.logger.Info("Notification channel test succeeded",
		zap.String("channel", channel.Name),
		zap.String("channel_type", channel.Type),
	)
	return nil
}

// RunDispatcher runs the delivery loop until the context is cancelled. It is
// the goroutine started from main; without it, nothing in the panel sends.
func (s *NotificationService) RunDispatcher(ctx context.Context) {
	if s.dispatcher == nil {
		s.logger.Error("Notification dispatcher not started: alert delivery is not configured, " +
			"so every queued alert will sit in notification_deliveries unsent")
		return
	}
	if err := s.repo.EnsureDeliverySchema(ctx); err != nil {
		s.logger.Error("Notification dispatcher not started", zap.Error(err))
		return
	}
	s.dispatcher.Run(ctx)
}

// DispatchOnce runs a single delivery pass. It exists for tests and for an
// operator-triggered flush, and it is what makes the loop above testable.
func (s *NotificationService) DispatchOnce(ctx context.Context) (int, error) {
	if s.dispatcher == nil {
		return 0, ErrDeliveryNotConfigured
	}
	return s.dispatcher.RunOnce(ctx)
}

// SupportedChannelTypes lists the channel types this panel can send to.
func (s *NotificationService) SupportedChannelTypes() []string {
	if s.registry == nil {
		return nil
	}
	return s.registry.Types()
}

// templatesFor overlays a tenant's stored templates onto the built-in set.
func (s *NotificationService) templatesFor(ctx context.Context, tenantID uuid.UUID) (notify.TemplateSet, error) {
	set := notify.DefaultTemplates()
	stored, err := s.repo.ListTemplates(ctx, tenantID)
	if err != nil {
		return set, err
	}
	for _, template := range stored {
		if !template.IsActive {
			continue
		}
		set = set.WithOverride(template.Type, template.Subject, template.Body)
	}
	return set, nil
}

// recordDeadLetter is the dispatcher's dead-letter hook. It writes an in-panel
// notification so that an alert nobody received still surfaces somewhere a
// human looks, instead of only in a log file.
func (s *NotificationService) recordDeadLetter(ctx context.Context, delivery notify.Delivery, cause string) {
	notification := &models.Notification{
		TenantID: delivery.TenantID,
		Type:     "notification_delivery_failed",
		Title:    fmt.Sprintf("Alert could not be delivered over %q", delivery.ChannelName),
		Message: fmt.Sprintf(
			"vKAI Panel gave up delivering an alert over the %s channel %q. The alert was: %s. Reason: %s",
			delivery.ChannelType, delivery.ChannelName, delivery.Subject, cause),
		Details: models.JSONMap{
			"delivery_id":  delivery.ID.String(),
			"channel_id":   delivery.ChannelID.String(),
			"channel_type": delivery.ChannelType,
			"channel_name": delivery.ChannelName,
			"dedup_key":    delivery.DedupKey,
			"event_kind":   string(delivery.Kind),
			"reason":       cause,
		},
	}
	if err := s.repo.Create(ctx, notification); err != nil {
		s.logger.Error("Could not record a dead-lettered alert in the panel inbox",
			zap.String("delivery_id", delivery.ID.String()),
			zap.Error(err))
	}
}

// ListDeliveries returns outbox rows for a tenant.
func (s *NotificationService) ListDeliveries(ctx context.Context, tenantID uuid.UUID, filter notify.DeliveryFilter) ([]notify.DeliveryRecord, int, error) {
	return s.repo.ListDeliveries(ctx, tenantID, filter)
}

// GetDelivery returns one outbox row.
func (s *NotificationService) GetDelivery(ctx context.Context, tenantID, id uuid.UUID) (*notify.DeliveryRecord, error) {
	return s.repo.GetDelivery(ctx, tenantID, id)
}

// RetryDelivery puts a dead letter back in the queue.
func (s *NotificationService) RetryDelivery(ctx context.Context, tenantID, id uuid.UUID) error {
	return s.repo.RetryDelivery(ctx, tenantID, id)
}

// ListAlertStates returns the deduplication state for a tenant.
func (s *NotificationService) ListAlertStates(ctx context.Context, tenantID uuid.UUID, limit int) ([]notify.AlertStateRecord, error) {
	return s.repo.ListAlertStates(ctx, tenantID, limit)
}

// redactChannel replaces every credential in a channel's config with a
// placeholder, in place.
//
// This is the boundary. Above it - the handlers, the API, the logs - a channel
// config never holds a real secret. Below it - the repository, the senders -
// it always does. Every read path in this service passes through here, and
// repository.UpdateChannel merges a redacted config back onto the stored one
// so that a client writing a redacted value back does not erase the real one.
func redactChannel(channel *models.NotificationChannel) *models.NotificationChannel {
	if channel == nil {
		return nil
	}
	channel.Config = models.JSONMap(notify.RedactConfig(channel.Config))
	return channel
}
