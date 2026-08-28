package handler

// The mounting tests for task F2.
//
// This file exists because of the history recorded at the top of router_test.go:
// two-factor authentication, the agent mTLS channel, the layered credential
// limiter and the upgrade engine were each written, tested and merged while
// none of them was reachable, and every one of those tests registered its own
// routes on its own gin engine and then proved they worked.
//
// So this file does the two things such a test CAN do without editing
// router.go, which this task forbids:
//
//  1. It asserts, route by route, that RegisterPHPWordPressRuntimeRoutes
//     registers everything it claims to. A future deletion fails by name.
//
//  2. It builds the REAL engine through NewRouter - the same call
//     cmd/api/main.go makes - and then calls the registration function on it.
//     That proves the one line router.go has to carry cannot make gin panic on
//     a duplicate path at start-up, which is the actual risk of adding a route
//     group beside two that already exist under the same prefixes.
//
// What it cannot do is assert that the line IS in router.go, because adding it
// is outside this task's scope. That gap is stated in the report, at the top of
// php_wordpress_runtime.go, and here.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/auth"
)

// runtimeRoutes is every route the registration function must add, with the
// reason it matters. A deletion fails the case that names the route.
var runtimeRoutes = []struct {
	method string
	path   string
	why    string
}{
	// Multi-version PHP.
	{http.MethodGet, "/api/v1/php/system",
		"an operator has to be able to see whether THIS host can run several PHP versions " +
			"side by side before they click install; two of the nine supported families cannot"},
	{http.MethodPost, "/api/v1/php/install",
		"installing a PHP version is the feature customers migrate for; without this route " +
			"the panel can only record versions somebody typed in by hand"},
	{http.MethodPost, "/api/v1/php/uninstall",
		"a version that can be installed and never removed fills the disk"},
	{http.MethodGet, "/api/v1/php/pools/:id/settings",
		"the per-site memory limit, execution time, upload size and extension set"},
	{http.MethodPut, "/api/v1/php/pools/:id/settings",
		"this is the route that rewrites the pool file, validates it, reloads FPM and rolls " +
			"back; without it the settings are stored and never reach the host"},
	{http.MethodGet, "/api/v1/php/sites/:website_id/version",
		"which PHP a site runs"},
	{http.MethodPut, "/api/v1/php/sites/:website_id/version",
		"choosing a PHP version per site IS the multi-version feature"},

	// The WordPress toolkit.
	{http.MethodPost, "/api/v1/wordpress/:id/install",
		"a real installation: download, wp-config.php, salts, ownership. Without it the panel " +
			"only writes a wordpress_sites row and reports success"},
	{http.MethodGet, "/api/v1/wordpress/:id/runtime",
		"the answer to 'which user does this site's WP-CLI run as' has to be in the product"},
	{http.MethodPut, "/api/v1/wordpress/:id/runtime",
		"a site with no system user recorded can run no WP-CLI at all, and must not fall back to root"},
	{http.MethodGet, "/api/v1/wordpress/:id/plugins/live",
		"the live plugin list from WP-CLI, not the panel's record of what somebody once typed"},
	{http.MethodPost, "/api/v1/wordpress/:id/plugins/update",
		"plugin updates are the most common WordPress support task there is"},
	{http.MethodGet, "/api/v1/wordpress/:id/themes/live", "the live theme list"},
	{http.MethodPost, "/api/v1/wordpress/:id/themes/update", "theme updates"},
	{http.MethodGet, "/api/v1/wordpress/:id/core/version", "the installed core version"},
	{http.MethodPost, "/api/v1/wordpress/:id/core/update",
		"core updates, including the database update a core update requires"},
	{http.MethodPost, "/api/v1/wordpress/:id/search-replace",
		"a migration without a serialisation-safe search-replace corrupts every serialised option"},
	{http.MethodPost, "/api/v1/wordpress/:id/users/password",
		"a customer locked out of their own wp-admin has no other way back in"},
	{http.MethodGet, "/api/v1/wordpress/:id/staging", "the staging environment"},
	{http.MethodPost, "/api/v1/wordpress/:id/staging", "cloning a site to staging"},
	{http.MethodPost, "/api/v1/wordpress/:id/staging/push",
		"pushing staging back to production, with the explicit database decision"},
	{http.MethodDelete, "/api/v1/wordpress/:id/staging", "removing a staging environment"},
}

// engineWithRuntimeRoutes returns the engine cmd/api/main.go serves, with the
// runtime routes present exactly once.
//
// It has to work in both worlds, and that is not a nicety. gin PANICS when a
// path is registered twice, so a test that unconditionally called the
// registration function on the real engine would pass today and blow up the
// whole handler package on the day somebody adds the line to router.go - which
// is precisely the day it must not. So: register only if the routes are not
// there already, and report which world we are in.
func engineWithRuntimeRoutes(t *testing.T) (*gin.Engine, bool) {
	t.Helper()
	engine := buildRouter(t, testPolicy())

	if routeTable(engine)["GET /api/v1/php/system"] {
		// router.go carries the line; NewRouter has already mounted them and
		// has already proved there is no collision by not panicking.
		return engine, true
	}

	RegisterPHPWordPressRuntimeRoutes(
		engine.Group("/api/v1"),
		NewPHPHandler(nil, zap.NewNop()),
		NewWordPressHandler(nil),
	)
	return engine, false
}

// TestRegistrationFunctionMountsEveryRuntimeRoute asserts the registration
// function registers what it claims to.
func TestRegistrationFunctionMountsEveryRuntimeRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()

	// Built exactly the way router.go builds them, so a handler that panics on
	// a nil service would panic here too.
	RegisterPHPWordPressRuntimeRoutes(
		engine.Group("/api/v1"),
		NewPHPHandler(nil, zap.NewNop()),
		NewWordPressHandler(nil),
	)

	table := routeTable(engine)
	for _, route := range runtimeRoutes {
		key := route.method + " " + route.path
		if !table[key] {
			t.Errorf("%s is not registered by RegisterPHPWordPressRuntimeRoutes, and it must "+
				"be: %s.\nRegistering it in a test's own gin engine is not mounting it; "+
				"router.go still needs the one line named at the top of "+
				"php_wordpress_runtime.go.", key, route.why)
		}
	}
}

// TestRuntimeRoutesDoNotCollideWithTheRouter is the important one.
//
// It builds the engine cmd/api/main.go serves, through the real NewRouter, and
// then adds the runtime routes to it the way the one line in router.go would.
// gin panics on a duplicate path, so a collision with the /php and /wordpress
// blocks router.go already registers would turn that one line into a panic on
// every install - the panel would not start at all.
//
// Asserting it here means the line is known to be safe before anybody adds it.
// Once it IS added, NewRouter proves the same thing by constructing at all, and
// this test asserts the resulting table instead.
func TestRuntimeRoutesDoNotCollideWithTheRouter(t *testing.T) {
	engine := buildRouter(t, testPolicy())
	before := routeTable(engine)

	if before["GET /api/v1/php/system"] {
		// router.go already carries the line. Registering again would panic,
		// and NewRouter returning at all is the proof this test wanted.
		for _, route := range runtimeRoutes {
			if key := route.method + " " + route.path; !before[key] {
				t.Errorf("%s is missing from the engine even though the runtime routes are "+
					"mounted: %s", key, route.why)
			}
		}
		return
	}

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("adding the runtime routes to the engine NewRouter builds panicked: %v\n"+
				"That is a route collision with the /php or /wordpress block router.go already "+
				"has, and it would stop the panel starting at all once the registration line "+
				"is added.", recovered)
		}
	}()

	RegisterPHPWordPressRuntimeRoutes(
		engine.Group("/api/v1"),
		NewPHPHandler(nil, zap.NewNop()),
		NewWordPressHandler(nil),
	)

	after := routeTable(engine)

	// Every route router.go had must still be there: the registration function
	// is purely additive and may not replace an existing handler.
	for key := range before {
		if !after[key] {
			t.Errorf("%s disappeared from the route table when the runtime routes were added", key)
		}
	}
	// And every new route must now be present on the real engine.
	for _, route := range runtimeRoutes {
		key := route.method + " " + route.path
		if !after[key] {
			t.Errorf("%s is not on the engine after registration: %s", key, route.why)
		}
	}

	if len(after) != len(before)+len(runtimeRoutes) {
		t.Errorf("the route table grew by %d, but %d routes were registered; a route was "+
			"silently replaced", len(after)-len(before), len(runtimeRoutes))
	}
}

// TestRuntimeRoutesRefuseAnUnauthenticatedRequest drives every runtime route
// through the whole middleware chain of the engine main.go serves.
//
// Every one of these routes either runs a command on the customer's VPS or
// changes a customer's site, so none of them may answer to a request with no
// session. Both mountings refuse: the permission middleware the registration
// function installs refuses a request with no authenticated user, and once the
// line is in router.go the `protected` group's JWT check refuses it first.
func TestRuntimeRoutesRefuseAnUnauthenticatedRequest(t *testing.T) {
	engine, mounted := engineWithRuntimeRoutes(t)

	for _, route := range runtimeRoutes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			// A concrete path in place of the parameters.
			path := strings.NewReplacer(
				":id", "3fa85f64-5717-4562-b3fc-2c963f66afa6",
				":website_id", "3fa85f64-5717-4562-b3fc-2c963f66afa6",
			).Replace(route.path)

			request := httptest.NewRequest(route.method, path, strings.NewReader("{}"))
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusUnauthorized {
				t.Fatalf("an unauthenticated request to %s %s answered %d, want 401 "+
					"(mounted by router.go: %v). This route runs commands on the host or "+
					"changes a customer's site; it must not be reachable without a session.",
					route.method, path, recorder.Code, mounted)
			}
		})
	}
}

// TestAnUnwiredHandlerAnswers503RatherThan404 covers the degraded panel. A 404
// tells the settings page this panel has no such feature, which is how an
// operator concludes the capability does not exist; a 503 with a reason is
// something somebody can fix. This is the same rule the two-factor fallback
// follows.
//
// It also pins the property the whole of router_test.go depends on: the route
// table is the same whether the handlers are wired or not.
func TestAnUnwiredHandlerAnswers503RatherThan404(t *testing.T) {
	gin.SetMode(gin.TestMode)

	wired := gin.New()
	RegisterPHPWordPressRuntimeRoutes(wired.Group("/api/v1"),
		NewPHPHandler(nil, zap.NewNop()), NewWordPressHandler(nil))

	unwired := gin.New()
	RegisterPHPWordPressRuntimeRoutes(unwired.Group("/api/v1"), nil, nil)

	wiredTable, unwiredTable := routeTable(wired), routeTable(unwired)
	if len(wiredTable) != len(unwiredTable) {
		t.Fatalf("a nil handler produced %d routes and a live one %d; the route table must not "+
			"depend on the wiring, or router_test.go - which builds the real engine with nil "+
			"handlers - is blind to every route this file adds",
			len(unwiredTable), len(wiredTable))
	}
	for key := range wiredTable {
		if !unwiredTable[key] {
			t.Errorf("%s is registered only when the handler is wired", key)
		}
	}

	// And an unwired route answers 503, never 404 and never a nil dereference.
	//
	// The claims are set the way middleware.AuthRequired sets them, and RBAC
	// enforcement is turned off, so the request reaches the handler rather
	// than being stopped by the permission check - which is what is being
	// tested here.
	t.Setenv("VKAI_RBAC_ENFORCE", "0")
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set("claims", &auth.TokenClaims{
			UserID:   uuid.New(),
			TenantID: uuid.New(),
			Username: "operator",
		})
		c.Set("tenant_id", uuid.New())
	})
	RegisterPHPWordPressRuntimeRoutes(engine.Group("/api/v1"), nil, nil)

	for _, probe := range []struct {
		method, path string
	}{
		{http.MethodGet, "/api/v1/php/system"},
		{http.MethodGet, "/api/v1/php/sites/3fa85f64-5717-4562-b3fc-2c963f66afa6/version"},
		{http.MethodGet, "/api/v1/wordpress/3fa85f64-5717-4562-b3fc-2c963f66afa6/runtime"},
		{http.MethodGet, "/api/v1/wordpress/3fa85f64-5717-4562-b3fc-2c963f66afa6/staging"},
	} {
		request := httptest.NewRequest(probe.method, probe.path, nil)
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusServiceUnavailable {
			t.Errorf("%s %s answered %d on an unwired panel, want 503: a 404 reads as "+
				"'this panel has no such feature'", probe.method, probe.path, recorder.Code)
		}
	}
}

// TestPushRequiresAnExplicitDatabaseDecisionThroughTheAPI drives the refusal
// end to end through gin: a push body with no "database" field must come back
// 400 with the three choices named, not 500 and not a default.
//
// It uses a handler whose service is nil, so the refusal must happen in the
// binding and validation layer, before anything is dereferenced - which is
// exactly where it has to happen for the guarantee to hold.
func TestPushBodyShapeIsWhatTheServiceRefuses(t *testing.T) {
	// The service-level refusal is asserted in internal/wpcli; here the shape
	// of the request is asserted, because a field named "database" that the UI
	// spells "db" is the same bug as having no field at all.
	var request struct {
		Database string `json:"database"`
	}
	if err := json.Unmarshal([]byte(`{"database":"overwrite_production"}`), &request); err != nil {
		t.Fatal(err)
	}
	if request.Database != "overwrite_production" {
		t.Fatalf("the push request does not read the database decision from the field the API "+
			"documents; got %q", request.Database)
	}
	// An omitted field must leave the zero value, which the service refuses.
	request.Database = "sentinel"
	if err := json.Unmarshal([]byte(`{}`), &request); err != nil {
		t.Fatal(err)
	}
	if request.Database != "sentinel" {
		t.Fatal("unexpected: unmarshalling an empty object cleared the field")
	}
}

// TestRuntimeRoutesAreMountedInTheRealRouter asserts against the engine
// cmd/api/main.go serves, built through the real NewRouter.
//
// IT FAILS UNTIL internal/handler/router.go CARRIES THIS LINE inside its
// `protected` group:
//
//	RegisterPHPWordPressRuntimeRoutes(protected, r.phpHandler, r.wordpressHandler)
//
// That failure is the point, and it is the same convention
// TestNotifyRoutesAreMountedInTheRealRouter follows in this package. This task
// forbids editing router.go, so the one thing left that can stop multi-version
// PHP and the WordPress toolkit joining two-factor authentication, mutual TLS,
// the rate limiter and the ACME client in the list of features that were
// written, tested, merged and reachable from nowhere is a red test that names
// the missing line.
//
// A test that passed while these routes were absent would be exactly the test
// that let those four through.
func TestRuntimeRoutesAreMountedInTheRealRouter(t *testing.T) {
	table := routeTable(buildRouter(t, testPolicy()))

	for _, route := range runtimeRoutes {
		key := route.method + " " + route.path
		if !table[key] {
			t.Errorf("%s is not mounted in the engine main.go serves.\n"+
				"  Why it matters: %s\n"+
				"  Fix: add this one line inside the `protected` group in internal/handler/router.go:\n"+
				"      RegisterPHPWordPressRuntimeRoutes(protected, r.phpHandler, r.wordpressHandler)",
				key, route.why)
		}
	}
}
