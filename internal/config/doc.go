// Package config reads archview.json: which packages to hide, how a source
// link should open, which layers may depend on which, and the CRAP thresholds.
//
// Validation reports every problem in one pass, with the line number of the
// entry at fault, so a broken file does not have to be fixed one error at a
// time. An empty path means "no config file"; callers then get the defaults.
package config
