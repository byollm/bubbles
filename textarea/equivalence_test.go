package textarea

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// The corpus below drives the viewport-window renderer through every
// configuration that changes a row's bytes (prompt, prompt function, line
// numbers, focus, placeholder, styles, dynamic height, content caps) and
// every mutation path (typing, deleting, splitting, merging, pasting, case
// changes, transposition, cursor and page moves, width and height changes,
// explicit viewport scrolls), and after each step checks two things:
//
//   - View() is byte-identical to the frozen v2.1.0 renderer in
//     oracle_test.go;
//   - the wrap cache and the row tree agree with a fresh wrap of every line.
//
// A missed invalidation at any mutation site shows up here as a stale row.

var corpusTexts = []string{
	"",
	"hello",
	"hello world this is a line that will wrap a few times at narrow widths",
	"one\ntwo\nthree",
	"\n\n\n",
	"a\n\nb\n\n\nc",
	strings.Repeat("word ", 60),
	strings.Repeat("x", 200),
	"tabs\tand\tsuch and some éèê accents",
	"日本語のテキスト wide runes 中文字符 mixed with ascii words here",
	"emoji \U0001F600 \U0001F680 rocket \U0001F469‍\U0001F4BB coder and more text after",
	"trailing spaces    \nleading spaces    here\n   indented",
	strings.Repeat("the quick brown fox jumps over the lazy dog\n", 40),
	strings.Repeat("世界", 50),
	"short\n" + strings.Repeat("a long line that goes on and on and on and on and on and on ", 8) + "\nshort again",
}

type corpusConfig struct {
	name        string
	width       int
	height      int
	lineNums    bool
	prompt      string
	promptFunc  bool
	dynamic     bool
	maxHeight   int
	maxContent  int
	focused     bool
	placeholder string
	eob         rune
	base        bool
}

func corpusConfigs() []corpusConfig {
	var cfgs []corpusConfig
	widths := []int{5, 12, 20, 40, 80}
	heights := []int{1, 3, 6, 12}
	i := 0
	for _, w := range widths {
		for _, h := range heights {
			c := corpusConfig{
				name:        fmt.Sprintf("w%d-h%d", w, h),
				width:       w,
				height:      h,
				lineNums:    i%2 == 0,
				prompt:      []string{"> ", "", "| ", "┃ "}[i%4],
				promptFunc:  i%5 == 0,
				dynamic:     i%3 == 0,
				maxHeight:   []int{99, 0, 4, 8}[i%4],
				maxContent:  []int{0, 0, 0, 10}[i%4],
				focused:     i%7 != 6,
				placeholder: []string{"Type here...", "", "A placeholder long enough to wrap at narrow widths for sure"}[i%3],
				eob:         []rune{' ', '~', '.'}[i%3],
				base:        i%6 == 5,
			}
			cfgs = append(cfgs, c)
			i++
		}
	}
	return cfgs
}

func newCorpusModel(cfg corpusConfig) Model {
	m := New()
	m.Prompt = cfg.prompt
	m.ShowLineNumbers = cfg.lineNums
	m.Placeholder = cfg.placeholder
	m.EndOfBufferCharacter = cfg.eob
	m.MaxHeight = cfg.maxHeight
	m.MaxContentHeight = cfg.maxContent
	m.DynamicHeight = cfg.dynamic
	m.MinHeight = 1
	if cfg.promptFunc {
		m.SetPromptFunc(3, func(info PromptInfo) string {
			if info.Focused {
				return fmt.Sprintf("%d:", info.LineNumber)
			}
			return "-"
		})
	}
	if cfg.base {
		s := m.Styles()
		s.Focused.Base = lipgloss.NewStyle().Padding(0, 1).Border(lipgloss.NormalBorder())
		s.Blurred.Base = s.Focused.Base
		s.Focused.CursorLine = lipgloss.NewStyle().Background(lipgloss.Color("236"))
		m.SetStyles(s)
	}
	m.SetWidth(cfg.width)
	m.SetHeight(cfg.height)
	if cfg.focused {
		m.Focus()
	} else {
		m.Blur()
	}
	return m
}

// checkConsistent verifies the wrap cache and row tree against a fresh wrap
// of every line.
func checkConsistent(t *testing.T, m Model, step string) {
	t.Helper()
	if len(m.wrapped) != len(m.value) {
		t.Fatalf("%s: wrapped has %d lines, value has %d", step, len(m.wrapped), len(m.value))
	}
	sum := 0
	for i, l := range m.value {
		want := wrap(l, m.width)
		got := m.wrapped[i]
		if len(got) != len(want) {
			t.Fatalf("%s: line %d: cached %d rows, fresh wrap %d rows", step, i, len(got), len(want))
		}
		for r := range want {
			if string(got[r]) != string(want[r]) {
				t.Fatalf("%s: line %d row %d: cached %q, fresh %q", step, i, r, string(got[r]), string(want[r]))
			}
		}
		if p := m.rows.prefix(i); p != sum {
			t.Fatalf("%s: prefix(%d) = %d, want %d", step, i, p, sum)
		}
		for r := range want {
			line, off := m.rows.find(sum + r)
			if line != i || off != r {
				t.Fatalf("%s: find(%d) = (%d,%d), want (%d,%d)", step, sum+r, line, off, i, r)
			}
		}
		sum += len(want)
	}
	if tot := m.rows.total(); tot != sum {
		t.Fatalf("%s: total() = %d, want %d", step, tot, sum)
	}
	if got, want := m.cursorLineNumber(), oracleCursorLineNumber(m); got != want {
		t.Fatalf("%s: cursorLineNumber = %d, oracle %d", step, got, want)
	}
}

func checkView(t *testing.T, m Model, step string) {
	t.Helper()
	checkConsistent(t, m, step)
	want := oracleView(m)
	got := m.View()
	if got != want {
		t.Fatalf("%s: View differs from v2.1.0 oracle\nwant:\n%q\ngot:\n%q", step, want, got)
	}
	// A second View must be idempotent: the first may have clamped the
	// viewport, the oracle sees the same clamp.
	if again := m.View(); again != want {
		t.Fatalf("%s: second View differs\nwant:\n%q\ngot:\n%q", step, want, again)
	}
}

var corpusKeys = []tea.KeyPressMsg{
	{Code: tea.KeyLeft}, {Code: tea.KeyRight}, {Code: tea.KeyUp}, {Code: tea.KeyDown},
	{Code: tea.KeyHome}, {Code: tea.KeyEnd}, {Code: tea.KeyPgUp}, {Code: tea.KeyPgDown},
	{Code: tea.KeyBackspace}, {Code: tea.KeyDelete}, {Code: tea.KeyEnter},
	{Code: 'w', Mod: tea.ModCtrl}, {Code: 'd', Mod: tea.ModAlt}, {Code: 'k', Mod: tea.ModCtrl},
	{Code: 'u', Mod: tea.ModCtrl}, {Code: 't', Mod: tea.ModCtrl}, {Code: 'c', Mod: tea.ModAlt},
	{Code: 'l', Mod: tea.ModAlt}, {Code: 'u', Mod: tea.ModAlt}, {Code: tea.KeyLeft, Mod: tea.ModAlt},
	{Code: tea.KeyRight, Mod: tea.ModAlt}, {Code: '<', Mod: tea.ModAlt}, {Code: '>', Mod: tea.ModAlt},
	{Code: 'a', Text: "a"}, {Code: ' ', Text: " "}, {Code: '日', Text: "日"}, {Code: 'Z', Text: "Z"},
	{Code: '\U0001F600', Text: "\U0001F600"},
}

// TestViewMatchesOracle is the byte-identity proof for the window renderer.
func TestViewMatchesOracle(t *testing.T) {
	t.Parallel()
	for _, cfg := range corpusConfigs() {
		t.Run(cfg.name, func(t *testing.T) {
			t.Parallel()
			rng := rand.New(rand.NewSource(int64(cfg.width*100 + cfg.height)))
			for ti, text := range corpusTexts {
				m := newCorpusModel(cfg)
				step := func(s string) string { return fmt.Sprintf("text %d %s", ti, s) }
				checkView(t, m, step("fresh"))
				m.SetValue(text)
				checkView(t, m, step("after SetValue"))
				m, _ = m.Update(nil)
				checkView(t, m, step("after Update(nil)"))

				// Walk the cursor to a few places and scroll explicitly.
				for i := range 6 {
					m.MoveToBegin()
					for range rng.Intn(50) {
						m.characterRight()
					}
					m.repositionView()
					checkView(t, m, step(fmt.Sprintf("cursor walk %d", i)))
					m.viewport.GotoTop()
					checkView(t, m, step(fmt.Sprintf("goto top %d", i)))
					m.viewport.ScrollDown(rng.Intn(5))
					checkView(t, m, step(fmt.Sprintf("scrolled %d", i)))
					m.viewport.GotoBottom()
					checkView(t, m, step(fmt.Sprintf("goto bottom %d", i)))
				}
				m.MoveToEnd()
				checkView(t, m, step("end"))

				// Random edits through Update.
				for i := range 60 {
					k := corpusKeys[rng.Intn(len(corpusKeys))]
					m, _ = m.Update(k)
					checkView(t, m, step(fmt.Sprintf("edit %d key %v", i, k)))
					if i%17 == 16 {
						m, _ = m.Update(tea.PasteMsg{Content: "pasted\nlines\nhere " + strings.Repeat("p", rng.Intn(60))})
						checkView(t, m, step(fmt.Sprintf("paste %d", i)))
					}
					if i%23 == 22 {
						m.SetWidth(5 + rng.Intn(60))
						checkView(t, m, step(fmt.Sprintf("set width %d", i)))
					}
					if i%29 == 28 {
						m.SetHeight(1 + rng.Intn(10))
						checkView(t, m, step(fmt.Sprintf("set height %d", i)))
					}
					if i%31 == 30 {
						m.InsertString("ins\nerted")
						checkView(t, m, step(fmt.Sprintf("insert string %d", i)))
					}
					if i%37 == 36 {
						m.Blur()
						checkView(t, m, step(fmt.Sprintf("blurred %d", i)))
						m.Focus()
						checkView(t, m, step(fmt.Sprintf("refocused %d", i)))
					}
				}
				m.Reset()
				checkView(t, m, step("reset"))
			}
		})
	}
}

// TestRowTree checks the Fenwick tree on its own against brute force.
func TestRowTree(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for n := range 70 {
		counts := make([]int, n)
		wrapped := make([][][]rune, n)
		for i := range counts {
			counts[i] = 1 + rng.Intn(4)
			wrapped[i] = make([][]rune, counts[i])
		}
		var tr rowTree
		tr.rebuild(wrapped)
		for range 20 {
			if n > 0 {
				i := rng.Intn(n)
				c := 1 + rng.Intn(4)
				tr.add(i, c-counts[i])
				counts[i] = c
			}
			sum := 0
			for i, c := range counts {
				if p := tr.prefix(i); p != sum {
					t.Fatalf("n=%d prefix(%d)=%d want %d", n, i, p, sum)
				}
				for r := range c {
					l, off := tr.find(sum + r)
					if l != i || off != r {
						t.Fatalf("n=%d find(%d)=(%d,%d) want (%d,%d)", n, sum+r, l, off, i, r)
					}
				}
				sum += c
			}
			if tr.total() != sum {
				t.Fatalf("n=%d total=%d want %d", n, tr.total(), sum)
			}
		}
	}
}
