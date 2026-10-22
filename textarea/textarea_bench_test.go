package textarea

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// benchProse builds n characters of prose: lines of roughly 70 characters
// separated by newlines, long enough to soft-wrap at the benchmark width now
// and then.
func benchProse(n int) string {
	const words = "the quick brown fox jumps over the lazy dog while the textarea keeps up "
	var b strings.Builder
	col := 0
	for b.Len() < n {
		for _, w := range strings.Fields(words) {
			if b.Len() >= n {
				break
			}
			if col+len(w)+1 > 70 {
				b.WriteByte('\n')
				col = 0
			}
			b.WriteString(w)
			b.WriteByte(' ')
			col += len(w) + 1
		}
	}
	return b.String()[:n]
}

// benchRows is the visible height of the benchmark textarea. The buffer is
// far taller, so a keystroke's cost must not depend on the buffer.
const benchRows = 8

// newBenchModel builds a focused textarea holding n characters with a
// viewport of benchRows rows. With clampHeight the height follows the content
// (DynamicHeight) and the caller clamps it back after each Update, which is
// how an application keeps a growing composer inside a cap without giving
// the textarea a MaxHeight; without it MaxHeight does the clamping.
func newBenchModel(n int, oneLine, clampHeight bool) Model {
	m := New()
	m.Prompt = " "
	m.ShowLineNumbers = false
	m.DynamicHeight = true
	m.MinHeight = 1
	if clampHeight {
		m.MaxHeight = 0
	} else {
		m.MaxHeight = benchRows
	}
	m.SetWidth(100)
	m.Focus()
	text := benchProse(n)
	if oneLine {
		text = strings.ReplaceAll(text, "\n", " ")
	}
	m.SetValue(text)
	m.SetHeight(benchRows)
	m, _ = m.Update(nil)
	m.SetHeight(benchRows)
	_ = m.View()
	return m
}

func benchKeystroke(b *testing.B, m Model, key tea.Msg) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m, _ = m.Update(key)
		if m.Height() > benchRows {
			m.SetHeight(benchRows)
		}
		_ = m.View()
	}
}

var benchSizes = []int{1 << 10, 4 << 10, 16 << 10, 32 << 10}

// BenchmarkKeystroke measures one printable keystroke -- Update followed by
// View, which is what a frame costs -- with the buffer holding 1k..32k
// characters of prose over many logical lines and the height clamped by the
// caller (MaxHeight 0).
func BenchmarkKeystroke(b *testing.B) {
	key := tea.KeyPressMsg{Code: 'k', Text: "k"}
	for _, n := range benchSizes {
		b.Run(fmt.Sprintf("%dk", n>>10), func(b *testing.B) {
			benchKeystroke(b, newBenchModel(n, false, true), key)
		})
	}
	b.Run("32k-one-line", func(b *testing.B) {
		benchKeystroke(b, newBenchModel(32<<10, true, true), key)
	})
}

// BenchmarkKeystrokeMaxHeight is BenchmarkKeystroke with MaxHeight set to the
// visible height instead of a caller-side clamp: in v2.1.0 this sized the
// wrap memo to MaxHeight entries and re-wrapped the whole buffer on every
// keystroke.
func BenchmarkKeystrokeMaxHeight(b *testing.B) {
	key := tea.KeyPressMsg{Code: 'k', Text: "k"}
	for _, n := range benchSizes {
		b.Run(fmt.Sprintf("%dk", n>>10), func(b *testing.B) {
			benchKeystroke(b, newBenchModel(n, false, false), key)
		})
	}
}

// BenchmarkCursorUp measures a vertical cursor move at the bottom of a large
// buffer: the cursor row lookup plus the frame.
func BenchmarkCursorUp(b *testing.B) {
	for _, n := range benchSizes {
		b.Run(fmt.Sprintf("%dk", n>>10), func(b *testing.B) {
			m := newBenchModel(n, false, true)
			up := tea.KeyPressMsg{Code: tea.KeyUp}
			down := tea.KeyPressMsg{Code: tea.KeyDown}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				m, _ = m.Update(up)
				if m.Height() > benchRows {
					m.SetHeight(benchRows)
				}
				_ = m.View()
				m, _ = m.Update(down)
				if m.Height() > benchRows {
					m.SetHeight(benchRows)
				}
				_ = m.View()
			}
		})
	}
}
