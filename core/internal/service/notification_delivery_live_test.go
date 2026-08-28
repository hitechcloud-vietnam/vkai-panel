// The alert path, driven end to end against a real PostgreSQL and a real HTTP
// receiver. Skipped unless VKAI_NOTIFY_DSN names a database with the numbered
// migrations plus migrations/pending/notify.sql applied.
//
//	VKAI_NOTIFY_DSN="postgres://postgres@127.0.0.1:5432/vkai_notify_check?sslmode=disable" \
//	  go test ./internal/service/ -run TestLiveNotify -v
//
// The unit tests in internal/notify prove each piece. This file proves they
// are connected: a caller raises an alert, a row lands in the outbox, the
// dispatcher picks it up, and a message arrives at a receiver that records
// what it got. Nothing here is mocked except the far end of the network.

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/notify"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
)

// receiver is an HTTP endpoint that records every webhook it is sent, and can
// be told to fail.
type receiver struct {
	mu       sync.Mutex
	payloads []map[string]interface{}
	status   int
	server   *httptest.Server
}

func newReceiver(t *testing.T) *receiver {
	t.Helper()
	r := &receiver{status: http.StatusOK}
	r.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		var payload map[string]interface{}
		_ = json.Unmarshal(body, &payload)

		r.mu.Lock()
		status := r.status
		if status == http.StatusOK {
			r.payloads = append(r.payloads, payload)
		}
		r.mu.Unlock()

		w.WriteHeader(status)
		if status != http.StatusOK {
			_, _ = w.Write([]byte("the receiver is having a bad day"))
		}
	}))
	t.Cleanup(r.server.Close)
	return r
}

func (r *receiver) received() []map[string]interface{} {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]map[string]interface{}, len(r.payloads))
	copy(out, r.payloads)
	return out
}

func (r *receiver) setStatus(status int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status = status
}

// liveNotifyService builds the real repository, the real service and the real
// dispatcher over the test database.
func liveNotifyService(t *testing.T, logger *zap.Logger) (*NotificationService, *sqlx.DB, uuid.UUID) {
	t.Helper()
	dsn := os.Getenv("VKAI_NOTIFY_DSN")
	if dsn == "" {
		t.Skip("no VKAI_NOTIFY_DSN")
	}
	db, err := sqlx.Connect("pgx", dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var tenantID uuid.UUID
	if err := db.Get(&tenantID, "SELECT id FROM tenants ORDER BY created_at LIMIT 1"); err != nil {
		t.Fatalf("no tenant in the test database: %v", err)
	}

	repo := repository.NewNotificationRepository(db)
	if err := repo.EnsureDeliverySchema(context.Background()); err != nil {
		t.Fatalf("%v", err)
	}

	service := NewNotificationService(repo, logger)
	service.AttachDelivery(notify.NewRegistry(notify.Deps{HTTPClient: &http.Client{Timeout: 5 * time.Second}}),
		DeliveryOptions{
			PanelBaseURL: "https://panel.example.vn",
			QuietPeriod:  time.Hour,
			MaxAttempts:  3,
			BaseBackoff:  time.Millisecond,
			MaxBackoff:   5 * time.Millisecond,
			SendTimeout:  5 * time.Second,
		})
	return service, db, tenantID
}

// makeWebhookChannel creates an active webhook channel pointing at a receiver.
func makeWebhookChannel(t *testing.T, service *NotificationService, db *sqlx.DB, tenantID uuid.UUID, url, name string) uuid.UUID {
	t.Helper()
	channel, err := service.CreateChannel(context.Background(), tenantID, &models.CreateNotificationChannelRequest{
		Name: name,
		Type: notify.ChannelWebhook,
		Config: models.JSONMap{
			"url":    url,
			"secret": "live-webhook-shared-secret",
		},
	})
	if err != nil {
		t.Fatalf("create channel: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM notification_deliveries WHERE channel_id = $1", channel.ID)
		_, _ = db.Exec("DELETE FROM notification_channels WHERE id = $1", channel.ID)
	})
	return channel.ID
}

// deactivateOtherChannels stops a channel left behind by another test, or
// configured in a shared database, from receiving this test's alerts.
func deactivateOtherChannels(t *testing.T, db *sqlx.DB, tenantID, keep uuid.UUID) {
	t.Helper()
	if _, err := db.Exec(
		"UPDATE notification_channels SET is_active = FALSE WHERE tenant_id = $1 AND id <> $2",
		tenantID, keep); err != nil {
		t.Fatalf("deactivate other channels: %v", err)
	}
}

// TestLiveNotifyEndToEnd is the whole path: a disk fills up, a human is told,
// it is told once, and it is told again when it clears.
func TestLiveNotifyEndToEnd(t *testing.T) {
	service, db, tenantID := liveNotifyService(t, zap.NewNop())
	endpoint := newReceiver(t)
	channelID := makeWebhookChannel(t, service, db, tenantID, endpoint.server.URL, "live e2e "+uuid.NewString()[:8])
	deactivateOtherChannels(t, db, tenantID, channelID)

	ctx := context.Background()
	dedupKey := "live-e2e:" + uuid.NewString()
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM notification_alert_state WHERE dedup_key = $1", dedupKey)
	})

	alert := notify.Alert{
		DedupKey: dedupKey, Kind: notify.KindFiring, Severity: notify.SeverityCritical,
		ServerID: "3f2b9c14-0000-4000-8000-00000000abcd", ServerName: "web-01.hcm.example.vn",
		Resource: "disk /var", Metric: "disk_used_percent",
		Value: 92.5, Threshold: 90, Condition: "gt", Unit: "%",
	}

	// 1. The disk crosses its threshold.
	result, err := service.Notify(ctx, tenantID, alert)
	if err != nil {
		t.Fatalf("notify: %v", err)
	}
	if !result.Notified {
		t.Fatalf("the first firing was suppressed: %+v", result)
	}
	if result.Channels != 1 {
		t.Fatalf("fanned out to %d channels, want 1: %+v", result.Channels, result)
	}

	// Nothing has been sent yet: Notify is off the request path.
	if got := len(endpoint.received()); got != 0 {
		t.Fatalf("%d messages were sent from the request path; delivery must be in the background", got)
	}

	// 2. The dispatcher runs.
	if _, err := service.DispatchOnce(ctx); err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	received := endpoint.received()
	if len(received) != 1 {
		t.Fatalf("the receiver got %d messages, want 1", len(received))
	}

	// 3. What arrived has to be actionable.
	payload := received[0]
	subject, _ := payload["subject"].(string)
	body, _ := payload["body"].(string)
	link, _ := payload["link"].(string)
	text := subject + "\n" + body

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
	if link != "https://panel.example.vn/monitoring/servers/3f2b9c14-0000-4000-8000-00000000abcd?metric=disk_used_percent" {
		t.Errorf("the link does not go to the relevant panel page: %q", link)
	}
	if payload["event"] != string(notify.KindFiring) {
		t.Errorf("event = %v, want firing", payload["event"])
	}

	// 4. The row is marked sent.
	deliveries, _, err := service.ListDeliveries(ctx, tenantID, notify.DeliveryFilter{DedupKey: dedupKey})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(deliveries) != 1 || deliveries[0].Status != notify.StatusSent {
		t.Fatalf("the outbox row reads %+v, want one row marked sent", deliveries)
	}

	// 5. Six hours of five-minute checks produce six more messages, not
	//    seventy-two: one per elapsed quiet period.
	for minute := 5; minute <= 360; minute += 5 {
		repeat := alert
		repeat.OccurredAt = time.Now().Add(time.Duration(minute) * time.Minute)
		if _, err := service.Notify(ctx, tenantID, repeat); err != nil {
			t.Fatalf("notify repeat: %v", err)
		}
	}
	if _, err := service.DispatchOnce(ctx); err != nil {
		t.Fatalf("dispatch repeats: %v", err)
	}
	if got := len(endpoint.received()); got != 7 {
		t.Errorf("six hours of five-minute checks delivered %d messages, want 7", got)
	}

	// 6. It clears: exactly one resolution message.
	resolved := alert
	resolved.Kind = notify.KindResolved
	resolved.Value = 41
	resolved.OccurredAt = time.Now().Add(370 * time.Minute)
	result, err = service.Notify(ctx, tenantID, resolved)
	if err != nil {
		t.Fatalf("notify resolved: %v", err)
	}
	if !result.Notified || result.Reason != notify.ReasonResolved {
		t.Fatalf("the alert cleared but no resolution was queued: %+v", result)
	}
	if _, err := service.DispatchOnce(ctx); err != nil {
		t.Fatalf("dispatch resolved: %v", err)
	}

	all := endpoint.received()
	if len(all) != 8 {
		t.Fatalf("total messages = %d, want 8 (7 firing + 1 resolved)", len(all))
	}
	last := all[len(all)-1]
	if last["event"] != string(notify.KindResolved) {
		t.Errorf("the last message is %v, want a resolution", last["event"])
	}
	if lastSubject, _ := last["subject"].(string); !strings.Contains(lastSubject, "RESOLVED") {
		t.Errorf("the resolution is not recognisable from its subject: %q", lastSubject)
	}

	// A second resolve sends nothing.
	again := resolved
	again.OccurredAt = time.Now().Add(375 * time.Minute)
	result, err = service.Notify(ctx, tenantID, again)
	if err != nil {
		t.Fatalf("notify second resolve: %v", err)
	}
	if result.Notified {
		t.Errorf("a second resolve queued another message: %+v", result)
	}
	if _, err := service.DispatchOnce(ctx); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if got := len(endpoint.received()); got != 8 {
		t.Errorf("a second resolve delivered a message: total = %d, want 8", got)
	}
}

// TestLiveNotifyDeadLetterReachesThePanelInbox: an alert that reaches nobody
// has to end up somewhere a human looks, or the panel is back to dropping
// alerts silently.
func TestLiveNotifyDeadLetterReachesThePanelInbox(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	service, db, tenantID := liveNotifyService(t, zap.New(core))

	endpoint := newReceiver(t)
	endpoint.setStatus(http.StatusServiceUnavailable) // retryable, so the budget is spent
	channelID := makeWebhookChannel(t, service, db, tenantID, endpoint.server.URL, "live dead "+uuid.NewString()[:8])
	deactivateOtherChannels(t, db, tenantID, channelID)

	ctx := context.Background()
	dedupKey := "live-dead:" + uuid.NewString()
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM notification_alert_state WHERE dedup_key = $1", dedupKey)
		_, _ = db.Exec("DELETE FROM notifications WHERE type = 'notification_delivery_failed' AND details->>'dedup_key' = $1", dedupKey)
	})

	if _, err := service.Notify(ctx, tenantID, notify.Alert{
		DedupKey: dedupKey, Kind: notify.KindFiring, Severity: notify.SeverityCritical,
		ServerName: "web-01", Resource: "disk /var", Metric: "disk_used_percent",
		Value: 92.5, Threshold: 90, Condition: "gt", Unit: "%",
	}); err != nil {
		t.Fatalf("notify: %v", err)
	}

	// Three attempts, with the backoff elapsing between them.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := service.DispatchOnce(ctx); err != nil {
			t.Fatalf("dispatch: %v", err)
		}
		dead, total, err := service.ListDeliveries(ctx, tenantID, notify.DeliveryFilter{
			DedupKey: dedupKey, Status: notify.StatusDeadLetter,
		})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if total == 1 {
			if !strings.Contains(dead[0].LastError, "gave up after 3 attempts") {
				t.Errorf("the dead letter does not say why: %q", dead[0].LastError)
			}
			if !strings.Contains(dead[0].LastError, "503") {
				t.Errorf("the dead letter dropped the receiver's status: %q", dead[0].LastError)
			}
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	_, total, _ := service.ListDeliveries(ctx, tenantID, notify.DeliveryFilter{
		DedupKey: dedupKey, Status: notify.StatusDeadLetter,
	})
	if total != 1 {
		t.Fatalf("dead letters = %d, want 1; the delivery neither succeeded nor was given up on", total)
	}

	// The in-panel inbox row is the part a human actually sees.
	var inboxRows int
	if err := db.Get(&inboxRows, `
		SELECT COUNT(*) FROM notifications
		WHERE tenant_id = $1 AND type = 'notification_delivery_failed'
		  AND details->>'dedup_key' = $2`, tenantID, dedupKey); err != nil {
		t.Fatalf("count inbox rows: %v", err)
	}
	if inboxRows != 1 {
		t.Errorf("the panel inbox has %d rows about this failure, want 1; "+
			"an alert that reached nobody is invisible", inboxRows)
	}

	// And it was logged at error level, not buried at debug.
	var loudEnough bool
	for _, entry := range logs.All() {
		if strings.Contains(entry.Message, "dead-lettered") && entry.Level >= zap.ErrorLevel {
			loudEnough = true
		}
	}
	if !loudEnough {
		t.Errorf("the dead letter was not logged at error level")
	}

	// The receiver recovers, and a retry gets through - which is the point of
	// keeping the row rather than deleting it.
	endpoint.setStatus(http.StatusOK)
	dead, _, _ := service.ListDeliveries(ctx, tenantID, notify.DeliveryFilter{
		DedupKey: dedupKey, Status: notify.StatusDeadLetter,
	})
	if err := service.RetryDelivery(ctx, tenantID, dead[0].ID); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if _, err := service.DispatchOnce(ctx); err != nil {
		t.Fatalf("dispatch after retry: %v", err)
	}
	if got := len(endpoint.received()); got != 1 {
		t.Errorf("the retried delivery arrived %d times, want 1", got)
	}
}

// TestLiveNotifyTestSendReturnsTheRealError: an operator pressing "test" needs
// the failure, not a delivery id.
func TestLiveNotifyTestSendReturnsTheRealError(t *testing.T) {
	service, db, tenantID := liveNotifyService(t, zap.NewNop())

	endpoint := newReceiver(t)
	channelID := makeWebhookChannel(t, service, db, tenantID, endpoint.server.URL, "live test "+uuid.NewString()[:8])
	ctx := context.Background()

	// A working channel says so, and the message actually arrives.
	if err := service.TestChannel(ctx, tenantID, channelID, "web-01"); err != nil {
		t.Fatalf("testing a working channel failed: %v", err)
	}
	received := endpoint.received()
	if len(received) != 1 {
		t.Fatalf("the test send delivered %d messages, want 1", len(received))
	}
	if received[0]["event"] != string(notify.KindTest) {
		t.Errorf("event = %v, want test", received[0]["event"])
	}

	// A test is never deduplicated: pressing it twice sends twice.
	if err := service.TestChannel(ctx, tenantID, channelID, "web-01"); err != nil {
		t.Fatalf("second test: %v", err)
	}
	if got := len(endpoint.received()); got != 2 {
		t.Errorf("a second test send delivered %d messages in total, want 2", got)
	}

	// A broken channel returns the receiver's own words.
	endpoint.setStatus(http.StatusUnauthorized)
	err := service.TestChannel(ctx, tenantID, channelID, "web-01")
	if err == nil {
		t.Fatalf("testing a channel that answers 401 reported success")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("the error does not carry the status an operator needs: %v", err)
	}
	if strings.Contains(err.Error(), "live-webhook-shared-secret") {
		t.Errorf("the channel's shared secret leaked into the test-send error: %v", err)
	}

	// No outbox row: a test send is synchronous by design.
	_, total, err := service.ListDeliveries(ctx, tenantID, notify.DeliveryFilter{ChannelID: &channelID})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 0 {
		t.Errorf("a test send queued %d outbox rows, want 0", total)
	}
}

// TestLiveNotifyChannelReadPathsNeverReturnASecret is the API-boundary rule
// checked against the real column rather than a literal.
func TestLiveNotifyChannelReadPathsNeverReturnASecret(t *testing.T) {
	service, db, tenantID := liveNotifyService(t, zap.NewNop())
	endpoint := newReceiver(t)
	channelID := makeWebhookChannel(t, service, db, tenantID,
		endpoint.server.URL+"/services/LIVESECRETPATH", "live secret "+uuid.NewString()[:8])
	ctx := context.Background()

	check := func(what string, config models.JSONMap) {
		t.Helper()
		encoded, err := json.Marshal(config)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		for _, secret := range []string{"live-webhook-shared-secret", "LIVESECRETPATH"} {
			if strings.Contains(string(encoded), secret) {
				t.Errorf("%s returned a credential: %s", what, encoded)
			}
		}
	}

	one, err := service.GetChannelByID(ctx, tenantID, channelID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	check("GetChannelByID", one.Config)

	all, err := service.ListChannels(ctx, tenantID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, channel := range all {
		check("ListChannels", channel.Config)
	}

	// The database still holds the real values, or the sender could not work.
	var stored string
	if err := db.Get(&stored, "SELECT config->>'secret' FROM notification_channels WHERE id = $1", channelID); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if stored != "live-webhook-shared-secret" {
		t.Errorf("redaction reached the stored column: %q", stored)
	}
}

// TestLiveNotifyDispatcherRefusesToStartWithoutTheMigration is the startup
// behaviour on an installation that has not applied
// migrations/pending/notify.sql - which, until that file is renumbered into
// migrations/, is every customer installation, because deploy/install.sh does
// not descend into pending/.
//
// The dispatcher must return immediately with one clear log line naming the
// file. The alternative - a loop that starts and fails on every poll - buries
// the cause under thousands of identical errors, and does it at 3am.
//
// Needs VKAI_NOTIFY_DSN_NOMIGRATION: a database with the numbered migrations
// applied and the pending one deliberately not.
func TestLiveNotifyDispatcherRefusesToStartWithoutTheMigration(t *testing.T) {
	dsn := os.Getenv("VKAI_NOTIFY_DSN_NOMIGRATION")
	if dsn == "" {
		t.Skip("no VKAI_NOTIFY_DSN_NOMIGRATION")
	}
	db, err := sqlx.Connect("pgx", dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = db.Close() }()

	core, logs := observer.New(zap.DebugLevel)
	service := NewNotificationService(repository.NewNotificationRepository(db), zap.New(core))
	service.AttachDelivery(notify.NewRegistry(notify.Deps{}), DeliveryOptions{
		Interval: time.Millisecond,
	})

	// If RunDispatcher entered its loop this would block until the timeout.
	done := make(chan struct{})
	go func() {
		service.RunDispatcher(context.Background())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("RunDispatcher entered its polling loop on a database with no delivery tables; " +
			"it would log the same failure forever instead of naming the cause once")
	}

	var named bool
	for _, entry := range logs.All() {
		if entry.Level >= zap.ErrorLevel && strings.Contains(entry.Message, "not started") {
			for _, field := range entry.Context {
				if field.Key == "error" && strings.Contains(fmt.Sprint(field.Interface), "migrations/pending/notify.sql") {
					named = true
				}
			}
		}
	}
	if !named {
		t.Errorf("the dispatcher did not log one error naming migrations/pending/notify.sql; " +
			"an operator has no way to learn why alerts stopped")
	}
}
