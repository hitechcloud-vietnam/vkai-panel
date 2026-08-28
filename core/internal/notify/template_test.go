package notify

import (
	"strings"
	"testing"
	"time"
)

// diskAlert is the alert this whole feature was described by: a disk filling
// up on a named server.
func diskAlert() Alert {
	a := Alert{
		DedupKey:   "server:web-01:disk:/var",
		Kind:       KindFiring,
		Severity:   SeverityCritical,
		ServerID:   "3f2b9c14-0000-4000-8000-00000000abcd",
		ServerName: "web-01.hcm.example.vn",
		Resource:   "disk /var",
		Metric:     "disk_used_percent",
		Value:      92.5,
		Threshold:  90,
		Condition:  "gt",
		Unit:       "%",
		OccurredAt: time.Date(2026, 3, 14, 9, 30, 0, 0, time.UTC),
	}
	a.Normalize(a.OccurredAt)
	return a
}

// TestRenderCarriesEverythingAnOperatorNeeds is the acceptance test for the
// message itself. An alert that does not say which machine, which resource,
// what was measured, what it was measured against, and where to click has
// failed at its only job.
func TestRenderCarriesEverythingAnOperatorNeeds(t *testing.T) {
	msg, err := Render(DefaultTemplates(), diskAlert(), RenderOptions{
		PanelBaseURL: "https://panel.example.vn",
		QuietPeriod:  time.Hour,
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	body := msg.Subject + "\n" + msg.Body

	required := map[string]string{
		"the server":     "web-01.hcm.example.vn",
		"the resource":   "disk /var",
		"the metric":     "disk_used_percent",
		"the value":      "92.5%",
		"the threshold":  "90%",
		"the severity":   "CRITICAL",
		"the time":       "2026-03-14 09:30:00",
		"the panel link": "https://panel.example.vn/monitoring/servers/3f2b9c14-0000-4000-8000-00000000abcd",
	}
	for what, want := range required {
		if !strings.Contains(body, want) {
			t.Errorf("the message does not carry %s (%q):\n--- subject ---\n%s\n--- body ---\n%s",
				what, want, msg.Subject, msg.Body)
		}
	}

	// The subject alone has to be triageable: which machine, how bad.
	if !strings.Contains(msg.Subject, "web-01.hcm.example.vn") {
		t.Errorf("subject does not name the server: %q", msg.Subject)
	}
	if !strings.Contains(msg.Subject, "CRITICAL") {
		t.Errorf("subject does not carry the severity: %q", msg.Subject)
	}

	// The link is on the message as a field too, for senders that render it
	// separately.
	if msg.Link == "" {
		t.Errorf("Message.Link is empty")
	}

	// And on the alert, which is what the outbox stores. A sender running
	// minutes later has no other way to rebuild an absolute URL: it knows
	// nothing about the panel's public address. Dropping this is how the
	// webhook payload shipped with an empty link.
	if msg.Alert.Link != msg.Link {
		t.Errorf("Alert.Link = %q, want the rendered link %q; "+
			"the link would be lost between rendering and delivery",
			msg.Alert.Link, msg.Link)
	}
}

func TestRenderResolvedMessage(t *testing.T) {
	alert := diskAlert()
	alert.Kind = KindResolved
	alert.Value = 41
	alert.Occurrences = 12
	alert.FiringSince = time.Date(2026, 3, 14, 9, 30, 0, 0, time.UTC)
	alert.OccurredAt = time.Date(2026, 3, 14, 11, 45, 0, 0, time.UTC)
	alert.Summary = ""
	alert.Normalize(alert.OccurredAt)

	msg, err := Render(DefaultTemplates(), alert, RenderOptions{PanelBaseURL: "https://panel.example.vn"})
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	if !strings.Contains(msg.Subject, "RESOLVED") {
		t.Errorf("a resolution message must be recognisable from the subject alone: %q", msg.Subject)
	}
	if !strings.Contains(msg.Subject, "web-01.hcm.example.vn") {
		t.Errorf("resolution subject does not name the server: %q", msg.Subject)
	}
	if !strings.Contains(msg.Body, "41%") {
		t.Errorf("resolution body does not carry the value it recovered to:\n%s", msg.Body)
	}
	// How long the incident lasted is the first thing anybody asks afterwards.
	if !strings.Contains(msg.Body, "2h 15m") {
		t.Errorf("resolution body does not carry the duration:\n%s", msg.Body)
	}
	if !strings.Contains(msg.Body, "12 checks") {
		t.Errorf("resolution body does not say how many checks it spanned:\n%s", msg.Body)
	}
}

func TestRenderRepeatedFiringSaysSo(t *testing.T) {
	alert := diskAlert()
	alert.Occurrences = 13
	alert.FiringSince = time.Date(2026, 3, 14, 9, 30, 0, 0, time.UTC)
	alert.OccurredAt = time.Date(2026, 3, 14, 10, 30, 0, 0, time.UTC)

	msg, err := Render(DefaultTemplates(), alert, RenderOptions{QuietPeriod: time.Hour})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(msg.Body, "13 checks") {
		t.Errorf("a reminder must say the alert has been repeating:\n%s", msg.Body)
	}
	// And the reader has to know the silence between reminders is deliberate.
	if !strings.Contains(msg.Body, "1 hour") {
		t.Errorf("the body does not state the quiet period:\n%s", msg.Body)
	}
}

func TestRenderExtraFields(t *testing.T) {
	alert := diskAlert()
	alert.Extra = map[string]string{"mount": "/var", "filesystem": "ext4"}

	msg, err := Render(DefaultTemplates(), alert, RenderOptions{})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for _, want := range []string{"mount: /var", "filesystem: ext4"} {
		if !strings.Contains(msg.Body, want) {
			t.Errorf("body is missing %q:\n%s", want, msg.Body)
		}
	}
	// Deterministic order, so a diff of two alerts is readable.
	if strings.Index(msg.Body, "filesystem:") > strings.Index(msg.Body, "mount:") {
		t.Errorf("extra fields are not in a stable order:\n%s", msg.Body)
	}
}

// TestRenderBrokenCustomTemplateStillProducesAMessage is the rule that an
// operator's typo must not cost an alert.
func TestRenderBrokenCustomTemplateStillProducesAMessage(t *testing.T) {
	broken := DefaultTemplates()
	broken.FiringSubject = "{{.NoSuchField}} unbalanced {{"
	broken.FiringBody = "{{range .NotAList}}{{end}}"

	msg, err := Render(broken, diskAlert(), RenderOptions{PanelBaseURL: "https://panel.example.vn"})
	if err == nil {
		t.Fatalf("a broken template rendered without reporting a problem")
	}
	if !strings.Contains(err.Error(), "built-in used instead") {
		t.Errorf("the error does not say what happened: %v", err)
	}

	// The message is still usable, and still carries the operator's facts.
	if msg.Subject == "" || msg.Body == "" {
		t.Fatalf("a broken template produced an empty message: %+v", msg)
	}
	if !strings.Contains(msg.Body, "web-01.hcm.example.vn") {
		t.Errorf("the fallback message lost the server name:\n%s", msg.Body)
	}
	if !strings.Contains(msg.Body, "https://panel.example.vn") {
		t.Errorf("the fallback message lost the link:\n%s", msg.Body)
	}
}

func TestTemplateSetWithOverride(t *testing.T) {
	set := DefaultTemplates().WithOverride(TemplateAlertFiring, "CUSTOM {{.Server}}", "")

	msg, err := Render(set, diskAlert(), RenderOptions{})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.HasPrefix(msg.Subject, "CUSTOM ") {
		t.Errorf("the custom subject was not used: %q", msg.Subject)
	}
	// An empty half inherits the default rather than rendering nothing.
	if !strings.Contains(msg.Body, "Threshold:") {
		t.Errorf("an empty custom body did not fall back to the built-in one:\n%s", msg.Body)
	}

	// Overriding one kind must not disturb another.
	resolved := diskAlert()
	resolved.Kind = KindResolved
	resolvedMsg, err := Render(set, resolved, RenderOptions{})
	if err != nil {
		t.Fatalf("render resolved: %v", err)
	}
	if !strings.Contains(resolvedMsg.Subject, "RESOLVED") {
		t.Errorf("overriding the firing template changed the resolved one: %q", resolvedMsg.Subject)
	}
}

func TestBuildLink(t *testing.T) {
	cases := []struct {
		name  string
		base  string
		alert Alert
		want  string
	}{
		{
			name:  "derived from the server id",
			base:  "https://panel.example.vn",
			alert: Alert{ServerID: "abc-123", Metric: "disk_used_percent"},
			want:  "https://panel.example.vn/monitoring/servers/abc-123?metric=disk_used_percent",
		},
		{
			name:  "an explicit path wins",
			base:  "https://panel.example.vn/",
			alert: Alert{ServerID: "abc-123", PanelPath: "/settings/notifications"},
			want:  "https://panel.example.vn/settings/notifications",
		},
		{
			name:  "no server at all",
			base:  "https://panel.example.vn",
			alert: Alert{},
			want:  "https://panel.example.vn/monitoring",
		},
		{
			name:  "no base URL yields a relative path rather than a broken absolute one",
			base:  "",
			alert: Alert{ServerID: "abc-123"},
			want:  "/monitoring/servers/abc-123",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := BuildLink(tc.base, tc.alert); got != tc.want {
				t.Errorf("BuildLink() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFormatValue(t *testing.T) {
	cases := []struct {
		value float64
		unit  string
		want  string
	}{
		{92.5, "%", "92.5%"},
		{90, "%", "90%"},
		{1.25, "GB", "1.25 GB"},
		{0, "", "0"},
		{92.567, "%", "92.57%"},
	}
	for _, tc := range cases {
		if got := formatValue(tc.value, tc.unit); got != tc.want {
			t.Errorf("formatValue(%v, %q) = %q, want %q", tc.value, tc.unit, got, tc.want)
		}
	}
}

func TestDefaultSummaryReadsAsASentence(t *testing.T) {
	alert := diskAlert()
	alert.Summary = ""
	alert.Normalize(alert.OccurredAt)

	want := "disk /var on web-01.hcm.example.vn is 92.5%, which is above the threshold of 90%."
	if alert.Summary != want {
		t.Errorf("summary = %q, want %q", alert.Summary, want)
	}
}
