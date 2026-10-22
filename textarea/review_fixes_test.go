package textarea

import (
	"runtime"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func bigModel(lines int) Model {
	m := New()
	m.MaxHeight = 0
	m.SetWidth(60)
	m.SetHeight(8)
	m.Focus()
	var b strings.Builder
	for i := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString("line of text number ")
		b.WriteString(strings.Repeat("x", i%40))
	}
	m.SetValue(b.String())
	m, _ = m.Update(nil)
	_ = m.View()
	return m
}

// TestResetClearsWrapCacheTail: after a long message is sent and the
// textarea Reset, none of its wrapped rows may stay reachable behind len in
// the reused wrap cache, and no rendered row may stay behind len in the
// viewport content slice. View works on a copy of the model and never
// shortens the original's len, so the viewport check starts at the current
// row count rather than at len.
func TestResetClearsWrapCacheTail(t *testing.T) {
	m := bigModel(2000)
	if cap(m.wrapped) < 2000 {
		t.Fatalf("cap(wrapped) = %d, want >= 2000", cap(m.wrapped))
	}
	m.Reset()
	_ = m.View()
	if len(m.wrapped) != 1 {
		t.Fatalf("len(wrapped) = %d after Reset, want 1", len(m.wrapped))
	}
	for i, rows := range m.wrapped[len(m.wrapped):cap(m.wrapped)] {
		if rows != nil {
			t.Fatalf("wrapped tail entry %d still holds %d rows after Reset", i, len(rows))
		}
	}
	for i, s := range m.lines[m.rows.total()+m.height+1 : cap(m.lines)] {
		if s != "" {
			t.Fatalf("viewport line tail entry %d still holds %q after Reset", i, s)
		}
	}

	// The same after shrinking through SetValue rather than Reset.
	m = bigModel(2000)
	m.SetValue("short")
	_ = m.View()
	for i, rows := range m.wrapped[len(m.wrapped):cap(m.wrapped)] {
		if rows != nil {
			t.Fatalf("wrapped tail entry %d still holds rows after SetValue", i)
		}
	}
	for i, s := range m.lines[m.rows.total()+m.height+1 : cap(m.lines)] {
		if s != "" {
			t.Fatalf("viewport line tail entry %d still holds %q after SetValue", i, s)
		}
	}
}

// viewBytes is the heap bytes one View allocates.
func viewBytes(m Model) uint64 {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	_ = m.View()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

// TestViewDoesNotGrowContentSliceOnCopy: View works on a copy of the model,
// so a content slice it had to grow would be lost and grown again on every
// frame. Every mutation reserves the slice instead, so View after SetValue,
// InsertString, SetWidth or SetHeight (with no Update in between) allocates
// only its rendered rows, the same amount for 20 lines and for 2000.
func TestViewDoesNotGrowContentSliceOnCopy(t *testing.T) {
	check := func(label string, mutate func(m *Model)) {
		t.Helper()
		small := bigModel(20)
		large := bigModel(2000)
		mutate(&small)
		mutate(&large)
		for _, m := range []*Model{&small, &large} {
			if n := m.rows.total() + m.height + 1; cap(m.lines) < n {
				t.Fatalf("%s: cap(lines) = %d, want >= %d after mutation", label, cap(m.lines), n)
			}
		}
		// Render once so both copies are warm, then measure a frame each.
		_ = small.View()
		_ = large.View()
		s, l := viewBytes(small), viewBytes(large)
		if l > s+4096 {
			t.Fatalf("%s: View allocates %d B at 2000 lines vs %d B at 20 lines: the content slice is being regrown per frame", label, l, s)
		}
	}
	check("SetValue", func(m *Model) { m.SetValue(m.Value() + "\nmore") })
	check("InsertString", func(m *Model) { m.InsertString("\ninserted\nlines") })
	check("SetWidth", func(m *Model) { m.SetWidth(45) })
	check("SetHeight", func(m *Model) { m.SetHeight(12) })
}

// TestWordLeftAtBufferStartReturns: alt+left with nothing but whitespace
// before the cursor used to spin forever.
func TestWordLeftAtBufferStartReturns(t *testing.T) {
	for _, text := range []string{"", "   ", " \n \n", "\n\nword"} {
		m := New()
		m.Focus()
		m.SetValue(text)
		m.MoveToBegin()
		if text == "   " {
			m.CursorEnd()
		}
		done := make(chan Model, 1)
		go func() {
			next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModAlt})
			done <- next
		}()
		select {
		case next := <-done:
			if next.Line() != 0 || next.Column() != 0 {
				t.Fatalf("%q: cursor at (%d,%d) after alt+left, want (0,0)", text, next.Line(), next.Column())
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("%q: alt+left did not return", text)
		}
	}
	// And it still finds a word when one is there.
	m := New()
	m.Focus()
	m.SetValue("one two")
	m.CursorEnd()
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModAlt})
	if m.Column() != 4 {
		t.Fatalf("alt+left from end of %q: column %d, want 4", "one two", m.Column())
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModAlt})
	if m.Column() != 0 {
		t.Fatalf("second alt+left: column %d, want 0", m.Column())
	}
}

// TestSetHeightAfterSetValueKeepsCursorVisible: a direct SetValue of more
// rows than the height, then SetHeight, with no Update in between (a
// restored draft, a model switch, a re-focus after blur). repositionView
// used to scroll against the viewport's line count from the last View, so
// the scroll clamped short and the cursor sat below the window until the
// next keystroke.
func TestSetHeightAfterSetValueKeepsCursorVisible(t *testing.T) {
	for _, dynamic := range []bool{true, false} {
		m := New()
		m.MaxHeight = 0
		m.DynamicHeight = dynamic
		m.SetWidth(60)
		m.SetHeight(8)
		m.Focus()
		m, _ = m.Update(nil)
		_ = m.View()

		lines := make([]string, 24)
		for i := range lines {
			lines[i] = "line"
		}
		m.SetValue(strings.Join(lines, "\n"))
		m.SetHeight(8)

		row := m.cursorLineNumber()
		top, bottom := m.viewport.YOffset(), m.viewport.YOffset()+m.viewport.Height()-1
		if row < top || row > bottom {
			t.Fatalf("dynamic=%v: cursor row %d outside visible rows [%d,%d] after SetValue + SetHeight", dynamic, row, top, bottom)
		}
		view := m.View()
		if !strings.Contains(view, "24 ") {
			t.Fatalf("dynamic=%v: View does not show the cursor line:\n%s", dynamic, view)
		}
	}
}
