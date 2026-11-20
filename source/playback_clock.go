package main

import (
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/unit"
	"image"
	"math"
	"strconv"
)

func appendClockTime(b []byte, seconds float64) []byte {
	if seconds < 0 || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		seconds = 0
	}
	n := int64(seconds)
	if n < 0 {
		n = 0
	}
	b = strconv.AppendInt(b, n/60, 10)
	return append(b, ':', byte('0'+n%60/10), byte('0'+n%10))
}

var clockDigits = [10]string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9"}

// Individual fixed-width glyphs keep elapsed-time variants out of the
// shaper's whole-string path cache. Only digits, colon and slash are cached.
func (u *UI) playbackClock(gtx layout.Context, position, total float64, size unit.Sp) layout.Dimensions {
	var storage [64]byte
	value := appendClockTime(storage[:0], position)
	split := len(value)
	value = append(value, ' ', '/', ' ')
	value = appendClockTime(value, total)
	x, y, height, lineHeight, baseline := 0, 0, 0, 0, 0
	em := gtx.Sp(size)
	widthFor := func(c byte) int {
		switch c {
		case ':':
			return max(1, (em*35+99)/100)
		case '/':
			return max(1, (em*45+99)/100)
		case ' ':
			return max(1, em/4)
		}
		return max(1, (em*65+99)/100)
	}
	fullWidth := 0
	for _, c := range value {
		fullWidth += widthFor(c)
	}
	clipStack := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
	defer clipStack.Pop()
	for i, c := range value {
		if i == split && fullWidth > gtx.Constraints.Max.X {
			y += lineHeight + gtx.Dp(1)
			x = 0
		}
		width := max(1, (em*65+99)/100)
		var glyph string
		switch {
		case c >= '0' && c <= '9':
			glyph = clockDigits[c-'0']
		case c == ':':
			glyph = ":"
			width = max(1, (em*35+99)/100)
		case c == '/':
			glyph = "/"
			width = max(1, (em*45+99)/100)
		default:
			x += max(1, em/4)
			continue
		}
		child := gtx
		child.Constraints.Min.X = width
		child.Constraints.Max.X = width
		pushed := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
		d := u.label(child, glyph, size, muted)
		pushed.Pop()
		lineHeight = max(lineHeight, d.Size.Y)
		height = max(height, y+d.Size.Y)
		baseline = d.Baseline
		x += width
	}
	return layout.Dimensions{Size: gtx.Constraints.Constrain(image.Pt(x, height)), Baseline: baseline}
}
