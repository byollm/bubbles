package textarea

import (
	"math/rand"
	"strings"
	"testing"
	"unicode"

	rw "github.com/mattn/go-runewidth"
	"github.com/rivo/uniseg"
)

// referenceWrap is the frozen v2.1.0 wrapping algorithm, extracted
// mechanically from `git show v2.1.0:textarea/textarea.go`. It measures the
// accumulated row and the current word from scratch for every word, which is
// what made wrapping a long line quadratic.
//
// Do not "fix" anything here; it is the reference the fast wrap() in
// textarea.go must stay byte-identical to.
func referenceWrap(runes []rune, width int) [][]rune {
	var (
		lines  = [][]rune{{}}
		word   = []rune{}
		row    int
		spaces int
	)

	// Word wrap the runes
	for _, r := range runes {
		if unicode.IsSpace(r) {
			spaces++
		} else {
			word = append(word, r)
		}

		if spaces > 0 {
			if uniseg.StringWidth(string(lines[row]))+uniseg.StringWidth(string(word))+spaces > width {
				row++
				lines = append(lines, []rune{})
				lines[row] = append(lines[row], word...)
				lines[row] = append(lines[row], repeatSpaces(spaces)...)
				spaces = 0
				word = nil
			} else {
				lines[row] = append(lines[row], word...)
				lines[row] = append(lines[row], repeatSpaces(spaces)...)
				spaces = 0
				word = nil
			}
		} else {
			lastCharLen := rw.RuneWidth(word[len(word)-1])
			if uniseg.StringWidth(string(word))+lastCharLen > width {
				if len(lines[row]) > 0 {
					row++
					lines = append(lines, []rune{})
				}
				lines[row] = append(lines[row], word...)
				word = nil
			}
		}
	}

	if uniseg.StringWidth(string(lines[row]))+uniseg.StringWidth(string(word))+spaces >= width {
		lines = append(lines, []rune{})
		lines[row+1] = append(lines[row+1], word...)
		spaces++
		lines[row+1] = append(lines[row+1], repeatSpaces(spaces)...)
	} else {
		lines[row] = append(lines[row], word...)
		spaces++
		lines[row] = append(lines[row], repeatSpaces(spaces)...)
	}

	return lines
}

func sameRows(a, b [][]rune) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if len(a[i]) != len(b[i]) {
			return false
		}
		for j := range a[i] {
			if a[i][j] != b[i][j] {
				return false
			}
		}
	}
	return true
}

// TestWrapMatchesReference pins the fast wrap() to the frozen v2.1.0
// algorithm. The fast wrap measures the accumulated row incrementally
// instead of re-measuring it for every word; a divergence here is a
// different set of soft-wrapped rows, which is a different rendered frame.
func TestWrapMatchesReference(t *testing.T) {
	corpus := []string{
		"",
		"a",
		"hello world",
		"hello world this is a line that will wrap a few times at narrow widths",
		"one\ntwo\nthree",
		strings.Repeat("word ", 60),
		strings.Repeat("x", 200),
		"tabs\tand\tsuch and some éèê accents",
		"日本語のテキスト wide runes 中文字符 mixed with ascii words here",
		"emoji \U0001F600 \U0001F680 rocket \U0001F469‍\U0001F4BB coder and more text after",
		"trailing spaces    \nleading spaces    here\n   indented",
		strings.Repeat("the quick brown fox jumps over the lazy dog\n", 40),
		strings.Repeat("世界", 50),
		"combining é́̂ and zwj \U0001F468‍\U0001F469‍\U0001F467 and flags \U0001F1E6\U0001F1E7\U0001F1E8",
		strings.Repeat("​", 30) + " zero width joiners",
	}
	for _, text := range corpus {
		for _, width := range []int{1, 2, 3, 5, 8, 20, 40, 80} {
			got := wrap([]rune(text), width)
			want := referenceWrap([]rune(text), width)
			if !sameRows(got, want) {
				t.Fatalf("wrap mismatch for %q at width %d:\n got %q\nwant %q",
					text, width, renderRows(got), renderRows(want))
			}
		}
	}
}

// TestWrapMatchesReferenceRandom fuzzes the same equivalence over random
// text built from the rune classes that make width measurement subtle:
// ASCII, wide runes, combining marks, ZWJ sequences, regional indicators and
// whitespace runs.
func TestWrapMatchesReferenceRandom(t *testing.T) {
	pool := []rune{
		'a', 'b', 'z', ' ', ' ', '\t', '\n',
		'é', 'ü', '日', '本', '語', '世', '界',
		'\U0001F600', '\U0001F680', '\U0001F469', '\U0001F4BB', '‍',
		'\U0001F1E6', '\U0001F1E7', '\U0001F1E8',
		'́', '​',
	}
	rng := rand.New(rand.NewSource(20261028))
	for i := 0; i < 3000; i++ {
		n := rng.Intn(60)
		text := make([]rune, n)
		for j := range text {
			text[j] = pool[rng.Intn(len(pool))]
		}
		width := 1 + rng.Intn(30)
		got := wrap(text, width)
		want := referenceWrap(text, width)
		if !sameRows(got, want) {
			t.Fatalf("wrap mismatch for %q at width %d:\n got %q\nwant %q",
				string(text), width, renderRows(got), renderRows(want))
		}
	}
}

// renderRows makes a wrap result readable in a failure message.
func renderRows(rows [][]rune) string {
	var b strings.Builder
	for _, row := range rows {
		b.WriteString("[")
		for _, r := range row {
			b.WriteRune(r)
		}
		b.WriteString("]")
	}
	return b.String()
}
