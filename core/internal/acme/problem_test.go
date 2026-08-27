package acme

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestProblemParsingWithSubproblems(t *testing.T) {
	body := `{
	  "type": "urn:ietf:params:acme:error:malformed",
	  "title": "Some of the identifiers requested were rejected",
	  "status": 403,
	  "detail": "Some of the identifiers requested were rejected",
	  "instance": "https://ca.example/problem/1",
	  "subproblems": [
	    {
	      "type": "urn:ietf:params:acme:error:unauthorized",
	      "detail": "Invalid response from http://203.0.113.10/.well-known/acme-challenge/abc: 404",
	      "identifier": {"type": "ip", "value": "203.0.113.10"}
	    },
	    {
	      "type": "urn:ietf:params:acme:error:rejectedIdentifier",
	      "detail": "This CA will not issue for panel.internal",
	      "identifier": {"type": "dns", "value": "panel.internal"}
	    }
	  ]
	}`

	var prob Problem
	if err := json.Unmarshal([]byte(body), &prob); err != nil {
		t.Fatalf("unmarshal problem: %v", err)
	}

	if prob.Type != "urn:ietf:params:acme:error:malformed" {
		t.Fatalf("type = %q", prob.Type)
	}
	if prob.Status != 403 {
		t.Fatalf("status = %d", prob.Status)
	}
	if prob.Instance != "https://ca.example/problem/1" {
		t.Fatalf("instance = %q", prob.Instance)
	}
	if len(prob.Subproblems) != 2 {
		t.Fatalf("got %d subproblems, want 2", len(prob.Subproblems))
	}
	first := prob.Subproblems[0]
	if first.Type != "urn:ietf:params:acme:error:unauthorized" {
		t.Fatalf("subproblem type = %q", first.Type)
	}
	if first.Identifier == nil || first.Identifier.Type != IdentifierIP || first.Identifier.Value != "203.0.113.10" {
		t.Fatalf("subproblem identifier = %+v", first.Identifier)
	}

	// The rendered error must carry everything a log line needs to diagnose the
	// failure without the original response body.
	msg := prob.Error()
	for _, want := range []string{
		"urn:ietf:params:acme:error:malformed",
		"status 403",
		"https://ca.example/problem/1",
		"urn:ietf:params:acme:error:unauthorized",
		"ip:203.0.113.10",
		"404",
		"urn:ietf:params:acme:error:rejectedIdentifier",
		"dns:panel.internal",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("problem message is missing %q:\n%s", want, msg)
		}
	}
}

func TestProblemErrorsIs(t *testing.T) {
	err := error(&Problem{Type: "urn:ietf:params:acme:error:unauthorized", Detail: "no"})
	if !errors.Is(err, &Problem{Type: "urn:ietf:params:acme:error:unauthorized"}) {
		t.Fatal("errors.Is must match problems by their URN type")
	}
	if errors.Is(err, &Problem{Type: errBadNonce}) {
		t.Fatal("errors.Is must not match a different URN type")
	}
}

func TestResponseToErrorProducesProblem(t *testing.T) {
	resp := &acmeResponse{
		Status: http.StatusForbidden,
		Header: http.Header{"Content-Type": []string{"application/problem+json"}},
		Body:   []byte(`{"type":"urn:ietf:params:acme:error:unauthorized","detail":"nope"}`),
	}
	err := resp.toError("https://ca.example/authz/1")
	var prob *Problem
	if !errors.As(err, &prob) {
		t.Fatalf("expected a *Problem, got %T: %v", err, err)
	}
	if prob.Status != http.StatusForbidden {
		t.Fatalf("status should be filled in from the response, got %d", prob.Status)
	}
}

func TestResponseToErrorProducesRateLimitError(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		header     http.Header
		body       string
		wantWait   time.Duration
		wantHeader string
	}{
		{
			name:   "HTTP 429 with seconds",
			status: http.StatusTooManyRequests,
			header: http.Header{
				"Content-Type": []string{"application/problem+json"},
				"Retry-After":  []string{"120"},
			},
			body:       `{"type":"urn:ietf:params:acme:error:rateLimited","detail":"too many certificates already issued"}`,
			wantWait:   120 * time.Second,
			wantHeader: "120",
		},
		{
			name:   "rateLimited URN without HTTP 429",
			status: http.StatusForbidden,
			header: http.Header{
				"Content-Type": []string{"application/problem+json"},
				"Retry-After":  []string{"3600"},
			},
			body:       `{"type":"urn:ietf:params:acme:error:rateLimited","detail":"slow down"}`,
			wantWait:   time.Hour,
			wantHeader: "3600",
		},
		{
			name:       "no Retry-After at all",
			status:     http.StatusTooManyRequests,
			header:     http.Header{"Content-Type": []string{"application/problem+json"}},
			body:       `{"type":"urn:ietf:params:acme:error:rateLimited","detail":"slow down"}`,
			wantWait:   0,
			wantHeader: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp := &acmeResponse{Status: tc.status, Header: tc.header, Body: []byte(tc.body)}
			err := resp.toError("https://ca.example/new-order")

			var rl *RateLimitError
			if !errors.As(err, &rl) {
				t.Fatalf("expected a *RateLimitError, got %T: %v", err, err)
			}
			if rl.RetryAfter != tc.wantWait {
				t.Fatalf("RetryAfter = %s, want %s", rl.RetryAfter, tc.wantWait)
			}
			if rl.RetryAfterHeader != tc.wantHeader {
				t.Fatalf("RetryAfterHeader = %q, want %q", rl.RetryAfterHeader, tc.wantHeader)
			}
			if rl.StatusCode != tc.status {
				t.Fatalf("StatusCode = %d", rl.StatusCode)
			}
			if rl.URL != "https://ca.example/new-order" {
				t.Fatalf("URL = %q", rl.URL)
			}
			// The problem document must still be reachable through the wrapper.
			var prob *Problem
			if !errors.As(err, &prob) {
				t.Fatal("a RateLimitError must unwrap to its problem document")
			}
			if !prob.IsRateLimited() {
				t.Fatalf("problem type = %q", prob.Type)
			}
		})
	}
}

func TestParseRetryAfterHTTPDate(t *testing.T) {
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	header := http.Header{"Retry-After": []string{now.Add(90 * time.Second).Format(http.TimeFormat)}}

	wait, raw := parseRetryAfter(header, now)
	if wait != 90*time.Second {
		t.Fatalf("wait = %s, want 90s", wait)
	}
	if raw == "" {
		t.Fatal("the raw header value must be preserved")
	}

	// A date already in the past means "retry now".
	past := http.Header{"Retry-After": []string{now.Add(-time.Minute).Format(http.TimeFormat)}}
	if wait, _ := parseRetryAfter(past, now); wait != 0 {
		t.Fatalf("wait for a past date = %s, want 0", wait)
	}

	// Garbage must not be mistaken for a duration but is still surfaced.
	garbage := http.Header{"Retry-After": []string{"soon"}}
	wait, raw = parseRetryAfter(garbage, now)
	if wait != 0 || raw != "soon" {
		t.Fatalf("garbage Retry-After: wait = %s, raw = %q", wait, raw)
	}
}

func TestResponseToErrorWithoutProblemDocument(t *testing.T) {
	resp := &acmeResponse{
		Status: http.StatusBadGateway,
		Header: http.Header{"Content-Type": []string{"text/html"}},
		Body:   []byte("<html>502 from a proxy</html>"),
	}
	err := resp.toError("https://ca.example/new-order")
	var prob *Problem
	if errors.As(err, &prob) {
		t.Fatal("a non-problem body must not be reported as a problem document")
	}
	if !strings.Contains(err.Error(), "502") || !strings.Contains(err.Error(), "proxy") {
		t.Fatalf("the raw body should reach the message: %v", err)
	}
}
