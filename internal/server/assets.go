package server

import "embed"

// assets holds the page and the artwork.
//
// They are compiled into the binary so that bough stays a single file with
// nothing to install beside it, and so the view works with no network at all.
//
//go:embed index.html bough.css bough.js img
var assets embed.FS
