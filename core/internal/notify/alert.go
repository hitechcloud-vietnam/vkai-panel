package notify

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Severity ranks an alert. It is carried into the rendered subject so an
// operator triaging a full inbox can sort without opening anything.
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

// Valid reports whether s is one of the three known severities.
func (s Severity) Valid() bool {
	switch s {
	case SeverityInfo, SeverityWarning, SeverityCritical:
		return true
	}
	return false
}

// EventKind says what happened to the alert, not what the alert is about.
type EventKind string

const (
	// KindFiring is a threshold that has been crossed, or is still crossed and
	// the quiet period has elapsed.
	KindFiring EventKind = "firing"
	// KindResolved is sent once, when a firing alert clears.
	KindResolved EventKind = "resolved"
	// KindTest comes from the test-send endpoint. It is never deduplicated: an
	// operator pressing "test" twice expects two messages.
	KindTest EventKind = "test"
)

// Valid reports whether k is one of the three known kinds.
func (k EventKind) Valid() bool {
	switch k {
	case KindFiring, KindResolved, KindTest:
		return true
	}
	return false
}

// Alert is what a caller hands to the notifier. Every field that an operator
// needs in order to act is a field here rather than a sentence in Summary,
// because a sender that posts JSON (a webhook) has to be able to read them
// apart, and because a template author has to be able to reorder them.
type Alert struct {
	// DedupKey groups observations that are the same incident. The caller
	// chooses it, and it is the only thing deduplication looks at. A
	// reasonable key for a disk alert is "server:<id>:disk:/var" - stable
	// across checks, distinct per mount point.
	DedupKey string `json:"dedup_key"`

	// Kind is firing, resolved or test.
	Kind EventKind `json:"kind"`

	Severity Severity `json:"severity"`

	// ServerID and ServerName identify the machine. ServerName is what a human
	// reads; ServerID is what the panel link is built from.
	ServerID   string `json:"server_id"`
	ServerName string `json:"server_name"`

	// Resource is the thing under pressure in the words an operator uses:
	// "disk /var", "memory", "cpu". Metric is the series it was measured from.
	Resource string `json:"resource"`
	Metric   string `json:"metric"`

	// Value is what was measured, Threshold is what it was compared against,
	// and Condition is the comparison ("gt", "lt", "gte", "lte", "eq", "ne")
	// as stored on monitoring_alerts.
	Value     float64 `json:"value"`
	Threshold float64 `json:"threshold"`
	Condition string  `json:"condition"`
	Unit      string  `json:"unit"`

	// Summary is one sentence. If it is empty a sentence is built from the
	// fields above, so a caller that fills in nothing else still sends
	// something readable.
	Summary string `json:"summary"`

	// PanelPath is the path, relative to the panel's base URL, of the page an
	// operator should open. Empty means "derive one from ServerID".
	PanelPath string `json:"panel_path"`

	// Link is the absolute URL of that page. A caller leaves it empty and
	// sets PanelPath; Render fills it in from the panel's base URL and it is
	// stored with the outbox row, so a sender that runs minutes later - and
	// knows nothing about the panel's public address - still has it.
	Link string `json:"link,omitempty"`

	// OccurredAt is when the observation was taken. Zero means now.
	OccurredAt time.Time `json:"occurred_at"`

	// Occurrences is filled in by the notifier from the deduplication state:
	// how many checks this incident has spanned. A caller does not set it.
	Occurrences int `json:"occurrences"`

	// FiringSince is filled in by the notifier for a resolution message, so it
	// can say how long the incident lasted.
	FiringSince time.Time `json:"firing_since,omitempty"`

	// Extra is anything else worth putting in front of an operator. It is
	// rendered as trailing "key: value" lines and passed through to webhook
	// payloads untouched.
	Extra map[string]string `json:"extra,omitempty"`
}

// ErrInvalidAlert is the base of every validation failure, so a caller can
// answer 400 without matching on message text.
var ErrInvalidAlert = errors.New("invalid alert")

// Validate rejects an alert that could not produce a useful message. It is
// deliberately strict about DedupKey for firing and resolved events: without a
// key there is no deduplication, and an alert that cannot be deduplicated is
// the outage-on-top-of-an-outage this package exists to prevent.
func (a *Alert) Validate() error {
	if !a.Kind.Valid() {
		return fmt.Errorf("%w: kind must be one of firing, resolved, test (got %q)", ErrInvalidAlert, a.Kind)
	}
	if a.Kind != KindTest && strings.TrimSpace(a.DedupKey) == "" {
		return fmt.Errorf("%w: dedup_key is required for %s events", ErrInvalidAlert, a.Kind)
	}
	if len(a.DedupKey) > 512 {
		return fmt.Errorf("%w: dedup_key must be 512 characters or fewer", ErrInvalidAlert)
	}
	if a.Severity == "" {
		a.Severity = SeverityWarning
	}
	if !a.Severity.Valid() {
		return fmt.Errorf("%w: severity must be one of info, warning, critical (got %q)", ErrInvalidAlert, a.Severity)
	}
	return nil
}

// Normalize fills in the defaults a caller is allowed to leave out. It is
// called by the notifier after Validate.
func (a *Alert) Normalize(now time.Time) {
	if a.OccurredAt.IsZero() {
		a.OccurredAt = now
	}
	if a.ServerName == "" {
		if a.ServerID != "" {
			a.ServerName = a.ServerID
		} else {
			a.ServerName = "this server"
		}
	}
	if a.Resource == "" {
		a.Resource = a.Metric
	}
	if a.Resource == "" {
		a.Resource = "the system"
	}
	if a.Summary == "" {
		a.Summary = a.defaultSummary()
	}
}

// defaultSummary writes the sentence a caller did not write.
func (a *Alert) defaultSummary() string {
	switch a.Kind {
	case KindResolved:
		return fmt.Sprintf("%s on %s is back within its threshold (%s, threshold %s).",
			a.Resource, a.ServerName, formatValue(a.Value, a.Unit), formatValue(a.Threshold, a.Unit))
	case KindTest:
		return fmt.Sprintf("Test message from vKAI Panel for %s.", a.ServerName)
	default:
		return fmt.Sprintf("%s on %s is %s, which is %s the threshold of %s.",
			a.Resource, a.ServerName, formatValue(a.Value, a.Unit),
			conditionPhrase(a.Condition), formatValue(a.Threshold, a.Unit))
	}
}

// conditionPhrase turns a stored condition code into words. An unknown code
// falls back to the code itself rather than to silence, so a typo in a
// monitoring rule shows up in the message instead of disappearing.
func conditionPhrase(condition string) string {
	switch condition {
	case "gt":
		return "above"
	case "gte":
		return "at or above"
	case "lt":
		return "below"
	case "lte":
		return "at or below"
	case "eq":
		return "equal to"
	case "ne":
		return "different from"
	case "":
		return "past"
	default:
		return condition
	}
}

// formatValue renders a measurement the way an operator writes it: no trailing
// zeros, and the unit attached with no space for "%" and with one otherwise.
func formatValue(v float64, unit string) string {
	s := strings.TrimSuffix(strings.TrimRight(fmt.Sprintf("%.2f", v), "0"), ".")
	if s == "" || s == "-" {
		s = "0"
	}
	switch unit {
	case "":
		return s
	case "%":
		return s + "%"
	default:
		return s + " " + unit
	}
}

// Message is the rendered result handed to a Sender. Subject and Body are for
// the senders that carry text; Alert is kept alongside for the senders that
// post structured JSON, so a webhook consumer does not have to parse prose.
type Message struct {
	Kind     EventKind `json:"kind"`
	Severity Severity  `json:"severity"`
	Subject  string    `json:"subject"`
	Body     string    `json:"body"`
	Link     string    `json:"link"`
	Alert    Alert     `json:"alert"`
}
