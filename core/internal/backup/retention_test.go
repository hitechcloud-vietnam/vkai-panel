package backup

import (
	"fmt"
	"testing"
	"time"
)

func gens(t *testing.T, class string, ages ...int) []Generation {
	t.Helper()
	now := referenceNow()
	out := make([]Generation, 0, len(ages))
	for _, days := range ages {
		out = append(out, Generation{
			ID:        fmt.Sprintf("%s-%dd", class, days),
			Key:       fmt.Sprintf("tenant/%s/res/%03d", class, days),
			Class:     class,
			CreatedAt: now.AddDate(0, 0, -days),
			Size:      1000,
		})
	}
	return out
}

func referenceNow() time.Time {
	return time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
}

func ids(generations []Generation) []string {
	out := make([]string, 0, len(generations))
	for _, g := range generations {
		out = append(out, g.ID)
	}
	return out
}

func TestRetentionKeepsTheNewestGenerations(t *testing.T) {
	// Ten daily generations, policy keeps three by count and none by age.
	all := gens(t, KindWebsite, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9)
	keep, expire, _ := SelectExpired(all, RetentionPolicy{KeepGenerations: 3, MinKeep: 1}, referenceNow())

	if len(keep) != 3 {
		t.Fatalf("kept %v, want 3 generations", ids(keep))
	}
	if len(expire) != 7 {
		t.Fatalf("expired %v, want 7 generations", ids(expire))
	}
	for _, want := range []string{"website-0d", "website-1d", "website-2d"} {
		if !containsID(keep, want) {
			t.Fatalf("the newest generations were not kept: %v", ids(keep))
		}
	}
	for _, mustGo := range []string{"website-3d", "website-9d"} {
		if !containsID(expire, mustGo) {
			t.Fatalf("an old generation was not expired: %v", ids(expire))
		}
	}
}

// The rule that matters most: whatever the policy says, the newest generation
// survives. A policy of zero applied to a live destination is the shape of an
// operator typing a number in the wrong box.
func TestRetentionNeverDeletesTheNewest(t *testing.T) {
	all := gens(t, KindDatabase, 0, 1, 2, 3)

	for _, policy := range []RetentionPolicy{
		{KeepGenerations: 0, KeepDays: 0, MinKeep: 0},
		{KeepGenerations: -5, KeepDays: -5, MinKeep: -5},
		{KeepGenerations: 1, KeepDays: 0, MinKeep: 1},
	} {
		keep, expire, decisions := SelectExpired(all, policy, referenceNow())
		if len(keep) == 0 {
			t.Fatalf("policy %+v emptied the destination", policy)
		}
		if !containsID(keep, "database-0d") {
			t.Fatalf("policy %+v deleted the newest generation", policy)
		}
		if containsID(expire, "database-0d") {
			t.Fatalf("policy %+v expired the newest generation", policy)
		}
		if decisions[0].Reason != "newest generation, never deleted" {
			t.Fatalf("the newest generation was kept for the wrong reason: %q", decisions[0].Reason)
		}
	}
}

// The other unconditional rule: the newest generation that has actually been
// restored is kept even when it is outside every policy window. Seven bad
// backups must not be able to push the last good one out.
func TestRetentionNeverDeletesTheNewestVerifiedGeneration(t *testing.T) {
	all := gens(t, KindWebsite, 0, 1, 2, 3, 4, 5, 6, 7)
	// Only the eight-day-old generation has ever been proved restorable.
	for i := range all {
		if all[i].ID == "website-7d" {
			all[i].Verified = true
		}
	}

	keep, expire, decisions := SelectExpired(all, RetentionPolicy{KeepGenerations: 2, MinKeep: 1}, referenceNow())

	if !containsID(keep, "website-7d") {
		t.Fatalf("the last generation known to restore was deleted; kept %v", ids(keep))
	}
	if containsID(expire, "website-7d") {
		t.Fatal("the last verified generation was expired")
	}
	var reason string
	for _, d := range decisions {
		if d.Generation.ID == "website-7d" {
			reason = d.Reason
		}
	}
	if reason != "newest generation that has passed a restore test, never deleted" {
		t.Fatalf("kept for the wrong reason: %q", reason)
	}
}

func TestRetentionKeepsByAge(t *testing.T) {
	all := gens(t, KindConfig, 0, 10, 40, 100, 200)
	keep, expire, _ := SelectExpired(all, RetentionPolicy{KeepDays: 90, MinKeep: 1}, referenceNow())

	for _, want := range []string{"config-0d", "config-10d", "config-40d"} {
		if !containsID(keep, want) {
			t.Fatalf("a generation inside the 90 day window was expired: kept %v", ids(keep))
		}
	}
	for _, want := range []string{"config-100d", "config-200d"} {
		if !containsID(expire, want) {
			t.Fatalf("a generation outside the 90 day window was kept: expired %v", ids(expire))
		}
	}
}

func TestRetentionMinKeepIsAFloor(t *testing.T) {
	all := gens(t, KindDatabase, 0, 100, 200, 300)
	// Every rule would drop everything but the newest; MinKeep holds three.
	keep, _, _ := SelectExpired(all, RetentionPolicy{KeepGenerations: 1, KeepDays: 1, MinKeep: 3}, referenceNow())
	if len(keep) != 3 {
		t.Fatalf("MinKeep 3 kept %v", ids(keep))
	}
}

func TestRetentionIsPerClass(t *testing.T) {
	// A busy database must not push a website's generations out. The classes
	// are pruned separately and each keeps its own newest.
	all := append(gens(t, KindDatabase, 0, 1, 2, 3, 4, 5), gens(t, KindWebsite, 20, 40)...)
	grouped := GroupByClass(all)

	if len(grouped) != 2 {
		t.Fatalf("grouping produced %d classes", len(grouped))
	}
	dbKeep, _, _ := SelectExpired(grouped[KindDatabase], DefaultRetention(KindDatabase), referenceNow())
	siteKeep, siteExpire, _ := SelectExpired(grouped[KindWebsite], RetentionPolicy{KeepGenerations: 1, KeepDays: 0, MinKeep: 1}, referenceNow())

	if len(dbKeep) != 6 {
		t.Fatalf("the database class kept %v", ids(dbKeep))
	}
	if len(siteKeep) != 1 || siteKeep[0].ID != "website-20d" {
		t.Fatalf("the website class kept %v", ids(siteKeep))
	}
	if len(siteExpire) != 1 || siteExpire[0].ID != "website-40d" {
		t.Fatalf("the website class expired %v", ids(siteExpire))
	}
}

func TestRetentionOnAnEmptyDestination(t *testing.T) {
	keep, expire, decisions := SelectExpired(nil, DefaultRetention(KindWebsite), referenceNow())
	if len(keep) != 0 || len(expire) != 0 || len(decisions) != 0 {
		t.Fatal("retention invented generations out of an empty list")
	}
}

func TestRetentionIsDeterministicForSimultaneousGenerations(t *testing.T) {
	now := referenceNow()
	all := []Generation{
		{ID: "a", Key: "tenant/files/res/a", Class: KindFiles, CreatedAt: now},
		{ID: "b", Key: "tenant/files/res/b", Class: KindFiles, CreatedAt: now},
		{ID: "c", Key: "tenant/files/res/c", Class: KindFiles, CreatedAt: now},
	}
	first, _, _ := SelectExpired(all, RetentionPolicy{KeepGenerations: 1, MinKeep: 1}, now)
	for i := 0; i < 20; i++ {
		again, _, _ := SelectExpired(all, RetentionPolicy{KeepGenerations: 1, MinKeep: 1}, now)
		if len(again) != len(first) || again[0].ID != first[0].ID {
			t.Fatal("retention is not deterministic when generations share a timestamp")
		}
	}
}

func TestDefaultRetentionDiffersPerClass(t *testing.T) {
	site := DefaultRetention(KindWebsite)
	db := DefaultRetention(KindDatabase)
	cfg := DefaultRetention(KindConfig)

	if db.KeepGenerations <= site.KeepGenerations {
		t.Fatal("a database is expected to keep more generations than a website tree")
	}
	if cfg.KeepDays <= site.KeepDays {
		t.Fatal("the panel configuration is expected to be kept longer than a website tree")
	}
	for _, p := range []RetentionPolicy{site, db, cfg} {
		if p.MinKeep < 1 {
			t.Fatalf("a shipped policy has MinKeep %d", p.MinKeep)
		}
	}
}

func containsID(generations []Generation, id string) bool {
	for _, g := range generations {
		if g.ID == id {
			return true
		}
	}
	return false
}
