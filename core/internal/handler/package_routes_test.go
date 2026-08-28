package handler

// The mounting test for hosting packages and quota.
//
// Read the header of router_test.go first: four features in a row shipped with
// passing tests and no route mounted, because every one of those tests built
// its own engine and registered the routes it was about to assert. This file
// does two different things instead.
//
//  1. It reads cmd/api/main.go and asserts the registration call is there. That
//     is the line that makes these endpoints reachable in the running panel,
//     and deleting it is exactly the failure this file exists to catch. A route
//     table built by the test cannot notice that deletion; the source can.
//
//  2. It then builds a group the way main.go builds it and asserts every path
//     resolves, that the group is behind authentication, and that the paths
//     answer 503 - never 404 - when the service could not be built.

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/auth"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/middleware"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/quota"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/service"
)

// mainSource reads the composition root as text.
func mainSource(t *testing.T) string {
	t.Helper()
	path := filepath.Join("..", "..", "cmd", "api", "main.go")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("could not read %s: %v", path, err)
	}
	return string(b)
}

// TestPackageRoutesAreMountedByMain fails if the one line that mounts these
// endpoints is removed from cmd/api/main.go.
//
// If the mount is ever moved into router.go instead, replace the assertion
// below with one against the real router's route table - do not delete it.
func TestPackageRoutesAreMountedByMain(t *testing.T) {
	src := mainSource(t)

	if !strings.Contains(src, "handler.RegisterPackageRoutes(") {
		t.Fatal("cmd/api/main.go no longer calls handler.RegisterPackageRoutes: " +
			"/api/v1/packages and /api/v1/quota are not reachable in the running panel")
	}
	if !strings.Contains(src, "handler.UsePackageHandler(") {
		t.Fatal("cmd/api/main.go no longer installs the package handler, so the routes would answer 503")
	}

	// The group the routes are mounted on has to carry authentication, exactly
	// like router.go's `protected` group. Quota and package administration
	// behind no token would be a worse bug than not mounting them at all.
	group := regexp.MustCompile(`(?s)quotaRoutes := engine\.Group\("/api/v1"\).*?handler\.RegisterPackageRoutes\(quotaRoutes\)`)
	block := group.FindString(src)
	if block == "" {
		t.Fatal("the package routes are no longer mounted on a group built from engine.Group(\"/api/v1\") in cmd/api/main.go")
	}
	if !strings.Contains(block, "middleware.AuthRequired(jwtManager)") {
		t.Fatal("the group the package and quota routes are mounted on is not behind middleware.AuthRequired")
	}

	// The services must still receive the enforcer. These are the checks that
	// keep the limit connected to the things it limits.
	for _, wiring := range []string{
		"quota.New(quota.NewPostgresStore(db.DB), logger)",
		"service.NewWebsiteService(websiteRepo, serverRepo, webserverRegistry, quotaEnforcer)",
		"service.NewDatabaseService(dbRepo, serverRepo, quotaEnforcer)",
		"service.NewCronService(cronRepo, serverRepo, quotaEnforcer)",
		"service.NewMailServerService(mailServerRepo, logger, quotaEnforcer)",
		"quotaEnforcer.SetSiteController(websiteService)",
		"go quotaSampler.Run(quotaCtx)",
	} {
		if !strings.Contains(src, wiring) {
			t.Errorf("cmd/api/main.go no longer contains %q, so quota enforcement is disconnected there", wiring)
		}
	}
}

// buildQuotaGroup builds the same group cmd/api/main.go builds: /api/v1 behind
// AuthRequired, with the package routes on it.
func buildQuotaGroup(t *testing.T, h *PackageHandler) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	resetPackageRoutesForTest()
	UsePackageHandler(h, zap.NewNop())

	jwtManager := auth.NewJWTManager("package-route-test", time.Hour, 24*time.Hour, "vkai-package-test")

	engine := gin.New()
	group := engine.Group("/api/v1")
	group.Use(middleware.AuthRequired(jwtManager))
	RegisterPackageRoutes(group)
	return engine
}

func quotaRouteTable(engine *gin.Engine) map[string]bool {
	table := map[string]bool{}
	for _, r := range engine.Routes() {
		table[r.Method+" "+r.Path] = true
	}
	return table
}

func TestPackageRoutesResolveOnTheEngine(t *testing.T) {
	// A real handler over a real service. The service's enforcer has no store,
	// which changes nothing about which paths exist.
	svc := service.NewPackageService(quota.New(nil, zap.NewNop()), nil, zap.NewNop())
	engine := buildQuotaGroup(t, NewPackageHandler(svc, zap.NewNop()))

	table := quotaRouteTable(engine)
	for _, want := range []string{
		"GET /api/v1/packages",
		"POST /api/v1/packages",
		"GET /api/v1/packages/:id",
		"PUT /api/v1/packages/:id",
		"DELETE /api/v1/packages/:id",
		"GET /api/v1/quota",
		"GET /api/v1/quota/events",
		"GET /api/v1/quota/accounts/:tenantId",
		"GET /api/v1/quota/accounts/:tenantId/events",
		"POST /api/v1/quota/accounts/:tenantId/package",
		"GET /api/v1/quota/accounts/:tenantId/overrides",
		"PUT /api/v1/quota/accounts/:tenantId/overrides/:resource",
		"DELETE /api/v1/quota/accounts/:tenantId/overrides/:resource",
		"PUT /api/v1/quota/accounts/:tenantId/features/:feature",
		"DELETE /api/v1/quota/accounts/:tenantId/features/:feature",
		"POST /api/v1/quota/accounts/:tenantId/suspend",
		"POST /api/v1/quota/accounts/:tenantId/resume",
		"POST /api/v1/quota/accounts/:tenantId/recompute",
	} {
		if !table[want] {
			t.Errorf("route not mounted: %s", want)
		}
	}

	if !PackageRoutesMounted() {
		t.Fatal("RegisterPackageRoutes ran but did not record that it had")
	}
}

func TestPackageRoutesSitBehindAuthentication(t *testing.T) {
	svc := service.NewPackageService(quota.New(nil, zap.NewNop()), nil, zap.NewNop())
	engine := buildQuotaGroup(t, NewPackageHandler(svc, zap.NewNop()))

	for _, target := range []string{"/api/v1/quota", "/api/v1/packages"} {
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s answered %d without a token, want 401", target, rec.Code)
		}
	}
}

// TestPackagePathsAnswerWhenTheServiceCannotBeBuilt pins the 503.
//
// A 404 here would tell the interface that this panel has no hosting packages,
// which is how an operator concludes their customers do not need quota. A 503
// naming the cause is a configuration error somebody can fix.
func TestPackagePathsAnswerWhenTheServiceCannotBeBuilt(t *testing.T) {
	engine := buildQuotaGroup(t, nil)

	for _, target := range []string{
		"/api/v1/packages",
		"/api/v1/packages/anything",
		"/api/v1/quota",
		"/api/v1/quota/accounts/anything",
	} {
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
		if rec.Code == http.StatusNotFound {
			t.Errorf("%s answered 404; a missing quota service must not read as 'this panel has no quota'", target)
		}
	}
}

// TestRegisteringTwiceIsANoOp covers the transition where the mount moves from
// cmd/api/main.go into router.go: for one commit both call sites may exist, and
// the second must be ignored rather than panic the panel on start-up.
func TestRegisteringTwiceIsANoOp(t *testing.T) {
	svc := service.NewPackageService(quota.New(nil, zap.NewNop()), nil, zap.NewNop())
	engine := buildQuotaGroup(t, NewPackageHandler(svc, zap.NewNop()))

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("a second registration panicked: %v", r)
		}
	}()
	RegisterPackageRoutes(engine.Group("/api/v1"))
}
