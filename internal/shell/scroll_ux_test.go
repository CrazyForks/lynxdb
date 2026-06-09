package shell

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func newScrollModel(t *testing.T) Model {
	t.Helper()

	model := NewModel("file", RunOpts{File: "access.log"})
	model.width = 80
	model.height = 12
	model.recalcLayout()
	for i := 0; i < 40; i++ {
		model.results.AppendText("result line")
	}

	return model
}

func TestPgUpEntersScrollModeAndScrolls(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	model := newScrollModel(t)

	before := model.results.viewport.YOffset()
	model, _ = pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyPgUp})
	if model.focus != ResultsFocus {
		t.Fatal("PgUp should move focus to results")
	}
	if got := model.results.viewport.YOffset(); got >= before {
		t.Fatalf("PgUp did not scroll up: y offset %d, was %d", got, before)
	}
}

func TestScrollModeVimTopBottomKeys(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	model := newScrollModel(t)
	model.focus = ResultsFocus

	model, _ = pressKey(t, model, tea.KeyPressMsg{Code: 'g', Text: "g"})
	if got := model.results.viewport.YOffset(); got != 0 {
		t.Fatalf("g should jump to top, y offset = %d", got)
	}

	model, _ = pressKey(t, model, tea.KeyPressMsg{Code: 'G', Text: "G", Mod: tea.ModShift})
	if !model.results.viewport.AtBottom() {
		t.Fatalf("G should jump to bottom, y offset = %d", model.results.viewport.YOffset())
	}
}

func TestMouseWheelScrollsResultsInScrollMode(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	model := newScrollModel(t)
	model.focus = ResultsFocus

	before := model.results.viewport.YOffset()
	next, _ := model.Update(tea.MouseWheelMsg{X: 2, Y: 2, Button: tea.MouseWheelUp})
	model = next.(Model)
	if got := model.results.viewport.YOffset(); got >= before {
		t.Fatalf("wheel up did not scroll: y offset %d, was %d", got, before)
	}
}

func TestStatusBarIdleHintsScroll(t *testing.T) {
	sb := NewStatusBar("server")
	sb.SetWidth(100)

	got := plain(sb.View(EditorFocus, false, false, false, 0, nil, false, true, defaultKeyMap()))
	for _, want := range []string{"pgup", "scroll"} {
		if !strings.Contains(got, want) {
			t.Fatalf("idle status bar missing %q in %q", want, got)
		}
	}
}

func TestStatusBarScrollModeHintsVimKeys(t *testing.T) {
	sb := NewStatusBar("server")
	sb.SetWidth(100)

	got := plain(sb.View(ResultsFocus, false, false, false, 0, nil, false, true, defaultKeyMap()))
	for _, want := range []string{"j/k", "g/G", "top/bottom", "esc", "y/Y", "copy"} {
		if !strings.Contains(got, want) {
			t.Fatalf("scroll mode status bar missing %q in %q", want, got)
		}
	}
}
