package shell

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestIsExitCommand(t *testing.T) {
	positives := []string{
		"exit", "EXIT", "quit", "QUIT", "logout",
		"q", "q;", "  quit ; ", ":q", `\q`, `\Q`,
		"exit;", "quit;;",
		"учше", "йгше", "дщпщге", "учшеж", "йгшеж", "дщпщгеж",
		"й", `\й`, `\Й`, "жй", "Жй",
	}
	for _, in := range positives {
		if !isExitCommand(in) {
			t.Errorf("isExitCommand(%q) = false, want true", in)
		}
	}

	negatives := []string{
		"", ";", "quitter", "q u", "exit now",
		"from * | head 5", "/quit", "level=error",
	}
	for _, in := range negatives {
		if isExitCommand(in) {
			t.Errorf("isExitCommand(%q) = true, want false", in)
		}
	}
}

func TestEditorSubmitExitWordQuitsWithoutHistory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	history := NewHistory()
	editor := NewEditor("lynxdb> ", "   ...> ", history, NewCompleter())
	editor.SetValue("quit")

	_, submit, slash := editor.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if submit != nil {
		t.Fatalf("exit word submitted as query %q", submit.query)
	}
	if slash == nil || !slash.quit {
		t.Fatalf("exit word did not produce quit, slash = %+v", slash)
	}
	for _, entry := range history.Entries(20) {
		if entry == "quit" {
			t.Fatal("exit word was added to history")
		}
	}
}

// withFakeClock pins shellNow to a controllable clock and restores it on cleanup.
func withFakeClock(t *testing.T, start time.Time) *time.Time {
	t.Helper()

	now := start
	shellNow = func() time.Time { return now }
	t.Cleanup(func() { shellNow = time.Now })

	return &now
}

func pressKey(t *testing.T, m Model, msg tea.KeyPressMsg) (Model, tea.Cmd) {
	t.Helper()

	next, cmd := m.Update(msg)
	model, ok := next.(Model)
	if !ok {
		t.Fatalf("unexpected model type %T", next)
	}

	return model, cmd
}

func isQuitCmd(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)

	return ok
}

var (
	ctrlC  = tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	escKey = tea.KeyPressMsg{Code: tea.KeyEscape}
)

func TestCtrlCDoublePressQuits(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	now := withFakeClock(t, time.Unix(1000, 0))

	model := NewModel("file", RunOpts{File: "access.log"})

	model, cmd := pressKey(t, model, ctrlC)
	if isQuitCmd(cmd) {
		t.Fatal("first Ctrl+C must not quit")
	}
	if model.quitArmedAt.IsZero() {
		t.Fatal("first Ctrl+C should arm quit confirmation")
	}
	if !strings.Contains(model.statusBar.flashMsg, "Ctrl+C") {
		t.Fatalf("status bar hint missing, got %q", model.statusBar.flashMsg)
	}

	*now = now.Add(time.Second)
	if _, cmd = pressKey(t, model, ctrlC); !isQuitCmd(cmd) {
		t.Fatal("second Ctrl+C within window should quit")
	}
}

func TestCtrlCArmExpiresAfterWindow(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	now := withFakeClock(t, time.Unix(1000, 0))

	model := NewModel("file", RunOpts{File: "access.log"})

	model, _ = pressKey(t, model, ctrlC)
	*now = now.Add(quitConfirmWindow + time.Second)

	model, cmd := pressKey(t, model, ctrlC)
	if isQuitCmd(cmd) {
		t.Fatal("Ctrl+C after the window expired must not quit")
	}
	if !model.quitArmedAt.Equal(*now) {
		t.Fatal("expired press should re-arm the confirmation")
	}
}

func TestCtrlCWithInputClearsNotQuits(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	model := NewModel("file", RunOpts{File: "access.log"})
	model.editor.SetValue("from nginx | head 5")

	model, cmd := pressKey(t, model, ctrlC)
	if isQuitCmd(cmd) {
		t.Fatal("Ctrl+C with pending input must not quit")
	}
	if got := model.editor.Value(); got != "" {
		t.Fatalf("Ctrl+C should clear input, got %q", got)
	}
	if !model.quitArmedAt.IsZero() {
		t.Fatal("Ctrl+C that cleared input must not arm quit")
	}
}

func TestCtrlCDuringRunningQueryCancelsNotQuits(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	model := NewModel("file", RunOpts{File: "access.log"})
	model.running = true

	model, cmd := pressKey(t, model, ctrlC)
	if isQuitCmd(cmd) {
		t.Fatal("Ctrl+C during a running query must not quit")
	}
	if model.running {
		t.Fatal("Ctrl+C should cancel the running query")
	}
	if !model.quitArmedAt.IsZero() {
		t.Fatal("query-cancel press must not arm quit")
	}
}

func TestEscDoublePressQuitsOnIdleEditor(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	now := withFakeClock(t, time.Unix(1000, 0))

	model := NewModel("file", RunOpts{File: "access.log"})

	model, cmd := pressKey(t, model, escKey)
	if isQuitCmd(cmd) {
		t.Fatal("first Esc must not quit")
	}
	if !strings.Contains(model.statusBar.flashMsg, "Esc") {
		t.Fatalf("status bar hint missing, got %q", model.statusBar.flashMsg)
	}

	*now = now.Add(time.Second)
	if _, cmd = pressKey(t, model, escKey); !isQuitCmd(cmd) {
		t.Fatal("second Esc within window should quit")
	}
}

func TestEscFromResultsFocusReturnsFocusNotArm(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	model := NewModel("file", RunOpts{File: "access.log"})
	model.focus = ResultsFocus

	model, cmd := pressKey(t, model, escKey)
	if isQuitCmd(cmd) {
		t.Fatal("Esc from results focus must not quit")
	}
	if model.focus != EditorFocus {
		t.Fatal("Esc should return focus to the editor")
	}
	if !model.quitArmedAt.IsZero() {
		t.Fatal("Esc that switched focus must not arm quit")
	}
}

func TestTypingDisarmsQuitConfirmation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	withFakeClock(t, time.Unix(1000, 0))

	model := NewModel("file", RunOpts{File: "access.log"})

	model, _ = pressKey(t, model, ctrlC)
	if model.quitArmedAt.IsZero() {
		t.Fatal("Ctrl+C should arm quit confirmation")
	}

	model, _ = pressKey(t, model, tea.KeyPressMsg{Code: 'a', Text: "a"})
	if !model.quitArmedAt.IsZero() {
		t.Fatal("typing should disarm quit confirmation")
	}
	if model.statusBar.flashMsg != "" {
		t.Fatalf("typing should clear the hint, got %q", model.statusBar.flashMsg)
	}
}

func TestStatusBarDefaultHelpShowsQuitHint(t *testing.T) {
	sb := NewStatusBar("server")
	sb.SetWidth(100)

	got := plain(sb.View(EditorFocus, false, false, false, 0, nil, false, true, defaultKeyMap()))
	for _, want := range []string{"ctrl+d", "quit"} {
		if !strings.Contains(got, want) {
			t.Fatalf("default status bar missing %q in %q", want, got)
		}
	}
}

func TestHelpTextMentionsAllExitRoutes(t *testing.T) {
	got := helpText()
	for _, want := range []string{"/quit", "exit", "logout", ":q", `\q`, "Ctrl+D", "Ctrl+C", "Esc"} {
		if !strings.Contains(got, want) {
			t.Fatalf("help text missing %q", want)
		}
	}
}
