package main

import (
	"cmp"
	"fmt"
	"slices"
)

// A finding is a line of sizecheck's report, PATH:LINE: msg.
type finding struct {
	path  string
	line  int
	msg   string
	stale bool // a stale baseline entry, which -update drops
}

// compare checks the measured files, and the baseline's entries in scope, against the caps and
// ceilings. It returns the findings, sorted, and the baseline -update writes; create adds to it
// the overages the baseline lacks.
func compare(base baseline, files map[string]measurement, inScope func(string) bool,
	create bool) ([]finding, baseline) {
	var findings []finding
	next := make(baseline)
	for path, ceilings := range base {
		if !inScope(path) {
			next[path] = ceilings
		} else if _, ok := files[path]; !ok {
			findings = append(findings, finding{path: path, line: 1, stale: true,
				msg: "stale baseline entry: the file is gone or git ignores it"})
		}
	}
	for path, m := range files {
		found, ceilings := compareFile(path, m, base[path], create)
		findings = append(findings, found...)
		if ceilings != (values{}) {
			next[path] = ceilings
		}
	}
	slices.SortStableFunc(findings, func(a, b finding) int {
		return cmp.Or(cmp.Compare(a.path, b.path), cmp.Compare(a.line, b.line))
	})
	return findings, next
}

// compareFile checks a file against the caps and its ceilings, and returns the findings and the
// ceilings -update gives it.
func compareFile(path string, m measurement, ceilings values, create bool) ([]finding, values) {
	if m.errMsg != "" {
		return []finding{{path: path, line: m.errLine, msg: "cannot parse: " + m.errMsg}}, ceilings
	}
	var findings []finding
	var next values
	for c := range numChecks {
		value, ceiling, limit := m.values[c], ceilings[c], checks[c].limit
		switch {
		case value <= limit && ceiling > 0:
			findings = append(findings, finding{path: path, line: 1, stale: true,
				msg: fmt.Sprintf("stale baseline entry %s=%d: now %d, within the cap of %d",
					checks[c].key, ceiling, value, limit)})
		case value <= limit:
		case ceiling == 0 && create:
			next[c] = value
		case ceiling == 0:
			findings = append(findings, overCap(path, m, c)...)
		case value > ceiling:
			next[c] = ceiling
			findings = append(findings, overCeiling(path, m, c, ceiling)...)
		default:
			next[c] = value
		}
	}
	return findings, next
}

// overCap reports a file over the cap of c, where the baseline has no ceiling for it.
func overCap(path string, m measurement, c check) []finding {
	switch c {
	case checkLines:
		return []finding{{path: path, line: 1,
			msg: fmt.Sprintf("%d lines, over the cap of %d", m.values[c], checks[c].limit)}}
	case checkPackage:
		return []finding{{path: path, line: m.pkg.line,
			msg: fmt.Sprintf("package comment of %d lines, over the cap of %d",
				m.pkg.length, checks[c].limit)}}
	}
	findings := make([]finding, 0, len(m.blocks))
	for _, b := range m.blocks {
		findings = append(findings, finding{path: path, line: b.line,
			msg: fmt.Sprintf("comment block of %d lines, over the cap of %d", b.length, maxComment)})
	}
	return findings
}

// overCeiling reports a file over the baseline's ceiling for c, and for comments each block over
// the cap, one of which is new.
func overCeiling(path string, m measurement, c check, ceiling int) []finding {
	switch c {
	case checkLines:
		return []finding{{path: path, line: 1,
			msg: fmt.Sprintf("%d lines, above the baseline's ceiling of %d", m.values[c], ceiling)}}
	case checkPackage:
		return []finding{{path: path, line: m.pkg.line,
			msg: fmt.Sprintf("package comment of %d lines, above the baseline's ceiling of %d",
				m.pkg.length, ceiling)}}
	}
	return append([]finding{{path: path, line: 1,
		msg: fmt.Sprintf("%d comment blocks over %d lines, above the baseline's ceiling of %d",
			m.values[c], maxComment, ceiling)}}, overCap(path, m, c)...)
}

// refused returns the findings -update cannot settle: all but the stale entries it drops.
func refused(findings []finding) []finding {
	return slices.DeleteFunc(slices.Clone(findings), func(f finding) bool { return f.stale })
}

// hasStale returns whether a finding is a stale entry.
func hasStale(findings []finding) bool {
	return slices.ContainsFunc(findings, func(f finding) bool { return f.stale })
}
