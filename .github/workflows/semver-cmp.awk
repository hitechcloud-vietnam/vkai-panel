# Semantic version ordering for the release workflows.
#
# Usage:  awk -v a=1.2.3 -v b=1.10.0 -f semver-cmp.awk
# Prints: -1 when a < b, 0 when they are equal, 1 when a > b.
#
# This exists because the obvious alternatives are wrong in ways that would let a
# release out: "sort -V" orders 1.0.0-rc.1 AFTER 1.0.0, and plain string
# comparison orders 1.9.0 after 1.10.0. Build metadata (+...) is ignored, as
# SemVer 2.0.0 requires. GitHub ignores files in .github/workflows that are not
# YAML, so this helper can live next to the workflows that use it.

function strip_build(s,   plus) {
	plus = index(s, "+")
	if (plus) s = substr(s, 1, plus - 1)
	return s
}

# Fills core[1..3] with MAJOR, MINOR, PATCH and returns the pre-release string.
function split_version(s, core,   dash, pre) {
	s = strip_build(s)
	sub(/^v/, "", s)
	dash = index(s, "-")
	if (dash) {
		pre = substr(s, dash + 1)
		s = substr(s, 1, dash - 1)
	} else {
		pre = ""
	}
	split(s, core, ".")
	return pre
}

function semver_cmp(x, y,   xc, yc, xp, yp, i, n, m, xi, yi, xnum, ynum) {
	xp = split_version(x, xc)
	yp = split_version(y, yc)

	for (i = 1; i <= 3; i++) {
		if (xc[i] + 0 < yc[i] + 0) return -1
		if (xc[i] + 0 > yc[i] + 0) return 1
	}

	# A version with a pre-release is older than the same version without one.
	if (xp == "" && yp != "") return 1
	if (xp != "" && yp == "") return -1
	if (xp == yp) return 0

	n = split(xp, xi, ".")
	m = split(yp, yi, ".")
	for (i = 1; i <= (n < m ? n : m); i++) {
		xnum = (xi[i] ~ /^[0-9]+$/)
		ynum = (yi[i] ~ /^[0-9]+$/)
		if (xnum && ynum) {
			if (xi[i] + 0 < yi[i] + 0) return -1
			if (xi[i] + 0 > yi[i] + 0) return 1
		} else if (xnum && !ynum) {
			return -1
		} else if (!xnum && ynum) {
			return 1
		} else {
			if (xi[i] < yi[i]) return -1
			if (xi[i] > yi[i]) return 1
		}
	}
	if (n < m) return -1
	if (n > m) return 1
	return 0
}

BEGIN { print semver_cmp(a, b) }
