package model

import (
	"strings"
)

// filterPackages drops the packages an exclude entry matches.
func filterPackages(list []listed, modulePath string, exclude []string) []listed {
	if len(exclude) == 0 {
		return list
	}
	kept := make([]listed, 0, len(list))
	for _, lp := range list {
		if excluded(lp.Path, modulePath, exclude) {
			continue
		}
		kept = append(kept, lp)
	}
	return kept
}

// excluded reports whether a package path falls under one of the exclude
// entries. Entries are module-relative path prefixes and match whole segments:
// "internal/gen" hides "internal/gen" and "internal/gen/x" but not
// "internal/genx".
func excluded(pkgPath, modulePath string, entries []string) bool {
	if len(entries) == 0 {
		return false
	}
	rel := pkgPath
	if modulePath != "" {
		if pkgPath == modulePath {
			rel = ""
		} else {
			rel = strings.TrimPrefix(pkgPath, modulePath+"/")
			if rel == pkgPath {
				return false // not inside this module
			}
		}
	}
	if rel == "" {
		return false
	}
	for _, e := range entries {
		if e != "" && (rel == e || strings.HasPrefix(rel, e+"/")) {
			return true
		}
	}
	return false
}
