// Package web serves the explorer: the page, the model as JSON, a read-only
// source view with Go highlighting, and the SSE stream -watch uses to reload
// the browser.
//
// Every response is derived from one immutable model handed in through Load, so
// handlers never coordinate with each other. Paths are resolved inside Root
// only; a symlink that escapes it is refused.
package web
