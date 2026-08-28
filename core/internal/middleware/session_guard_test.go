package middleware

// The session binding gate as a wrapper: what it acts on and what it must
// leave alone.
//
// The end-to-end behaviour - a session established, moved, re-bound, ended -
// is driven through the real router in internal/handler. What is checked here
// is the wrapper's own contract, which is easy to get wrong in a way that is
// invisible from higher up: this gate must be able to refuse a request and must
// never be able to admit one.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/auth"
)

type stubEvaluator struct {
	verdict auth.SessionVerdict
	err     error
	seen    []auth.SessionRequest
}

func (s *stubEvaluator) EvaluateSession(_ context.Context, req auth.SessionRequest) (auth.SessionVerdict, error) {
	s.seen = append(s.seen, req)
	return s.verdict, s.err
}

func reachedHandler() (http.Handler, *bool) {
	reached := false
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusTeapot)
	}), &reached
}

func guardedRequest(t *testing.T, guard http.Handler, method, path, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.RemoteAddr = "203.0.113.10:5555"
	recorder := httptest.NewRecorder()
	guard.ServeHTTP(recorder, req)
	return recorder
}

func testJWT(t *testing.T) (*auth.JWTManager, string) {
	t.Helper()
	manager := auth.NewJWTManager("session-guard-test", time.Hour, 24*time.Hour, "vkai-guard-test")
	pair, err := manager.GenerateTokenPair(uuid.New(), uuid.New(), "operator", "op@example.test", nil)
	if err != nil {
		t.Fatalf("mint a token: %v", err)
	}
	return manager, pair.AccessToken
}

func TestBindSessionsOnlyActsOnAuthenticatedRequests(t *testing.T) {
	manager, token := testJWT(t)
	evaluator := &stubEvaluator{verdict: auth.SessionVerdict{Allow: false, Reason: "revoked"}}

	next, reached := reachedHandler()
	guard := BindSessions(next, SessionGuardOptions{Evaluator: evaluator, JWT: manager, Logger: zap.NewNop()})

	t.Run("no credential at all", func(t *testing.T) {
		*reached = false
		if code := guardedRequest(t, guard, http.MethodPost, "/api/v1/auth/login", "").Code; code != http.StatusTeapot {
			t.Fatalf("the login form was refused by the session gate: %d", code)
		}
		if len(evaluator.seen) != 0 {
			t.Fatal("the gate consulted the session store for an unauthenticated request")
		}
	})

	t.Run("a bearer token that is not ours", func(t *testing.T) {
		*reached = false
		if code := guardedRequest(t, guard, http.MethodGet, "/api/v1/websites", "not-a-token").Code; code != http.StatusTeapot {
			t.Fatalf("an invalid token was refused here instead of by the route's own authentication: %d", code)
		}
	})

	t.Run("a valid token is evaluated", func(t *testing.T) {
		*reached = false
		recorder := guardedRequest(t, guard, http.MethodGet, "/api/v1/websites", token)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("a refused session reached the handler: %d", recorder.Code)
		}
		if *reached {
			t.Fatal("the handler ran for a session the evaluator refused")
		}
		if len(evaluator.seen) == 0 {
			t.Fatal("the evaluator was never asked")
		}
		last := evaluator.seen[len(evaluator.seen)-1]
		if last.IP != "203.0.113.10" {
			t.Fatalf("the gate reported the source address as %q", last.IP)
		}
		if last.TokenID == "" {
			t.Fatal("the gate did not pass the token's jti, so no session can be identified")
		}
	})
}

func TestBindSessionsAnswersInTheAPIsOwnShape(t *testing.T) {
	manager, token := testJWT(t)

	for _, tc := range []struct {
		name    string
		verdict auth.SessionVerdict
		status  int
		code    string
	}{
		{
			name:    "an ended session",
			verdict: auth.SessionVerdict{Allow: false, Reason: "revoked"},
			status:  http.StatusUnauthorized,
			code:    SessionEndedCode,
		},
		{
			name:    "a session that must prove its password",
			verdict: auth.SessionVerdict{Allow: false, ReauthRequired: true, Reason: "network_changed"},
			status:  http.StatusForbidden,
			code:    SessionReauthCode,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			next, _ := reachedHandler()
			guard := BindSessions(next, SessionGuardOptions{
				Evaluator: &stubEvaluator{verdict: tc.verdict},
				JWT:       manager,
				Logger:    zap.NewNop(),
			})

			recorder := guardedRequest(t, guard, http.MethodDelete, "/api/v1/websites/1", token)
			if recorder.Code != tc.status {
				t.Fatalf("status = %d, want %d", recorder.Code, tc.status)
			}

			var body struct {
				Success bool `json:"success"`
				Error   *struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatalf("the gate did not answer in the API's envelope: %v (%s)", err, recorder.Body.String())
			}
			if body.Success || body.Error == nil || body.Error.Code != tc.code {
				t.Fatalf("body = %s, want an error with code %s", recorder.Body.String(), tc.code)
			}
		})
	}
}

// TestBindSessionsLeavesTheWayOutOpen is the trap this control would otherwise
// set for itself: both routes that get a moved session out of the state are
// state-changing, so refusing every state-changing request would leave the
// operator with no move except abandoning the session.
func TestBindSessionsLeavesTheWayOutOpen(t *testing.T) {
	manager, token := testJWT(t)

	for _, path := range []string{ReauthenticatePath, LogoutPath} {
		next, reached := reachedHandler()
		guard := BindSessions(next, SessionGuardOptions{
			Evaluator: &stubEvaluator{verdict: auth.SessionVerdict{Allow: false, ReauthRequired: true}},
			JWT:       manager,
			Logger:    zap.NewNop(),
		})

		if code := guardedRequest(t, guard, http.MethodPost, path, token).Code; code != http.StatusTeapot {
			t.Fatalf("%s was blocked for a session that has to re-authenticate: %d", path, code)
		}
		if !*reached {
			t.Fatalf("%s did not reach its handler", path)
		}
	}

	// An ended session is a different matter: there is no way back from it,
	// and neither route may be reached.
	for _, path := range []string{ReauthenticatePath, LogoutPath} {
		next, reached := reachedHandler()
		guard := BindSessions(next, SessionGuardOptions{
			Evaluator: &stubEvaluator{verdict: auth.SessionVerdict{Allow: false, Reason: "revoked"}},
			JWT:       manager,
			Logger:    zap.NewNop(),
		})

		if code := guardedRequest(t, guard, http.MethodPost, path, token).Code; code != http.StatusUnauthorized {
			t.Fatalf("%s was reachable with an ended session: %d", path, code)
		}
		if *reached {
			t.Fatalf("%s ran its handler for an ended session", path)
		}
	}
}

// TestBindSessionsFailsOpenOnAStoreOutageAndSaysSo: a session store that
// cannot be reached must not take the panel down, but the operator has to be
// able to tell that the control is not running.
func TestBindSessionsFailsOpenOnAStoreOutage(t *testing.T) {
	manager, token := testJWT(t)

	next, reached := reachedHandler()
	guard := BindSessions(next, SessionGuardOptions{
		Evaluator: &stubEvaluator{err: context.DeadlineExceeded},
		JWT:       manager,
		Logger:    zap.NewNop(),
	})

	if code := guardedRequest(t, guard, http.MethodGet, "/api/v1/websites", token).Code; code != http.StatusTeapot {
		t.Fatalf("a session store outage refused an authenticated request: %d", code)
	}
	if !*reached {
		t.Fatal("the request did not reach the handler")
	}
}

func TestBindSessionsWithoutAnEvaluatorIsANoOp(t *testing.T) {
	manager, token := testJWT(t)

	next, reached := reachedHandler()
	guard := BindSessions(next, SessionGuardOptions{JWT: manager, Logger: zap.NewNop()})

	if code := guardedRequest(t, guard, http.MethodGet, "/api/v1/websites", token).Code; code != http.StatusTeapot {
		t.Fatalf("an unwired gate refused a request: %d", code)
	}
	if !*reached {
		t.Fatal("an unwired gate swallowed the request")
	}
	if BindSessions(nil, SessionGuardOptions{}) != nil {
		t.Fatal("wrapping nothing produced a handler")
	}
}
