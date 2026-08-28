package handler

// What this file can and cannot prove, stated first so nobody reads more into
// a green run than is there.
//
// It CANNOT prove the offsite backup routes are mounted in the panel, because
// mounting them is one line in router.go and this task was not allowed to edit
// that file. The line is:
//
//	RegisterBackupOffsiteRoutes(protected, r.backupHandler)
//
// What it DOES prove is the two things that are worth proving from here:
//
//  1. Adding that line will not break the panel. gin panics at startup when two
//     routes collide, and this feature adds a second group under a prefix that
//     already has one. TestOffsiteRoutesCoexistWithTheExistingBackupGroup
//     registers the existing eight routes exactly as router.go registers them,
//     then registers these, and asserts the resulting table contains all of
//     both. If the two ever collided, the panel would not start, and this test
//     is where that is found instead of on a customer's machine.
//
//  2. The existing /backups group is still mounted in the REAL router, built
//     through NewRouter the way cmd/api/main.go builds it. That is the
//     assertion router_test.go exists to make, extended to the routes this
//     feature builds on.

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/middleware"
)

// registerExistingBackupRoutes is a copy of the `backups := protected.Group(...)`
// block in Router.Setup. It is duplicated here rather than called, because
// router.go does not expose it; if that block changes and this copy does not,
// the coexistence claim below is about a slightly different table, which is a
// weaker claim rather than a wrong one.
func registerExistingBackupRoutes(rg *gin.RouterGroup, h *BackupHandler) {
	backups := rg.Group("/backups", middleware.RequirePermission("backup"))
	{
		backups.POST("/jobs", h.CreateJob)
		backups.GET("/jobs", h.ListJobs)
		backups.GET("/jobs/:id", h.GetJob)
		backups.PUT("/jobs/:id", h.UpdateJob)
		backups.DELETE("/jobs/:id", h.DeleteJob)
		backups.POST("/jobs/:id/run", h.RunBackup)
		backups.GET("/records", h.ListRecords)
		backups.DELETE("/records/:id", h.DeleteRecord)
	}
}

func TestOffsiteRoutesCoexistWithTheExistingBackupGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	api := engine.Group("/api/v1")

	// A nil handler is enough: registration takes method values, never calls,
	// so the route table is identical to a live one. What is being tested is
	// the table, not the bodies.
	var h *BackupHandler = NewBackupHandler(nil)

	// The order router.go would use: the existing block, then the new one.
	// gin panics on a conflict, so reaching the assertions is already half the
	// result.
	registerExistingBackupRoutes(api, h)
	RegisterBackupOffsiteRoutes(api, h)

	table := routeTable(engine)

	existing := []struct{ method, path string }{
		{http.MethodPost, "/api/v1/backups/jobs"},
		{http.MethodGet, "/api/v1/backups/jobs"},
		{http.MethodGet, "/api/v1/backups/jobs/:id"},
		{http.MethodPut, "/api/v1/backups/jobs/:id"},
		{http.MethodDelete, "/api/v1/backups/jobs/:id"},
		{http.MethodPost, "/api/v1/backups/jobs/:id/run"},
		{http.MethodGet, "/api/v1/backups/records"},
		{http.MethodDelete, "/api/v1/backups/records/:id"},
	}
	for _, route := range existing {
		if !table[route.method+" "+route.path] {
			t.Fatalf("registering the offsite routes displaced the existing route %s %s", route.method, route.path)
		}
	}

	added := []struct {
		method, path, why string
	}{
		{http.MethodPost, "/api/v1/backups/destinations",
			"there is nowhere to send a backup until a destination can be created"},
		{http.MethodGet, "/api/v1/backups/destinations",
			"an operator has to be able to see where backups are going"},
		{http.MethodGet, "/api/v1/backups/destinations/:id", "one destination"},
		{http.MethodDelete, "/api/v1/backups/destinations/:id", "removing one"},
		{http.MethodPost, "/api/v1/backups/destinations/:id/probe",
			"listing a bucket succeeds without write permission; this is the call that actually writes"},

		{http.MethodPut, "/api/v1/backups/jobs/:id/offsite",
			"a job with no destination and no retention never leaves the machine"},
		{http.MethodGet, "/api/v1/backups/jobs/:id/offsite", "reading those settings back"},
		{http.MethodPost, "/api/v1/backups/jobs/:id/offsite/run",
			"this is the call that takes an encrypted backup to a destination"},
		{http.MethodPost, "/api/v1/backups/jobs/:id/offsite/retention",
			"pruning on demand, not only after a backup"},

		{http.MethodGet, "/api/v1/backups/artifacts", "what archives exist"},
		{http.MethodGet, "/api/v1/backups/artifacts/:id", "one archive"},
		{http.MethodPost, "/api/v1/backups/artifacts/:id/verify",
			"the whole point: prove this archive restores"},
		{http.MethodGet, "/api/v1/backups/artifacts/:id/verifications",
			"the history of that proof for one archive"},
		{http.MethodGet, "/api/v1/backups/verifications", "the history across the tenant"},
		{http.MethodPost, "/api/v1/backups/verifications/run-due",
			"what the scheduled restorability pass calls"},
		{http.MethodGet, "/api/v1/backups/health",
			"how much of what we store has been proved to restore"},

		{http.MethodPost, "/api/v1/backups/restores",
			"restore in one action, defaulting to a dry run"},
		{http.MethodGet, "/api/v1/backups/restores", "what has been restored"},
		{http.MethodGet, "/api/v1/backups/restores/:id", "one restore and its plan"},

		{http.MethodGet, "/api/v1/backups/operations", "what is running now"},
		{http.MethodGet, "/api/v1/backups/operations/:id", "progress of one operation"},
		{http.MethodPost, "/api/v1/backups/operations/:id/cancel",
			"a backup that cannot be stopped is a backup that fills the disk"},
	}
	for _, route := range added {
		if !table[route.method+" "+route.path] {
			t.Fatalf("%s %s is not registered: %s", route.method, route.path, route.why)
		}
	}
}

// TestRegisterBackupOffsiteRoutesIsSafeWithNoHandler covers the degraded panel:
// a build where the backup handler was never constructed must not panic at
// startup, it must simply have no backup routes.
func TestRegisterBackupOffsiteRoutesIsSafeWithNoHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	RegisterBackupOffsiteRoutes(engine.Group("/api/v1"), nil)

	for path := range routeTable(engine) {
		t.Fatalf("a nil handler still registered %s", path)
	}
}

// TestExistingBackupRoutesAreStillMountedInTheRealRouter asserts through the
// engine cmd/api/main.go serves, not through one this test built. The offsite
// routes cannot be asserted here until router.go carries the registration
// line; these are the routes they extend.
func TestExistingBackupRoutesAreStillMountedInTheRealRouter(t *testing.T) {
	table := routeTable(buildRouter(t, testPolicy()))

	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/backups/jobs"},
		{http.MethodPost, "/api/v1/backups/jobs/:id/run"},
		{http.MethodGet, "/api/v1/backups/records"},
	} {
		if !table[route.method+" "+route.path] {
			t.Fatalf("%s %s is not mounted in the real router", route.method, route.path)
		}
	}
}
