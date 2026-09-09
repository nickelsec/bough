package server

import (
	"strings"
	"testing"
)

// The drawing is moved by one transform and one only.
//
// The svg used to carry a viewBox of the canvas size with xMidYMid meet, which
// fitted and centred the canvas inside the window before the pan and zoom
// transform ran at all. Two scalings stacked: fit() worked out a scale and an
// offset in CSS pixels, and the browser then multiplied both by whatever the
// viewBox mapping needed and shifted them again.
//
// The centring the viewBox did was on the canvas box, which is not the
// drawing. The layout leaves a wide, lopsided margin to pan into, so centring
// the canvas puts the drawing off to one side: on a one day history the middle
// of the tree landed eleven hundred pixels right of the middle of a nineteen
// hundred pixel window, off the screen entirely.
//
// Every layout check in this package works in canvas units and so agreed the
// view was centred while the page showed it jammed against the right hand
// edge. That is the gap this test closes.
func TestOneTransformMovesTheDrawing(t *testing.T) {
	b, err := assets.ReadFile("bough.js")
	if err != nil {
		t.Fatal(err)
	}
	// Comments are stripped first. This file's own explanation of the bug
	// names the attribute it is checking for, and a check that its own
	// reasoning trips is no check at all.
	src := withoutComments(string(b))

	// A viewBox built from the canvas size is the bug itself.
	if strings.Contains(src, `"0 0 " + model.width`) {
		t.Error("the svg fits the canvas to the window, which centres on the canvas box rather than on the drawing")
	}

	// meet is what does the fitting. slice would crop instead; neither belongs
	// on an svg whose coordinates are meant to be CSS pixels.
	if strings.Contains(src, "xMidYMid") {
		t.Error("preserveAspectRatio is fitting the svg, so the transform is no longer the only thing that moves the drawing")
	}

	// And the coordinate system has to be the window, or one unit in the
	// transform stops being one pixel on screen.
	if !strings.Contains(src, "function sizeToStage") {
		t.Error("nothing sizes the svg to the stage, so its units are not CSS pixels")
	}

	// The stage changes width when the record drawer slides open, and the
	// svg's units are measured against that width.
	if !strings.Contains(src, "transitionend") {
		t.Error("nothing follows the drawer, so the drawing stretches while it is open")
	}
}

// withoutComments drops // lines so a check reads the code and not the prose
// around it. Crude, but it only has to handle this one file.
func withoutComments(src string) string {
	var out []string
	for _, line := range strings.Split(src, "\n") {
		if at := strings.Index(line, "//"); at >= 0 {
			line = line[:at]
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}
