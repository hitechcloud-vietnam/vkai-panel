// The alert path from an HTTP request to a message on the wire, with nothing
// faked but the far end of the network.
//
// Skipped unless VKAI_NOTIFY_DSN names a database with the numbered migrations
// plus migrations/pending/notify.sql applied:
//
//	VKAI_NOTIFY_DSN="postgres://postgres@127.0.0.1:5432/vkai_notify_check?sslmode=disable" \
//	  go test ./internal/handler/ -run TestLiveNotify -v
//
// What this covers that nothing else does: the request arrives at a route
// mounted by RegisterNotifyRoutes, passes through the permission middleware,
// reaches the real handler, the real service and the real repository, and is
// delivered by the dispatcher goroutine that main.go starts - not by a
// DispatchOnce a test called by hand. Every other test in this feature stubs
// at least one of those joins.

package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/auth"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/notify"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/service"
)

// liveReceiver records the webhooks it is sent.
type liveReceiver struct {
	mu       sync.Mutex
	payloads []map[string]interface{}
	server   *httptest.Server
}

func newLiveReceiver(t *testing.T) *liveReceiver {
	t.Helper()
	r := &liveReceiver{}
	r.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		var payload map[string]interface{}
		_ = json.Unmarshal(body, &payload)
		r.mu.Lock()
		r.payloads = append(r.payloads, payload)
		r.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(r.server.Close)
	return r
}

func (r *liveReceiver) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.payloads)
}

func (r *liveReceiver) first() map[string]interface{} {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.payloads) == 0 {
		return nil
	}
	return r.payloads[0]
}

// TestLiveNotifyAlertReachesAReceiverOverHTTP drives the whole path.
func TestLiveNotifyAlertReachesAReceiverOverHTTP(t *testing.T) {
	dsn := os.Getenv("VKAI_NOTIFY_DSN")
	if dsn == "" {
		t.Skip("no VKAI_NOTIFY_DSN")
	}
	db, err := sqlx.Connect("pgx", dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = db.Close() }()

	var tenantID uuid.UUID
	if err := db.Get(&tenantID, "SELECT id FROM tenants ORDER BY created_at LIMIT 1"); err != nil {
		t.Fatalf("no tenant in the test database: %v", err)
	}

	// The stack cmd/api/main.go builds, built the same way.
	logger := zap.NewNop()
	repo := repository.NewNotificationRepository(db)
	notificationService := service.NewNotificationService(repo, logger)
	notificationService.AttachDelivery(
		notify.NewRegistry(notify.Deps{HTTPClient: &http.Client{Timeout: 5 * time.Second}}),
		service.DeliveryOptions{
			PanelBaseURL: "https://panel.example.vn",
			QuietPeriod:  time.Hour,
			MaxAttempts:  3,
			Interval:     20 * time.Millisecond,
			BaseBackoff:  time.Millisecond,
			MaxBackoff:   5 * time.Millisecond,
		})
	notificationHandler := NewNotificationHandler(notificationService, logger)

	// The dispatcher goroutine main.go starts. Nothing in this test calls
	// DispatchOnce: if the loop does not run, the alert never arrives.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go notificationService.RunDispatcher(ctx)

	endpoint := newLiveReceiver(t)
	channel, err := notificationService.CreateChannel(ctx, tenantID, &models.CreateNotificationChannelRequest{
		Name:   "live http " + uuid.NewString()[:8],
		Type:   notify.ChannelWebhook,
		Config: models.JSONMap{"url": endpoint.server.URL},
	})
	if err != nil {
		t.Fatalf("create channel: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM notification_deliveries WHERE channel_id = $1", channel.ID)
		_, _ = db.Exec("DELETE FROM notification_channels WHERE id = $1", channel.ID)
	})
	if _, err := db.Exec(
		"UPDATE notification_channels SET is_active = FALSE WHERE tenant_id = $1 AND id <> $2",
		tenantID, channel.ID); err != nil {
		t.Fatalf("deactivate other channels: %v", err)
	}

	// The engine, with the routes mounted the way router.go mounts them.
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	protected := engine.Group("/api/v1")
	protected.Use(func(c *gin.Context) {
		// Stands in for middleware.AuthRequired: the same three values it puts
		// in the context, so RequirePermission runs for real.
		c.Set("claims", &auth.TokenClaims{
			UserID: uuid.New(), TenantID: tenantID,
			// middleware.HasPermission joins with a dot and maps GET to
			// "read" and every mutating method to "write".
			Permissions: []string{"notifications.read", "notifications.write"},
		})
		c.Set("tenant_id", tenantID)
		c.Set("user_id", uuid.New())
		c.Next()
	})
	RegisterNotifyRoutes(protected, notificationHandler)

	dedupKey := "live-http:" + uuid.NewString()
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM notification_alert_state WHERE dedup_key = $1", dedupKey)
	})

	body := `{
		"dedup_key":   "` + dedupKey + `",
		"kind":        "firing",
		"severity":    "critical",
		"server_id":   "3f2b9c14-0000-4000-8000-00000000abcd",
		"server_name": "web-01.hcm.example.vn",
		"resource":    "disk /var",
		"metric":      "disk_used_percent",
		"value":       92.5,
		"threshold":   90,
		"condition":   "gt",
		"unit":        "%"
	}`

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/alerts", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("POST /notifications/alerts answered %d, want 202: %s", recorder.Code, recorder.Body)
	}
	var accepted struct {
		Notified    bool     `json:"notified"`
		Reason      string   `json:"reason"`
		Channels    int      `json:"channels"`
		DeliveryIDs []string `json:"delivery_ids"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &accepted); err != nil {
		t.Fatalf("decode response: %v: %s", err, recorder.Body)
	}
	if !accepted.Notified || accepted.Channels != 1 || len(accepted.DeliveryIDs) != 1 {
		t.Fatalf("the response says nothing was queued: %+v", accepted)
	}

	// Wait for the background loop. This is the assertion: if RunDispatcher
	// is not running, or the outbox row is not claimable, nothing arrives.
	deadline := time.Now().Add(10 * time.Second)
	for endpoint.count() == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if endpoint.count() == 0 {
		t.Fatalf("no message arrived within 10s; the alert was accepted over HTTP and never delivered")
	}

	payload := endpoint.first()
	subject, _ := payload["subject"].(string)
	messageBody, _ := payload["body"].(string)
	link, _ := payload["link"].(string)
	text := subject + "\n" + messageBody

	for what, want := range map[string]string{
		"the server":    "web-01.hcm.example.vn",
		"the resource":  "disk /var",
		"the value":     "92.5%",
		"the threshold": "90%",
		"the severity":  "CRITICAL",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the delivered message does not carry %s (%q):\n%s", what, want, text)
		}
	}
	if !strings.HasPrefix(link, "https://panel.example.vn/monitoring/servers/") {
		t.Errorf("the delivered message has no link to the relevant panel page: %q", link)
	}

	// The test-send endpoint, over the same mounted route.
	testRecorder := httptest.NewRecorder()
	engine.ServeHTTP(testRecorder, httptest.NewRequest(http.MethodPost,
		"/api/v1/notifications/channels/"+channel.ID.String()+"/test", nil))
	if testRecorder.Code != http.StatusOK {
		t.Errorf("the test-send endpoint answered %d, want 200: %s", testRecorder.Code, testRecorder.Body)
	}

	// And the outbox listing, which is where an operator looks afterwards.
	listRecorder := httptest.NewRecorder()
	engine.ServeHTTP(listRecorder, httptest.NewRequest(http.MethodGet,
		"/api/v1/notifications/deliveries?dedup_key="+dedupKey, nil))
	if listRecorder.Code != http.StatusOK {
		t.Fatalf("the deliveries endpoint answered %d: %s", listRecorder.Code, listRecorder.Body)
	}
	if !strings.Contains(listRecorder.Body.String(), string(notify.StatusSent)) {
		t.Errorf("the outbox listing does not show the delivery as sent: %s", listRecorder.Body)
	}
}
