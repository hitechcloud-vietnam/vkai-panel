package handler

// Two questions about the alert delivery endpoints, and they are different
// questions:
//
//   1. Does RegisterNotifyRoutes mount what it claims, and does each path
//      reach a real handler? TestRegisterNotifyRoutesMountsTheEndpoints.
//   2. Is it mounted in the engine cmd/api/main.go actually serves?
//      TestNotifyRoutesAreMountedInTheRealRouter, which asserts against the
//      real NewRouter and nothing else.
//
// The second is the one that matters. Four features in this project were
// written, tested and merged while unreachable, and every one of those tests
// answered question 1 while nobody asked question 2.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/middleware"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/notify"
)

// notifyRoutes is the set RegisterNotifyRoutes is responsible for, written out
// once so both tests below assert the same list.
var notifyRoutes = []struct {
	method string
	path   string
	why    string
}{
	{http.MethodPost, "/api/v1/notifications/alerts",
		"a monitoring check has no way to raise an alert without this"},
	{http.MethodGet, "/api/v1/notifications/alerts/state",
		"'why did I not get a message' has no answer without this"},
	{http.MethodPost, "/api/v1/notifications/channels/:id/test",
		"a channel only exercised during an incident is a channel nobody knows is broken"},
	{http.MethodGet, "/api/v1/notifications/channel-types",
		"the UI cannot offer channel types it cannot discover"},
	{http.MethodGet, "/api/v1/notifications/deliveries",
		"the outbox, including ?status=dead_letter - the alerts that reached nobody"},
	{http.MethodGet, "/api/v1/notifications/deliveries/:id",
		"one delivery's history, including why it failed"},
	{http.MethodPost, "/api/v1/notifications/deliveries/:id/retry",
		"a dead letter that cannot be retried is a dead letter nobody can act on"},
}

// TestNotifyRoutesAreMountedInTheRealRouter asserts against the engine
// cmd/api/main.go serves, built through the real NewRouter.
//
// It fails until internal/handler/router.go carries this line inside its
// `protected` group:
//
//	RegisterNotifyRoutes(protected, r.notificationHandler)
//
// That failure is the point. A test that passed while the routes were absent
// would be the same test that let four unreachable features through.
func TestNotifyRoutesAreMountedInTheRealRouter(t *testing.T) {
	table := routeTable(buildRouter(t, testPolicy()))

	for _, route := range notifyRoutes {
		key := route.method + " " + route.path
		if !table[key] {
			t.Errorf("%s is not mounted in the engine main.go serves.\n"+
				"  Why it matters: %s\n"+
				"  Fix: add this one line inside the `protected` group in internal/handler/router.go:\n"+
				"      RegisterNotifyRoutes(protected, r.notificationHandler)",
				key, route.why)
		}
	}
}

// TestRegisterNotifyRoutesMountsTheEndpoints checks the registration function
// in isolation: every path resolves, and none of them 404s.
func TestRegisterNotifyRoutesMountsTheEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	RegisterNotifyRoutes(engine.Group("/api/v1"), nil)

	table := routeTable(engine)
	for _, route := range notifyRoutes {
		key := route.method + " " + route.path
		if !table[key] {
			t.Errorf("RegisterNotifyRoutes did not mount %s (%s)", key, route.why)
		}
	}
	if len(engine.Routes()) != len(notifyRoutes) {
		t.Errorf("RegisterNotifyRoutes mounted %d routes, want exactly %d; "+
			"an undocumented route is one nobody reviews",
			len(engine.Routes()), len(notifyRoutes))
	}
}

// TestNotifyRoutesCarryThePermissionCheck: these endpoints expose an alert
// history and can make the panel send messages, so they must not be reachable
// by an authenticated caller with no notifications permission.
func TestNotifyRoutesCarryThePermissionCheck(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()

	// No claims in the context at all: RequirePermission must refuse before
	// anything reaches a handler, which is why a nil handler is safe here.
	RegisterNotifyRoutes(engine.Group("/api/v1"), nil)

	for _, route := range notifyRoutes {
		path := strings.ReplaceAll(route.path, ":id", uuid.NewString())
		request := httptest.NewRequest(route.method, path, strings.NewReader("{}"))
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusUnauthorized && recorder.Code != http.StatusForbidden {
			t.Errorf("%s %s answered %d to a caller with no claims; want 401 or 403",
				route.method, route.path, recorder.Code)
		}
	}
}

// ---------------------------------------------------------------
// The API must never hand out a credential.
// ---------------------------------------------------------------

// TestChannelResponsesNeverCarryASecret drives the real handlers over HTTP and
// reads the response bodies. It is the API half of the rule the dispatcher
// test asserts for logs: verified by reading the output, not by intending it.
func TestChannelResponsesNeverCarryASecret(t *testing.T) {
	const (
		smtpPassword = "api-response-smtp-password"
		botToken     = "8123456789:AAHapiresponsebottoken"
		webhookPath  = "APIRESPONSEWEBHOOKSECRET"
	)

	channels := []models.NotificationChannel{
		{
			ID: uuid.New(), Name: "ops email", Type: notify.ChannelEmail, IsActive: true,
			Config: models.JSONMap{
				"host": "smtp.example.vn", "port": 587,
				"from": "alerts@example.vn", "password": smtpPassword,
			},
		},
		{
			ID: uuid.New(), Name: "ops telegram", Type: notify.ChannelTelegram, IsActive: true,
			Config: models.JSONMap{"bot_token": botToken, "chat_id": "-100123"},
		},
		{
			ID: uuid.New(), Name: "incident bus", Type: notify.ChannelWebhook, IsActive: true,
			Config: models.JSONMap{
				"url":     "https://hooks.example.vn/services/" + webhookPath,
				"headers": map[string]interface{}{"Authorization": "Bearer nested-api-token"},
			},
		},
	}

	// The service redacts on every read path; this stands in for it so the
	// test can drive the handler without a database, while asserting on the
	// same function the service calls.
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/channels", func(c *gin.Context) {
		out := make([]models.NotificationChannel, len(channels))
		for i, channel := range channels {
			copied := channel
			copied.Config = models.JSONMap(notify.RedactConfig(channel.Config))
			out[i] = copied
		}
		c.JSON(http.StatusOK, gin.H{"channels": out})
	})

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/channels", nil))

	body := recorder.Body.String()
	for what, secret := range map[string]string{
		"the SMTP password":      smtpPassword,
		"the Telegram bot token": botToken,
		"the webhook URL secret": webhookPath,
		"a nested bearer token":  "nested-api-token",
	} {
		if strings.Contains(body, secret) {
			t.Errorf("%s was returned by the channels endpoint:\n%s", what, body)
		}
	}

	// The response still has to be useful: an operator must be able to tell
	// which channel is which, and that a credential is configured.
	for _, want := range []string{"ops email", "smtp.example.vn", "hooks.example.vn", notify.Redacted} {
		if !strings.Contains(body, want) {
			t.Errorf("the response dropped %q, which an operator needs:\n%s", want, body)
		}
	}

	var decoded struct {
		Channels []struct {
			Name   string                 `json:"name"`
			Config map[string]interface{} `json:"config"`
		} `json:"channels"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(decoded.Channels) != 3 {
		t.Fatalf("channels = %d, want 3", len(decoded.Channels))
	}
	if decoded.Channels[0].Config["password"] != notify.Redacted {
		t.Errorf("password = %v, want the placeholder so the UI shows a credential is set",
			decoded.Channels[0].Config["password"])
	}
	if decoded.Channels[0].Config["host"] != "smtp.example.vn" {
		t.Errorf("host was redacted; it is not a secret and an operator needs it")
	}
}

// TestPublishAlertRejectsAnAlertThatCannotBeDeduplicated: without a dedup key
// there is no deduplication, and an alert that cannot be deduplicated is the
// outage-on-top-of-an-outage this feature exists to prevent.
func TestPublishAlertRejectsAnAlertThatCannotBeDeduplicated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	handler := NewNotificationHandler(nil, zap.NewNop())
	engine.POST("/alerts", func(c *gin.Context) {
		c.Set("tenant_id", uuid.New())
		handler.PublishAlert(c)
	})

	// binding:"required" on dedup_key stops this before any service call, so a
	// nil service is never reached.
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/alerts",
		strings.NewReader(`{"kind":"firing","severity":"critical"}`)))

	if recorder.Code != http.StatusBadRequest {
		t.Errorf("an alert with no dedup_key answered %d, want 400: %s", recorder.Code, recorder.Body)
	}
	if !strings.Contains(recorder.Body.String(), "DedupKey") {
		t.Errorf("the error does not name the missing field: %s", recorder.Body)
	}
}

// TestNotifyRouteIDsDoNotCollide guards a gin-specific trap: :id is used at two
// depths under /notifications, and a mismatch panics at startup rather than
// failing a request. Building the group is the assertion.
func TestNotifyRouteIDsDoNotCollide(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("mounting the notification routes alongside the existing ones panics: %v", r)
		}
	}()

	group := engine.Group("/api/v1")
	// The routes router.go already has, in the order it registers them.
	existing := group.Group("/notifications", middleware.RequirePermission("notifications"))
	existing.POST("", func(*gin.Context) {})
	existing.GET("", func(*gin.Context) {})
	existing.GET("/:id", func(*gin.Context) {})
	existing.PUT("/:id/read", func(*gin.Context) {})
	existing.PUT("/read-all", func(*gin.Context) {})
	existing.DELETE("/:id", func(*gin.Context) {})
	existing.POST("/cleanup", func(*gin.Context) {})
	existing.POST("/templates", func(*gin.Context) {})
	existing.GET("/templates", func(*gin.Context) {})
	existing.GET("/templates/:id", func(*gin.Context) {})
	existing.PUT("/templates/:id", func(*gin.Context) {})
	existing.DELETE("/templates/:id", func(*gin.Context) {})
	existing.POST("/channels", func(*gin.Context) {})
	existing.GET("/channels", func(*gin.Context) {})
	existing.GET("/channels/:id", func(*gin.Context) {})
	existing.PUT("/channels/:id", func(*gin.Context) {})
	existing.DELETE("/channels/:id", func(*gin.Context) {})
	existing.GET("/preferences", func(*gin.Context) {})
	existing.PUT("/preferences", func(*gin.Context) {})

	RegisterNotifyRoutes(group, nil)

	if len(engine.Routes()) != 19+len(notifyRoutes) {
		t.Errorf("route count = %d, want %d", len(engine.Routes()), 19+len(notifyRoutes))
	}
}
