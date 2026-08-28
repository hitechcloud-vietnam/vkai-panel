// The tests in this file talk to a real PostgreSQL. They are skipped unless
// VKAI_NOTIFY_DSN names one, because the module's suite must stay runnable
// with no database - but they are how the SQL in notification.go is actually
// verified. A fake cannot check that FOR UPDATE SKIP LOCKED hands two
// concurrent workers disjoint rows, that a CHECK constraint rejects a status
// the code never intends to write, or that an ON CONFLICT upsert leaves one
// row rather than two.
//
//	initdb / pg_ctl start on a throwaway cluster, then:
//	  createdb vkai_notify_check
//	  psql -d vkai_notify_check -c 'CREATE EXTENSION IF NOT EXISTS "uuid-ossp"'
//	  for f in migrations/0*.sql; do psql -v ON_ERROR_STOP=1 -d vkai_notify_check -f $f; done
//	  psql -v ON_ERROR_STOP=1 -d vkai_notify_check -f migrations/pending/notify.sql
//	  VKAI_NOTIFY_DSN="postgres://postgres@127.0.0.1:5432/vkai_notify_check?sslmode=disable" \
//	    go test ./internal/repository/ -run TestLiveNotify -v

package repository

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/notify"
)

// liveNotifyRepo opens the test database and returns a repository plus the
// tenant every row will hang off.
func liveNotifyRepo(t *testing.T) (*NotificationRepository, *sqlx.DB, uuid.UUID) {
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
	return NewNotificationRepository(db), db, tenantID
}

// makeChannel creates a channel with a secret in its config and returns its id.
func makeChannel(t *testing.T, repo *NotificationRepository, tenantID uuid.UUID, name string) uuid.UUID {
	t.Helper()
	channel := &models.NotificationChannel{
		TenantID: tenantID,
		Name:     name,
		Type:     notify.ChannelEmail,
		Config: models.JSONMap{
			"host":     "smtp.example.vn",
			"from":     "alerts@example.vn",
			"to":       "ops@example.vn",
			"password": "live-test-smtp-password",
		},
		IsActive: true,
	}
	if err := repo.CreateChannel(context.Background(), channel); err != nil {
		t.Fatalf("create channel: %v", err)
	}
	t.Cleanup(func() {
		_ = repo.DeleteChannel(context.Background(), tenantID, channel.ID)
	})
	return channel.ID
}

// TestLiveNotifySchemaIsPresent fails loudly if the pending migration was not
// applied, so every other failure in this file has one obvious cause.
func TestLiveNotifySchemaIsPresent(t *testing.T) {
	repo, _, _ := liveNotifyRepo(t)
	if err := repo.EnsureDeliverySchema(context.Background()); err != nil {
		t.Fatalf("EnsureDeliverySchema: %v", err)
	}
}

// TestLiveNotifyOutboxRoundTrip drives one delivery through every state the
// dispatcher can put it in, against the real SQL.
func TestLiveNotifyOutboxRoundTrip(t *testing.T) {
	repo, db, tenantID := liveNotifyRepo(t)
	ctx := context.Background()
	channelID := makeChannel(t, repo, tenantID, "live outbox "+uuid.NewString()[:8])

	alert := notify.Alert{
		DedupKey: "live:" + uuid.NewString(), Kind: notify.KindFiring,
		Severity: notify.SeverityCritical, ServerName: "web-01", Resource: "disk /var",
		Metric: "disk_used_percent", Value: 92.5, Threshold: 90, Condition: "gt", Unit: "%",
		OccurredAt: time.Now().UTC().Truncate(time.Second),
	}

	id, err := repo.EnqueueDelivery(ctx, notify.EnqueueRequest{
		TenantID: tenantID, ChannelID: channelID, DedupKey: alert.DedupKey,
		Kind: notify.KindFiring, Subject: "[CRITICAL] web-01: disk /var is 92.5%",
		Body: "the body", Alert: alert, MaxAttempts: 3,
	})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec("DELETE FROM notification_deliveries WHERE id = $1", id) })

	// Claim it. The channel's type and config must arrive with it, or the
	// dispatcher would need a second query per attempt.
	claimed, err := repo.ClaimDue(ctx, time.Now(), 10)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	var found *notify.Delivery
	for i := range claimed {
		if claimed[i].ID == id {
			found = &claimed[i]
		}
	}
	if found == nil {
		t.Fatalf("the enqueued delivery was not claimed (claimed %d rows)", len(claimed))
	}
	if found.ChannelType != notify.ChannelEmail {
		t.Errorf("channel type = %q, want %q", found.ChannelType, notify.ChannelEmail)
	}
	if found.ChannelConfig.Secret("password").Reveal() != "live-test-smtp-password" {
		t.Errorf("the channel config did not arrive with the delivery; the sender could not be built")
	}
	if !found.ChannelActive {
		t.Errorf("channel_active = false for an active channel")
	}
	if found.Alert.Value != 92.5 || found.Alert.Threshold != 90 {
		t.Errorf("the structured alert did not survive the JSONB round trip: %+v", found.Alert)
	}
	if found.MaxAttempts != 3 {
		t.Errorf("max_attempts = %d, want 3", found.MaxAttempts)
	}

	// A second claim must not return it: it is marked 'sending'.
	again, err := repo.ClaimDue(ctx, time.Now(), 10)
	if err != nil {
		t.Fatalf("second claim: %v", err)
	}
	for _, d := range again {
		if d.ID == id {
			t.Fatalf("a claimed delivery was handed out twice")
		}
	}

	// Reschedule, and check it comes back only when it is due.
	future := time.Now().Add(30 * time.Second)
	if err := repo.Reschedule(ctx, id, 1, future, "connection refused"); err != nil {
		t.Fatalf("reschedule: %v", err)
	}
	notYet, _ := repo.ClaimDue(ctx, time.Now(), 10)
	for _, d := range notYet {
		if d.ID == id {
			t.Fatalf("a rescheduled delivery was claimed before its backoff elapsed")
		}
	}
	dueNow, _ := repo.ClaimDue(ctx, future.Add(time.Second), 10)
	var reclaimed bool
	for _, d := range dueNow {
		if d.ID == id {
			reclaimed = true
			if d.Attempts != 1 {
				t.Errorf("attempts = %d after one reschedule, want 1", d.Attempts)
			}
		}
	}
	if !reclaimed {
		t.Fatalf("a rescheduled delivery was never claimed again")
	}

	// Dead-letter it, and check the row an operator would read.
	if err := repo.DeadLetter(ctx, id, time.Now(), "gave up after 3 attempts: connection refused"); err != nil {
		t.Fatalf("dead letter: %v", err)
	}
	record, err := repo.GetDelivery(ctx, tenantID, id)
	if err != nil {
		t.Fatalf("get delivery: %v", err)
	}
	if record.Status != notify.StatusDeadLetter {
		t.Errorf("status = %q, want %q", record.Status, notify.StatusDeadLetter)
	}
	if record.DeadLetteredAt == nil {
		t.Errorf("dead_lettered_at was not set")
	}
	if record.ChannelName == "" || record.ChannelType != notify.ChannelEmail {
		t.Errorf("the delivery record does not carry the channel: %+v", record)
	}

	// Retry puts it back with a fresh budget - the point of keeping the row.
	if err := repo.RetryDelivery(ctx, tenantID, id); err != nil {
		t.Fatalf("retry: %v", err)
	}
	record, _ = repo.GetDelivery(ctx, tenantID, id)
	if record.Status != notify.StatusPending || record.Attempts != 0 || record.DeadLetteredAt != nil {
		t.Errorf("after a retry the row reads %+v, want pending with no attempts", record)
	}

	// Only a dead letter can be retried.
	if err := repo.RetryDelivery(ctx, tenantID, id); err == nil {
		t.Errorf("a pending delivery was 'retried'")
	}
}

// TestLiveNotifyClaimIsExclusive is the one a fake cannot check: two workers
// polling the same table must not both send the same alert.
func TestLiveNotifyClaimIsExclusive(t *testing.T) {
	repo, db, tenantID := liveNotifyRepo(t)
	ctx := context.Background()
	channelID := makeChannel(t, repo, tenantID, "live exclusive "+uuid.NewString()[:8])

	const rows = 20
	dedupKey := "live-exclusive:" + uuid.NewString()
	for i := 0; i < rows; i++ {
		if _, err := repo.EnqueueDelivery(ctx, notify.EnqueueRequest{
			TenantID: tenantID, ChannelID: channelID, DedupKey: dedupKey,
			Kind: notify.KindFiring, Subject: "s", Body: "b",
		}); err != nil {
			t.Fatalf("enqueue %d: %v", i, err)
		}
	}
	t.Cleanup(func() { _, _ = db.Exec("DELETE FROM notification_deliveries WHERE dedup_key = $1", dedupKey) })

	// Six workers claiming at once.
	var (
		mu    sync.Mutex
		seen  = make(map[uuid.UUID]int)
		wg    sync.WaitGroup
		total int
	)
	now := time.Now()
	for worker := 0; worker < 6; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			claimed, err := repo.ClaimDue(ctx, now, rows)
			if err != nil {
				t.Errorf("claim: %v", err)
				return
			}
			mu.Lock()
			defer mu.Unlock()
			for _, d := range claimed {
				if d.DedupKey != dedupKey {
					continue
				}
				seen[d.ID]++
				total++
			}
		}()
	}
	wg.Wait()

	for id, count := range seen {
		if count > 1 {
			t.Errorf("delivery %s was claimed by %d workers; that alert would be sent %d times", id, count, count)
		}
	}
	if total != rows {
		t.Errorf("%d of %d rows were claimed in total; the rest were skipped rather than delivered", total, rows)
	}
}

// TestLiveNotifyReleaseStale proves the reaper: a row a dead worker left
// behind comes back, rather than being a silently dropped alert.
func TestLiveNotifyReleaseStale(t *testing.T) {
	repo, db, tenantID := liveNotifyRepo(t)
	ctx := context.Background()
	channelID := makeChannel(t, repo, tenantID, "live stale "+uuid.NewString()[:8])

	dedupKey := "live-stale:" + uuid.NewString()
	id, err := repo.EnqueueDelivery(ctx, notify.EnqueueRequest{
		TenantID: tenantID, ChannelID: channelID, DedupKey: dedupKey,
		Kind: notify.KindFiring, Subject: "s", Body: "b",
	})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec("DELETE FROM notification_deliveries WHERE id = $1", id) })

	if _, err := repo.ClaimDue(ctx, time.Now(), 50); err != nil {
		t.Fatalf("claim: %v", err)
	}
	// Backdate the lease the way a worker that died an hour ago would leave it.
	if _, err := db.Exec("UPDATE notification_deliveries SET updated_at = NOW() - INTERVAL '1 hour' WHERE id = $1", id); err != nil {
		t.Fatalf("backdate: %v", err)
	}

	released, err := repo.ReleaseStale(ctx, time.Now().Add(-5*time.Minute))
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if released < 1 {
		t.Fatalf("ReleaseStale returned %d, want at least the abandoned row", released)
	}

	claimed, _ := repo.ClaimDue(ctx, time.Now(), 50)
	var back bool
	for _, d := range claimed {
		if d.ID == id {
			back = true
		}
	}
	if !back {
		t.Errorf("the abandoned delivery was never claimed again; that alert is lost")
	}
}

// TestLiveNotifyAlertStateDeduplicates runs the real dedup path - lock, decide,
// upsert - against Postgres, including the six-hour disk case.
func TestLiveNotifyAlertStateDeduplicates(t *testing.T) {
	repo, db, tenantID := liveNotifyRepo(t)
	ctx := context.Background()

	dedupKey := "live-dedup:" + uuid.NewString()
	t.Cleanup(func() { _, _ = db.Exec("DELETE FROM notification_alert_state WHERE dedup_key = $1", dedupKey) })

	start := time.Now().UTC().Truncate(time.Second)
	observe := func(kind notify.EventKind, offset time.Duration) notify.Decision {
		t.Helper()
		decision, err := repo.ObserveAlert(ctx, tenantID, dedupKey, notify.Observation{
			Kind: kind, At: start.Add(offset), QuietPeriod: time.Hour, Value: 92.5, Threshold: 90,
		})
		if err != nil {
			t.Fatalf("observe: %v", err)
		}
		return decision
	}

	if d := observe(notify.KindFiring, 0); !d.Notify || d.Reason != notify.ReasonFirstFiring {
		t.Fatalf("first firing: %+v", d)
	}

	// Six hours of five-minute checks.
	notified := 1
	for minute := 5; minute <= 360; minute += 5 {
		if observe(notify.KindFiring, time.Duration(minute)*time.Minute).Notify {
			notified++
		}
	}
	if notified != 7 {
		t.Errorf("six hours of five-minute checks produced %d messages, want 7", notified)
	}

	state, err := repo.GetAlertState(ctx, tenantID, dedupKey)
	if err != nil || state == nil {
		t.Fatalf("get alert state: %v %+v", err, state)
	}
	if state.Occurrences != 73 {
		t.Errorf("occurrences = %d, want 73 folded into one incident", state.Occurrences)
	}
	if state.State != notify.StateFiring {
		t.Errorf("state = %q, want firing", state.State)
	}

	// One row, not seventy-three: the upsert has to be an upsert.
	var rows int
	if err := db.Get(&rows, "SELECT COUNT(*) FROM notification_alert_state WHERE tenant_id = $1 AND dedup_key = $2", tenantID, dedupKey); err != nil {
		t.Fatalf("count: %v", err)
	}
	if rows != 1 {
		t.Errorf("alert state rows = %d, want 1", rows)
	}

	// Resolving sends exactly one message.
	if d := observe(notify.KindResolved, 370*time.Minute); !d.Notify || d.Reason != notify.ReasonResolved {
		t.Fatalf("resolve: %+v", d)
	}
	if d := observe(notify.KindResolved, 375*time.Minute); d.Notify {
		t.Errorf("a second resolve sent another message: %+v", d)
	}

	// And a resolve for a key that never fired creates nothing.
	unknown := "live-never-fired:" + uuid.NewString()
	d, err := repo.ObserveAlert(ctx, tenantID, unknown, notify.Observation{
		Kind: notify.KindResolved, At: start, QuietPeriod: time.Hour,
	})
	if err != nil {
		t.Fatalf("observe unknown: %v", err)
	}
	if d.Notify {
		t.Errorf("a resolve for an alert that never fired notified: %+v", d)
	}
	got, err := repo.GetAlertState(ctx, tenantID, unknown)
	if err != nil {
		t.Fatalf("get unknown state: %v", err)
	}
	if got != nil {
		t.Errorf("a resolve for an unknown key created a row: %+v", got)
		_, _ = db.Exec("DELETE FROM notification_alert_state WHERE dedup_key = $1", unknown)
	}
}

// TestLiveNotifyUpdateChannelKeepsTheStoredSecret is the redaction round trip
// against the real column: read a channel, write the redacted config back, and
// check the database still holds the password.
func TestLiveNotifyUpdateChannelKeepsTheStoredSecret(t *testing.T) {
	repo, db, tenantID := liveNotifyRepo(t)
	ctx := context.Background()
	channelID := makeChannel(t, repo, tenantID, "live redact "+uuid.NewString()[:8])

	// What the API would have handed a client.
	stored, err := repo.GetChannelByID(ctx, tenantID, channelID)
	if err != nil {
		t.Fatalf("get channel: %v", err)
	}
	redacted := notify.RedactConfig(stored.Config)
	if redacted["password"] != notify.Redacted {
		t.Fatalf("the config handed out still contains the password: %v", redacted["password"])
	}

	// The client edits the host and writes the whole object back.
	redacted["host"] = "smtp.new.example.vn"
	incoming := models.JSONMap(redacted)
	if _, err := repo.UpdateChannel(ctx, tenantID, channelID, &models.UpdateNotificationChannelRequest{
		Config: &incoming,
	}); err != nil {
		t.Fatalf("update channel: %v", err)
	}

	var password, host string
	if err := db.Get(&password, "SELECT config->>'password' FROM notification_channels WHERE id = $1", channelID); err != nil {
		t.Fatalf("read password back: %v", err)
	}
	if err := db.Get(&host, "SELECT config->>'host' FROM notification_channels WHERE id = $1", channelID); err != nil {
		t.Fatalf("read host back: %v", err)
	}

	if password != "live-test-smtp-password" {
		t.Errorf("the stored password is now %q; writing a redacted config back destroyed the credential", password)
	}
	if host != "smtp.new.example.vn" {
		t.Errorf("the edited host was not saved: %q", host)
	}
}

// TestLiveNotifyStatusConstraint proves the CHECK constraint is real, so a
// typo in a future query cannot quietly park rows in a status nothing polls.
func TestLiveNotifyStatusConstraint(t *testing.T) {
	repo, db, tenantID := liveNotifyRepo(t)
	channelID := makeChannel(t, repo, tenantID, "live check "+uuid.NewString()[:8])

	_, err := db.Exec(`
		INSERT INTO notification_deliveries (tenant_id, channel_id, event_kind, status)
		VALUES ($1, $2, 'firing', 'in_flight')`, tenantID, channelID)
	if err == nil {
		t.Errorf("an unknown status was accepted; a typo could park alerts where nothing polls")
		_, _ = db.Exec("DELETE FROM notification_deliveries WHERE status = 'in_flight'")
	}

	_, err = db.Exec(`
		INSERT INTO notification_deliveries (tenant_id, channel_id, event_kind, status)
		VALUES ($1, $2, 'exploded', 'pending')`, tenantID, channelID)
	if err == nil {
		t.Errorf("an unknown event kind was accepted")
		_, _ = db.Exec("DELETE FROM notification_deliveries WHERE event_kind = 'exploded'")
	}
}

// TestLiveNotifyListDeliveries exercises the listing the API serves, including
// the dead-letter filter that answers "which alerts reached nobody".
//
// It exists because the first version of this file did not select the joined
// channel columns under the names the record struct expects, and only a real
// database said so - the failure is a scan error, which no fake produces.
func TestLiveNotifyListDeliveries(t *testing.T) {
	repo, db, tenantID := liveNotifyRepo(t)
	ctx := context.Background()
	channelID := makeChannel(t, repo, tenantID, "live list "+uuid.NewString()[:8])

	dedupKey := "live-list:" + uuid.NewString()
	t.Cleanup(func() { _, _ = db.Exec("DELETE FROM notification_deliveries WHERE dedup_key = $1", dedupKey) })

	var ids []uuid.UUID
	for i := 0; i < 3; i++ {
		id, err := repo.EnqueueDelivery(ctx, notify.EnqueueRequest{
			TenantID: tenantID, ChannelID: channelID, DedupKey: dedupKey,
			Kind: notify.KindFiring, Subject: "[CRITICAL] web-01: disk /var", Body: "b",
		})
		if err != nil {
			t.Fatalf("enqueue: %v", err)
		}
		ids = append(ids, id)
	}
	if err := repo.DeadLetter(ctx, ids[0], time.Now(), "gave up after 5 attempts: connection refused"); err != nil {
		t.Fatalf("dead letter: %v", err)
	}

	all, total, err := repo.ListDeliveries(ctx, tenantID, notify.DeliveryFilter{DedupKey: dedupKey})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 3 || len(all) != 3 {
		t.Fatalf("listed %d of %d, want 3 of 3", len(all), total)
	}
	for _, record := range all {
		if record.ChannelName == "" {
			t.Errorf("a listed delivery has no channel name; the join is not being read")
		}
		if record.ChannelType != notify.ChannelEmail {
			t.Errorf("channel type = %q, want %q", record.ChannelType, notify.ChannelEmail)
		}
	}

	dead, deadTotal, err := repo.ListDeliveries(ctx, tenantID, notify.DeliveryFilter{
		DedupKey: dedupKey, Status: notify.StatusDeadLetter,
	})
	if err != nil {
		t.Fatalf("list dead letters: %v", err)
	}
	if deadTotal != 1 || len(dead) != 1 {
		t.Fatalf("dead letters = %d of %d, want 1 of 1", len(dead), deadTotal)
	}
	if !strings.Contains(dead[0].LastError, "connection refused") {
		t.Errorf("the dead letter does not carry the reason: %q", dead[0].LastError)
	}

	// The channel filter has to narrow, not just decorate.
	other := makeChannel(t, repo, tenantID, "live list other "+uuid.NewString()[:8])
	_, otherTotal, err := repo.ListDeliveries(ctx, tenantID, notify.DeliveryFilter{
		DedupKey: dedupKey, ChannelID: &other,
	})
	if err != nil {
		t.Fatalf("list by channel: %v", err)
	}
	if otherTotal != 0 {
		t.Errorf("filtering by a channel with no deliveries returned %d rows", otherTotal)
	}
}

// TestLiveNotifyMissingMigrationIsNamed proves what an installation that has
// not run migrations/pending/notify.sql actually does.
//
// It matters because deploy/install.sh applies migrations with
// `find "${CORE_DIR}/migrations" -maxdepth 1 -name '*.sql'`, which does not
// descend into pending/. Until this migration is renumbered into the sequence,
// every customer install is this case. The panel must say so in one clear
// sentence at startup rather than failing every alert at 3am.
//
// Needs a second database with the numbered migrations applied and the pending
// one deliberately not, named by VKAI_NOTIFY_DSN_NOMIGRATION.
func TestLiveNotifyMissingMigrationIsNamed(t *testing.T) {
	dsn := os.Getenv("VKAI_NOTIFY_DSN_NOMIGRATION")
	if dsn == "" {
		t.Skip("no VKAI_NOTIFY_DSN_NOMIGRATION")
	}
	db, err := sqlx.Connect("pgx", dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = db.Close() }()

	repo := NewNotificationRepository(db)
	err = repo.EnsureDeliverySchema(context.Background())
	if err == nil {
		t.Fatalf("EnsureDeliverySchema passed on a database without the migration; " +
			"the dispatcher would start and fail every alert instead of saying why")
	}
	if !errors.Is(err, ErrDeliveryTablesMissing) {
		t.Errorf("error = %v, want ErrDeliveryTablesMissing so callers can match on it", err)
	}
	if !strings.Contains(err.Error(), "migrations/pending/notify.sql") {
		t.Errorf("the error does not name the file an operator has to apply: %v", err)
	}

	// The rest of the notification feature - the in-panel inbox, the channels,
	// the templates - has to keep working. A panel that cannot deliver alerts
	// is degraded; a panel that will not serve its notification pages is an
	// outage.
	var tenantID uuid.UUID
	if err := db.Get(&tenantID, "SELECT id FROM tenants ORDER BY created_at LIMIT 1"); err != nil {
		t.Fatalf("no tenant: %v", err)
	}
	ctx := context.Background()
	if _, err := repo.ListChannels(ctx, tenantID); err != nil {
		t.Errorf("listing channels failed without the delivery migration: %v", err)
	}
	if _, err := repo.ListTemplates(ctx, tenantID); err != nil {
		t.Errorf("listing templates failed without the delivery migration: %v", err)
	}
	if _, _, err := repo.List(ctx, tenantID, nil, nil, 10, 0); err != nil {
		t.Errorf("listing notifications failed without the delivery migration: %v", err)
	}
}
