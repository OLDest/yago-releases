package main

import (
	"gioui.org/font"
	"testing"
)

func TestBundledFontCoverage(t *testing.T) {
	for _, face := range uiFonts() {
		if face.Font.Weight != font.Normal && face.Font.Weight != font.Bold {
			t.Fatal("unused weight", face.Font)
		}
		for _, r := range "YaGo Моя волна Ёё № ♥ ↓ ↑ ← → … − −15 с HQ Basic × · [x]" {
			if _, ok := face.Face.Face().Font.NominalGlyph(r); !ok {
				t.Fatalf("missing UI glyph %q in %v", r, face.Font)
			}
		}
	}
}

func TestFontFallbackLoadsOnlyWhenNeeded(t *testing.T) {
	u := &UI{theme: newUITheme()}
	original := u.theme.Shaper
	u.textTheme("YaGo Моя волна Ёё ♥ ↓ …")
	if u.systemFonts || u.theme.Shaper != original {
		t.Fatal("basic UI loaded system fonts")
	}
	u.textTheme("東京 音楽")
	fallback := u.theme.Shaper
	if !u.systemFonts || fallback == original {
		t.Fatal("multilingual fallback was not enabled")
	}
	u.textTheme("한국어 العربية 🎵")
	if u.theme.Shaper != fallback {
		t.Fatal("fallback shaper recreated")
	}
	u.releaseLibraryView()
	if !u.systemFonts {
		t.Fatal("library cleanup lost fallback policy")
	}
}

func TestOnlyUsedThemeIcons(t *testing.T) {
	u := &UI{theme: newUITheme()}
	if u.theme.Icon.CheckBoxChecked != nil || u.theme.Icon.RadioChecked != nil {
		t.Fatal("icons created eagerly")
	}
	u.ensureCheckboxIcons()
	checked, unchecked := u.theme.Icon.CheckBoxChecked, u.theme.Icon.CheckBoxUnchecked
	if checked == nil || unchecked == nil {
		t.Fatal("checkbox icons missing")
	}
	u.ensureCheckboxIcons()
	if checked != u.theme.Icon.CheckBoxChecked || unchecked != u.theme.Icon.CheckBoxUnchecked {
		t.Fatal("icons recreated")
	}
	if u.theme.Icon.RadioChecked != nil || u.theme.Icon.RadioUnchecked != nil {
		t.Fatal("unused radio icons loaded")
	}
}
