package backup

// Retention: deciding which generations to delete.
//
// This is the code most likely to destroy data, so it is written to be read.
// Every rule below is a rule about what to KEEP; a generation is deleted only
// because no keep rule claimed it. That direction matters: a bug in a "delete
// when" rule deletes something it should not, and a bug in a "keep when" rule
// keeps something it need not.
//
// Two of the keep rules are unconditional and exist to bound the damage any
// future policy change can do:
//
//	the newest generation is never deleted, whatever the policy says
//	the newest generation that has passed a restore test is never deleted
//
// The second is the one that is easy to leave out. A policy of "keep 7" applied
// to seven consecutive corrupt backups deletes the last good one, and it does
// so silently, at the exact moment the operator most needs it. Keeping the
// newest verified generation costs one archive of disk and removes that.

import (
	"sort"
	"time"
)

// Generation is one stored backup, as retention sees it.
type Generation struct {
	// ID is whatever the caller uses to identify the record; retention passes
	// it back untouched.
	ID string `json:"id"`
	// Key is the object key at the destination.
	Key string `json:"key"`
	// Class is the retention class: website, database, files or config. Each
	// class is pruned independently.
	Class     string    `json:"class"`
	CreatedAt time.Time `json:"created_at"`
	Size      int64     `json:"size"`
	// Verified is true when this generation has been restored into a scratch
	// location and checked. It is the flag that makes the second keep rule
	// possible, and it is why the verification pass writes its result back.
	Verified bool `json:"verified"`
}

// RetentionPolicy is the per-class policy.
type RetentionPolicy struct {
	// KeepGenerations keeps the N newest. 0 means the count rule is not used.
	KeepGenerations int `json:"keep_generations"`
	// KeepDays keeps anything younger than N days. 0 means the age rule is
	// not used.
	KeepDays int `json:"keep_days"`
	// MinKeep is the floor, applied after everything else. It is clamped to at
	// least 1: a policy that can empty a destination is not a policy.
	MinKeep int `json:"min_keep"`
}

// DefaultRetention is the shipped policy for a class.
//
// The numbers differ per class because the questions differ. A website tree is
// large and changes slowly, so generations are the useful unit. A database
// changes constantly and a month-old dump is rarely what anyone wants, so the
// count is higher and the window shorter. The panel's own configuration is a
// few kilobytes and is the thing you need when the machine is gone, so it is
// kept for a quarter and costs nothing.
func DefaultRetention(class string) RetentionPolicy {
	switch class {
	case KindDatabase:
		return RetentionPolicy{KeepGenerations: 14, KeepDays: 14, MinKeep: 2}
	case KindConfig:
		return RetentionPolicy{KeepGenerations: 30, KeepDays: 90, MinKeep: 3}
	case KindWebsite, KindFiles:
		return RetentionPolicy{KeepGenerations: 7, KeepDays: 30, MinKeep: 2}
	default:
		return RetentionPolicy{KeepGenerations: 7, KeepDays: 30, MinKeep: 1}
	}
}

func (p RetentionPolicy) normalised() RetentionPolicy {
	if p.MinKeep < 1 {
		p.MinKeep = 1
	}
	if p.KeepGenerations < 0 {
		p.KeepGenerations = 0
	}
	if p.KeepDays < 0 {
		p.KeepDays = 0
	}
	return p
}

// RetentionDecision is why each generation was kept or expired, so that a
// prune can be explained after the fact rather than only observed.
type RetentionDecision struct {
	Generation Generation `json:"generation"`
	Keep       bool       `json:"keep"`
	Reason     string     `json:"reason"`
}

// SelectExpired applies a policy to the generations of ONE class and returns
// what to keep, what to expire, and why.
//
// The caller is responsible for passing generations of a single class:
// mixing classes would let a busy database's generations push a website's out.
func SelectExpired(generations []Generation, policy RetentionPolicy, now time.Time) (keep, expire []Generation, decisions []RetentionDecision) {
	policy = policy.normalised()

	sorted := make([]Generation, len(generations))
	copy(sorted, generations)
	// Newest first. Ties broken by key so the result is deterministic when two
	// generations share a timestamp - which happens the first time an operator
	// runs a backup twice in the same second while testing.
	sort.Slice(sorted, func(i, j int) bool {
		if !sorted[i].CreatedAt.Equal(sorted[j].CreatedAt) {
			return sorted[i].CreatedAt.After(sorted[j].CreatedAt)
		}
		return sorted[i].Key > sorted[j].Key
	})

	newestVerified := -1
	for i, gen := range sorted {
		if gen.Verified {
			newestVerified = i
			break
		}
	}

	cutoff := time.Time{}
	if policy.KeepDays > 0 {
		cutoff = now.AddDate(0, 0, -policy.KeepDays)
	}

	for i, gen := range sorted {
		reason := ""
		switch {
		case i == 0:
			reason = "newest generation, never deleted"
		case i == newestVerified:
			reason = "newest generation that has passed a restore test, never deleted"
		case i < policy.MinKeep:
			reason = "within the minimum number of generations kept"
		case policy.KeepGenerations > 0 && i < policy.KeepGenerations:
			reason = "within the newest generations kept by policy"
		case policy.KeepDays > 0 && gen.CreatedAt.After(cutoff):
			reason = "younger than the retention window"
		}

		if reason != "" {
			keep = append(keep, gen)
			decisions = append(decisions, RetentionDecision{Generation: gen, Keep: true, Reason: reason})
			continue
		}
		expire = append(expire, gen)
		decisions = append(decisions, RetentionDecision{
			Generation: gen,
			Keep:       false,
			Reason:     "outside every retention rule for this class",
		})
	}
	return keep, expire, decisions
}

// GroupByClass splits generations into their retention classes.
func GroupByClass(generations []Generation) map[string][]Generation {
	out := map[string][]Generation{}
	for _, gen := range generations {
		out[gen.Class] = append(out[gen.Class], gen)
	}
	return out
}
