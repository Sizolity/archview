// Package crap adds change-risk scores to the model by running go-crap once
// and mapping its report onto the model's functions and methods.
//
// go-crap reports `file:line` positions and a coverage percentage (0–100, not
// a fraction); the mapping normalises the path, looks the function up by its
// declaration line, and attaches cyclomatic complexity, coverage and CRAP to
// it. A missing or failing tool is a warning on the model, never a failed scan.
package crap
