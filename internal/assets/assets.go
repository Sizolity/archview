// Package assets embeds the browser-side files: the page, its pure helpers and
// the stylesheet used by the source viewer.
package assets

import _ "embed"

//go:embed ui.html
var UI []byte

//go:embed ui.js
var JS []byte

//go:embed source.css
var CSS []byte
