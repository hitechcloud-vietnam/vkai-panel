package handler

// Whether the tamper-evident audit endpoints are REACHABLE.
//
// The rule this repository learned the hard way: a test that builds its own gin
// engine, registers the routes it wants and then proves they answer has proved
// that the routes CAN be mounted. It has said nothing about whether they ARE.
// Four security features shipped that way.
//
// So this file does two separate things and does not confuse them:
//
//   - TestRegisterAuditRoutesMounts and
//     TestAuditChainRoutesCoexistWithTheExistingAuditBlock check the
//     registration function itself, including that it cannot collide with what
//     router.go already mounts. These always run.
//   - TestAuditChainRoutesAreMountedInTheRealRouter checks the engine
//     cmd/api/main.go actually serves. Until one line is added to router.go it
//     SKIPS, and the skip names the exact line. A skip is visible in `go test
//     -v` and in CI output; a passing test that quietly asserted nothing would
//     be the failure this file exists to prevent.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/auth"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/service"
)

// theRegistrationLine is the one line router.go needs. It is a constant here so
// that the report, the doc comment on RegisterAuditRoutes and the test that
// looks for it cannot drift apart.
const theRegistrationLine = "RegisterAuditRoutes(protected, r.auditHandler)"

// auditChainRoutes is every path RegisterAuditRoutes is responsible for.
var auditChainRoutes = []struct {
	method string
	path   string
	why    string
}{
	{"GET", "/api/v1/audit/chain/status", "the cheap dashboard answer: head, range, last verification"},
	{"GET", "/api/v1/audit/chain/verify", "walk the chain and report the first break"},
	{"POST", "/api/v1/audit/chain/verify", "the same, for a caller that wants a body"},
	{"GET", "/api/v1/audit/chain/export", "the bundle an outside auditor verifies"},
	{"GET", "/api/v1/audit/chain/seals", "every prune and every export, permanently"},
	{"GET", "/api/v1/audit/chain/retention", "what a prune would remove, and the operator's command"},
	{"GET", "/api/v1/audit/chain/procedure", "the published verification specification"},
	{"GET", "/api/v1/audit/chain/verifier", "a standalone implementation of it"},
}

func TestRegisterAuditRoutesMounts(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// A handler whose service exists. Registration takes method values and
	// never calls them, so the service needs no database for the route table to
	// be the real one - but it must be non-nil, because a handler without a
	// service deliberately mounts a 503 wildcard instead (see below).
	engine := gin.New()
	RegisterAuditRoutes(engine.Group("/api/v1"),
		NewAuditHandler(service.NewAuditService(nil, zap.NewNop()), zap.NewNop()))

	table := routeTable(engine)
	for _, route := range auditChainRoutes {
		key := route.method + " " + route.path
		if !table[key] {
			t.Errorf("%s is not mounted (%s)", key, route.why)
		}
	}
}

// TestRegisterAuditRoutesAnswers503WhenTheServiceIsMissing.
//
// A panel whose audit service could not be built must still start, and the
// paths must still exist. Mounting nothing would be indistinguishable, from
// outside, from the line in router.go having been forgotten - and that is the
// failure this whole file is about. 404 would read as "this panel has no such
// feature". 503 with a cause is something an operator can act on.
func TestRegisterAuditRoutesAnswers503WhenTheServiceIsMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for name, h := range map[string]*AuditHandler{
		"a nil handler":             nil,
		"a handler with no service": NewAuditHandler(nil, zap.NewNop()),
	} {
		t.Run(name, func(t *testing.T) {
			engine := gin.New()
			engine.Use(gin.Recovery())
			v1 := engine.Group("/api/v1")
			// Past the permission gate, so what is measured is the audit
			// routes' own answer rather than the guard in front of them.
			v1.Use(func(c *gin.Context) {
				c.Set("claims", &auth.TokenClaims{RoleIDs: []string{"super_admin"}})
				c.Next()
			})
			RegisterAuditRoutes(v1, h)

			if len(engine.Routes()) == 0 {
				t.Fatal("nothing was mounted; that is indistinguishable from the " +
					"registration line being missing from router.go")
			}

			w := httptest.NewRecorder()
			engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/audit/chain/status", nil))
			if w.Code != http.StatusServiceUnavailable {
				t.Fatalf("answered %d, want 503: %s", w.Code, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), "unavailable") {
				t.Fatalf("the 503 does not say what is wrong: %s", w.Body.String())
			}
		})
	}
}

// TestAuditChainRoutesCoexistWithTheExistingAuditBlock is the collision test.
//
// router.go already mounts /audit/search, /audit/stats, /audit/:id and
// /audit/cleanup. Adding /audit/chain/... next to a wildcard segment is exactly
// the shape that makes some routers panic at start-up, and a panic at start-up
// on every install is a worse outcome than the feature being missing. So the
// two are registered together here, in the same order router.go would, and both
// are driven to a response.
func TestAuditChainRoutesCoexistWithTheExistingAuditBlock(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	v1 := engine.Group("/api/v1")

	// What router.go has today, standing in for the real handlers.
	existing := v1.Group("/audit")
	existing.GET("/search", func(c *gin.Context) { c.String(http.StatusOK, "search") })
	existing.GET("/stats", func(c *gin.Context) { c.String(http.StatusOK, "stats") })
	existing.GET("/:id", func(c *gin.Context) { c.String(http.StatusOK, "id="+c.Param("id")) })
	existing.POST("/cleanup", func(c *gin.Context) { c.String(http.StatusOK, "cleanup") })

	// The one line. If this panics, the line cannot be added and the report is
	// wrong.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("registering the chain routes alongside the existing audit block panicked: %v\n"+
				"That would take down every install at start-up. The route shape has to change.", r)
		}
	}()
	RegisterAuditRoutes(v1, NewAuditHandler(service.NewAuditService(nil, zap.NewNop()), zap.NewNop()))

	// Both halves resolve, and the static segment wins over the wildcard.
	cases := []struct {
		path string
		want string
	}{
		{"/api/v1/audit/search", "search"},
		{"/api/v1/audit/chain/status", ""},                            // reaches the real handler
		{"/api/v1/audit/3f0ca80e-4f32-4c63-803e-9577fcb88ba6", "id="}, // still the wildcard
	}
	for _, c := range cases {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, c.path, nil)
		engine.ServeHTTP(w, req)

		if w.Code == http.StatusNotFound {
			t.Errorf("%s resolved to nothing", c.path)
			continue
		}
		if c.want != "" && !strings.HasPrefix(w.Body.String(), c.want) {
			t.Errorf("%s was served by the wrong route: body %q", c.path, w.Body.String())
		}
	}
}

// TestAuditChainRoutesAreMountedInTheRealRouter asserts against the engine
// cmd/api/main.go serves, built through the real NewRouter.
//
// It skips, loudly, until router.go carries the registration line. It is not a
// pass: it is the difference between "this feature is wired in" and "this
// feature could be wired in", stated where somebody will see it.
func TestAuditChainRoutesAreMountedInTheRealRouter(t *testing.T) {
	table := routeTable(buildRouter(t, testPolicy()))

	// The shared harness builds NewRouter with nil for every handler that needs
	// a database, so what appears here is the 503 wildcard rather than the eight
	// live paths. Either shape proves the same thing and the only thing this
	// test can prove: RegisterAuditRoutes is called from router.go. Which paths
	// it mounts is TestRegisterAuditRoutesMounts' job, over a real handler.
	mounted := false
	for key := range table {
		if strings.Contains(key, "/api/v1/audit/chain") {
			mounted = true
			break
		}
	}

	if !mounted {
		t.Fatalf("THE AUDIT CHAIN ENDPOINTS ARE NOT MOUNTED.\n\n"+
			"internal/handler/router.go must call, inside the protected group:\n\n"+
			"    %s\n\n"+
			"It is purely additive: the existing `audit := protected.Group(\"/audit\", ...)`\n"+
			"block stays exactly as it is, and this function registers only\n"+
			"/audit/chain/... . Without that line, %d endpoints - including the\n"+
			"tamper verification pass and the auditor's export - are unreachable in\n"+
			"the running panel, however well they are tested here.",
			theRegistrationLine, len(auditChainRoutes))
	}

	// When the harness ever passes a real audit handler, hold the router to
	// every path rather than to the wildcard.
	if table["GET /api/v1/audit/chain/status"] {
		for _, route := range auditChainRoutes {
			key := route.method + " " + route.path
			if !table[key] {
				t.Errorf("%s is missing from the real router (%s)", key, route.why)
			}
		}
	}
}

// TestTheRegistrationLineNamesSomethingThatExists keeps the instruction honest.
// A report that tells somebody to add a line calling a function that has been
// renamed is worse than no report.
func TestTheRegistrationLineNamesSomethingThatExists(t *testing.T) {
	if !strings.HasPrefix(theRegistrationLine, "RegisterAuditRoutes(") {
		t.Fatalf("the documented line no longer calls RegisterAuditRoutes: %q", theRegistrationLine)
	}

	// The function exists with the signature the line uses; this would not
	// compile otherwise.
	var f func(*gin.RouterGroup, *AuditHandler) = RegisterAuditRoutes
	if f == nil {
		t.Fatal("RegisterAuditRoutes is nil")
	}
}
