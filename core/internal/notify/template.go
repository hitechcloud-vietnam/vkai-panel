package notify

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"text/template"
	"time"
)

// TemplateSet holds the subject and body for each kind of event.
//
// An empty field falls back to the built-in default for that field, so an
// operator who wants to change only the firing subject writes one row in
// notification_templates and inherits the rest.
type TemplateSet struct {
	FiringSubject   string
	FiringBody      string
	ResolvedSubject string
	ResolvedBody    string
	TestSubject     string
	TestBody        string
}

// Template type identifiers, matching notification_templates.type. Using the
// existing table means an operator edits alert wording where they already edit
// every other message, rather than in a second place that looks the same.
const (
	TemplateAlertFiring   = "alert_firing"
	TemplateAlertResolved = "alert_resolved"
	TemplateAlertTest     = "alert_test"
)

// The built-in templates.
//
// Every one of them answers the five questions an operator has at 3am: which
// server, which resource, what was measured, what it was measured against, and
// where do I click. A message that makes someone open the panel to find out
// which machine it is about has failed at the only job it has.
const (
	defaultFiringSubject = `[{{.SeverityUpper}}] {{.Server}}: {{.Resource}} is {{.Value}} (threshold {{.Threshold}})`

	defaultFiringBody = `{{.Summary}}

Server:    {{.Server}}
Resource:  {{.Resource}}{{if .Metric}} (metric {{.Metric}}){{end}}
Measured:  {{.Value}}
Threshold: {{.ConditionPhrase}} {{.Threshold}}
Severity:  {{.SeverityUpper}}
Detected:  {{.OccurredAt}}
{{if gt .Occurrences 1}}Repeated:  {{.Occurrences}} checks since {{.FiringSince}}
{{end}}{{range .Extra}}{{.Key}}: {{.Value}}
{{end}}
Open the panel: {{.Link}}

You are receiving this because a monitoring rule on this server crossed its
threshold. Further messages for this alert are held for {{.QuietPeriod}}.`

	defaultResolvedSubject = `[RESOLVED] {{.Server}}: {{.Resource}} is back to {{.Value}}`

	defaultResolvedBody = `{{.Summary}}

Server:    {{.Server}}
Resource:  {{.Resource}}{{if .Metric}} (metric {{.Metric}}){{end}}
Measured:  {{.Value}}
Threshold: {{.ConditionPhrase}} {{.Threshold}}
Resolved:  {{.OccurredAt}}
{{if .Duration}}Duration:  {{.Duration}}
{{end}}{{if gt .Occurrences 1}}Repeated:  {{.Occurrences}} checks while firing
{{end}}
Open the panel: {{.Link}}`

	defaultTestSubject = `[TEST] vKAI Panel notification channel test`

	defaultTestBody = `This is a test message sent from vKAI Panel.

If you are reading it, this channel works and an alert from {{.Server}} will
reach you the same way.

Sent: {{.OccurredAt}}

Open the panel: {{.Link}}`
)

// DefaultTemplates returns the built-in set.
func DefaultTemplates() TemplateSet {
	return TemplateSet{
		FiringSubject:   defaultFiringSubject,
		FiringBody:      defaultFiringBody,
		ResolvedSubject: defaultResolvedSubject,
		ResolvedBody:    defaultResolvedBody,
		TestSubject:     defaultTestSubject,
		TestBody:        defaultTestBody,
	}
}

// WithOverride returns a copy of the set with one event kind's subject and
// body replaced. An empty subject or body leaves that half at the default.
func (ts TemplateSet) WithOverride(templateType, subject, body string) TemplateSet {
	set := func(dstSubject, dstBody *string) {
		if strings.TrimSpace(subject) != "" {
			*dstSubject = subject
		}
		if strings.TrimSpace(body) != "" {
			*dstBody = body
		}
	}
	switch templateType {
	case TemplateAlertFiring:
		set(&ts.FiringSubject, &ts.FiringBody)
	case TemplateAlertResolved:
		set(&ts.ResolvedSubject, &ts.ResolvedBody)
	case TemplateAlertTest:
		set(&ts.TestSubject, &ts.TestBody)
	}
	return ts
}

// forKind picks the subject and body for an event kind.
func (ts TemplateSet) forKind(kind EventKind) (string, string) {
	switch kind {
	case KindResolved:
		return ts.ResolvedSubject, ts.ResolvedBody
	case KindTest:
		return ts.TestSubject, ts.TestBody
	default:
		return ts.FiringSubject, ts.FiringBody
	}
}

// ExtraPair is one of the caller's extra fields, in a form a template can
// range over deterministically.
type ExtraPair struct {
	Key   string
	Value string
}

// View is what a template sees.
//
// Every numeric field arrives pre-formatted as a string. That is deliberate:
// an operator editing a template in the panel cannot then write something that
// fails to render at the moment an alert fires, which would turn a disk alert
// into a silent template error.
type View struct {
	Kind            string
	Severity        string
	SeverityUpper   string
	Server          string
	ServerID        string
	Resource        string
	Metric          string
	Value           string
	Threshold       string
	Unit            string
	Condition       string
	ConditionPhrase string
	Summary         string
	Link            string
	OccurredAt      string
	FiringSince     string
	Duration        string
	QuietPeriod     string
	DedupKey        string
	Occurrences     int
	Extra           []ExtraPair

	// The raw numbers, for a template that wants to compare rather than print.
	ValueNumber     float64
	ThresholdNumber float64
}

// RenderOptions carries what rendering needs beyond the alert itself.
type RenderOptions struct {
	// PanelBaseURL is the panel's externally reachable base, for example
	// https://panel.example.vn. An empty base produces a relative link rather
	// than a broken absolute one.
	PanelBaseURL string
	// QuietPeriod is written into the firing body so the reader knows how long
	// the silence between reminders is, rather than wondering whether alerting
	// has stopped.
	QuietPeriod time.Duration
	// Location renders timestamps in the operator's timezone. Nil means UTC.
	Location *time.Location
}

// NewView builds the template view for an alert.
func NewView(a Alert, opts RenderOptions) View {
	location := opts.Location
	if location == nil {
		location = time.UTC
	}

	extra := make([]ExtraPair, 0, len(a.Extra))
	for k, v := range a.Extra {
		extra = append(extra, ExtraPair{Key: k, Value: v})
	}
	sort.Slice(extra, func(i, j int) bool { return extra[i].Key < extra[j].Key })

	view := View{
		Kind:            string(a.Kind),
		Severity:        string(a.Severity),
		SeverityUpper:   strings.ToUpper(string(a.Severity)),
		Server:          a.ServerName,
		ServerID:        a.ServerID,
		Resource:        a.Resource,
		Metric:          a.Metric,
		Value:           formatValue(a.Value, a.Unit),
		Threshold:       formatValue(a.Threshold, a.Unit),
		Unit:            a.Unit,
		Condition:       a.Condition,
		ConditionPhrase: conditionPhrase(a.Condition),
		Summary:         a.Summary,
		Link:            BuildLink(opts.PanelBaseURL, a),
		OccurredAt:      a.OccurredAt.In(location).Format("2006-01-02 15:04:05 MST"),
		QuietPeriod:     humanDuration(opts.QuietPeriod),
		DedupKey:        a.DedupKey,
		Occurrences:     a.Occurrences,
		Extra:           extra,
		ValueNumber:     a.Value,
		ThresholdNumber: a.Threshold,
	}

	if !a.FiringSince.IsZero() {
		view.FiringSince = a.FiringSince.In(location).Format("2006-01-02 15:04:05 MST")
		if a.Kind == KindResolved && a.OccurredAt.After(a.FiringSince) {
			view.Duration = humanDuration(a.OccurredAt.Sub(a.FiringSince))
		}
	}
	return view
}

// BuildLink returns the URL of the panel page an operator should open.
//
// The alert may name a path; otherwise one is derived from the server id. A
// base URL that is missing yields a relative path, which is still useful to
// somebody already looking at the panel and is honest about not knowing the
// panel's public address.
func BuildLink(base string, a Alert) string {
	path := a.PanelPath
	if path == "" {
		switch {
		case a.ServerID != "":
			path = "/monitoring/servers/" + url.PathEscape(a.ServerID)
			if a.Metric != "" {
				path += "?metric=" + url.QueryEscape(a.Metric)
			}
		default:
			path = "/monitoring"
		}
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		return path
	}
	return base + path
}

// humanDuration renders a duration the way an operator says it.
func humanDuration(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%d seconds", int(d.Seconds()))
	case d < time.Hour:
		minutes := int(d.Minutes())
		if minutes == 1 {
			return "1 minute"
		}
		return fmt.Sprintf("%d minutes", minutes)
	case d < 24*time.Hour:
		hours := int(d.Hours())
		minutes := int(d.Minutes()) % 60
		if minutes == 0 {
			if hours == 1 {
				return "1 hour"
			}
			return fmt.Sprintf("%d hours", hours)
		}
		return fmt.Sprintf("%dh %dm", hours, minutes)
	default:
		days := int(d.Hours()) / 24
		hours := int(d.Hours()) % 24
		if hours == 0 {
			if days == 1 {
				return "1 day"
			}
			return fmt.Sprintf("%d days", days)
		}
		return fmt.Sprintf("%dd %dh", days, hours)
	}
}

// Render turns an alert into the message that will be delivered.
//
// A template an operator has edited into something that will not parse or will
// not execute must not cost anybody an alert. When a custom template fails,
// the built-in one is used for that field and the failure is returned
// alongside the message, for the caller to log. The message is always usable.
func Render(ts TemplateSet, a Alert, opts RenderOptions) (Message, error) {
	view := NewView(a, opts)
	defaults := DefaultTemplates()

	subjectText, bodyText := ts.forKind(a.Kind)
	defaultSubject, defaultBody := defaults.forKind(a.Kind)

	var problems []string

	subject, err := renderOne("subject", subjectText, view)
	if err != nil {
		problems = append(problems, err.Error())
		subject, _ = renderOne("subject", defaultSubject, view)
	}

	body, err := renderOne("body", bodyText, view)
	if err != nil {
		problems = append(problems, err.Error())
		body, _ = renderOne("body", defaultBody, view)
	}

	// The link travels on the alert as well as on the message. The outbox
	// stores the alert, and the sender that reads it back has no way to
	// rebuild an absolute URL without the panel's base address.
	a.Link = view.Link

	msg := Message{
		Kind:     a.Kind,
		Severity: a.Severity,
		Subject:  strings.TrimSpace(subject),
		Body:     strings.TrimSpace(body),
		Link:     view.Link,
		Alert:    a,
	}
	if len(problems) > 0 {
		return msg, fmt.Errorf("custom notification template failed, built-in used instead: %s",
			strings.Join(problems, "; "))
	}
	return msg, nil
}

// renderOne parses and executes a single template.
func renderOne(name, text string, view View) (string, error) {
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("%s template is empty", name)
	}
	// Option "missingkey=zero" is not set: View has no maps, so an unknown
	// field is a parse-time or execute-time error, which is what should happen
	// when a template refers to something that does not exist.
	parsed, err := template.New(name).Parse(text)
	if err != nil {
		return "", fmt.Errorf("%s template does not parse: %w", name, err)
	}
	var out strings.Builder
	if err := parsed.Execute(&out, view); err != nil {
		return "", fmt.Errorf("%s template does not render: %w", name, err)
	}
	return out.String(), nil
}
