package main

import (
	"fmt"
	"gioui.org/font"
	"gioui.org/font/opentype"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	fontapi "github.com/go-text/typesetting/font"
	"golang.org/x/exp/shiny/materialdesign/icons"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"sync"
)

var fontsOnce sync.Once
var bundledFaces []font.FontFace
var regularCoverage *fontapi.Font

func uiFonts() []font.FontFace {
	fontsOnce.Do(func() {
		for _, ttf := range [][]byte{goregular.TTF, gobold.TTF} {
			faces, e := opentype.ParseCollection(ttf)
			if e != nil {
				panic(fmt.Errorf("parse UI font: %w", e))
			}
			// Font metadata inference calls Go Bold "SemiBold". Register the
			// bundled face under the exact weight requested by the brand label.
			if len(bundledFaces) != 0 {
				for i := range faces {
					faces[i].Font.Weight = font.Bold
				}
			}
			bundledFaces = append(bundledFaces, faces...)
		}
		regularCoverage = bundledFaces[0].Face.Face().Font
	})
	return bundledFaces
}

func newUIShaper(systemFonts bool) *text.Shaper {
	if systemFonts {
		return text.NewShaper(text.WithCollection(uiFonts()))
	}
	return text.NewShaper(text.NoSystemFonts(), text.WithCollection(uiFonts()))
}

// Only the two checkbox icons are needed, and only on the update settings page.
// Material's default theme eagerly constructs unused radio-button icons too.
func newUITheme() *material.Theme {
	return &material.Theme{Shaper: newUIShaper(false), TextSize: unit.Sp(16), FingerSize: unit.Dp(38),
		Palette: material.Palette{Bg: bg, Fg: white, ContrastBg: accent, ContrastFg: bg}}
}

// Load the system font index once, only when displayed text needs fallback.
// Keep a single shaper, so its glyph/layout caches are not duplicated.
func (u *UI) textTheme(s string) *material.Theme {
	if !u.systemFonts {
		uiFonts()
		for _, r := range s {
			if r <= '~' {
				continue
			}
			if _, ok := regularCoverage.NominalGlyph(r); !ok {
				u.systemFonts = true
				u.theme.Shaper = newUIShaper(true)
				break
			}
		}
	}
	return u.theme
}

func (u *UI) ensureCheckboxIcons() {
	if u.theme.Icon.CheckBoxChecked != nil {
		return
	}
	var err error
	u.theme.Icon.CheckBoxChecked, err = widget.NewIcon(icons.ToggleCheckBox)
	if err != nil {
		panic(err)
	}
	u.theme.Icon.CheckBoxUnchecked, err = widget.NewIcon(icons.ToggleCheckBoxOutlineBlank)
	if err != nil {
		panic(err)
	}
}
