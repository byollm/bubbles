package textarea

// This file is a frozen, test-only copy of the v2.1.0 renderer, kept so
// TestViewMatchesOracle can prove the viewport-window renderer in textarea.go
// is byte-identical to the implementation it replaced. It is extracted
// mechanically from `git show v2.1.0:textarea/textarea.go`: method receivers
// became explicit parameters, the memo cache became a direct wrap() call
// (memoization never changed the output), and nothing else moved. Do not
// "fix" anything here; it is the reference.

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/rivo/uniseg"
)

func oracleWrap(runes []rune, width int) [][]rune {
	return wrap(runes, width)
}

func oracleLineInfo(m Model) LineInfo {
	grid := oracleWrap(m.value[m.row], m.width)

	// Find out which line we are currently on. This can be determined by the
	// m.col and counting the number of runes that we need to skip.
	var counter int
	for i, line := range grid {
		// We've found the line that we are on
		if counter+len(line) == m.col && i+1 < len(grid) {
			// We wrap around to the next line if we are at the end of the
			// previous line so that we can be at the very beginning of the row
			return LineInfo{
				CharOffset:   0,
				ColumnOffset: 0,
				Height:       len(grid),
				RowOffset:    i + 1,
				StartColumn:  m.col,
				Width:        len(grid[i+1]),
				CharWidth:    uniseg.StringWidth(string(line)),
			}
		}

		if counter+len(line) >= m.col {
			return LineInfo{
				CharOffset:   uniseg.StringWidth(string(line[:max(0, m.col-counter)])),
				ColumnOffset: m.col - counter,
				Height:       len(grid),
				RowOffset:    i,
				StartColumn:  counter,
				Width:        len(line),
				CharWidth:    uniseg.StringWidth(string(line)),
			}
		}

		counter += len(line)
	}
	return LineInfo{}
}

// repositionView repositions the view of the viewport based on the defined
// scrolling behavior.
func oracleViewLines(m *Model) string {
	if len(m.Value()) == 0 && m.row == 0 && m.col == 0 && m.Placeholder != "" {
		return oraclePlaceholderView(*m)
	}
	m.virtualCursor.TextStyle = m.activeStyle().computedCursorLine()

	var (
		s                strings.Builder
		style            lipgloss.Style
		newLines         int
		widestLineNumber int
		lineInfo         = oracleLineInfo(*m)
		styles           = m.activeStyle()
	)

	displayLine := 0
	for l, line := range m.value {
		wrappedLines := oracleWrap(line, m.width)

		if m.row == l {
			style = styles.computedCursorLine()
		} else {
			style = styles.computedText()
		}

		for wl, wrappedLine := range wrappedLines {
			prompt := oraclePromptView(*m, displayLine)
			prompt = styles.computedPrompt().Render(prompt)
			s.WriteString(style.Render(prompt))
			displayLine++

			var ln string
			if m.ShowLineNumbers {
				if wl == 0 { // normal line
					isCursorLine := m.row == l
					s.WriteString(oracleLineNumberView(*m, l+1, isCursorLine))
				} else { // soft wrapped line
					isCursorLine := m.row == l
					s.WriteString(oracleLineNumberView(*m, -1, isCursorLine))
				}
			}

			// Note the widest line number for padding purposes later.
			lnw := uniseg.StringWidth(ln)
			if lnw > widestLineNumber {
				widestLineNumber = lnw
			}

			strwidth := uniseg.StringWidth(string(wrappedLine))
			padding := m.width - strwidth
			// If the trailing space causes the line to be wider than the
			// width, we should not draw it to the screen since it will result
			// in an extra space at the end of the line which can look off when
			// the cursor line is showing.
			if strwidth > m.width {
				// The character causing the line to be wider than the width is
				// guaranteed to be a space since any other character would
				// have been wrapped.
				wrappedLine = []rune(strings.TrimSuffix(string(wrappedLine), " "))
				padding -= m.width - strwidth
			}
			if m.row == l && lineInfo.RowOffset == wl {
				s.WriteString(style.Render(string(wrappedLine[:lineInfo.ColumnOffset])))
				if m.col >= len(line) && lineInfo.CharOffset >= m.width {
					m.virtualCursor.SetChar(" ")
					s.WriteString(m.virtualCursor.View())
				} else {
					m.virtualCursor.SetChar(string(wrappedLine[lineInfo.ColumnOffset]))
					s.WriteString(style.Render(m.virtualCursor.View()))
					s.WriteString(style.Render(string(wrappedLine[lineInfo.ColumnOffset+1:])))
				}
			} else {
				s.WriteString(style.Render(string(wrappedLine)))
			}
			s.WriteString(style.Render(strings.Repeat(" ", max(0, padding))))
			s.WriteRune('\n')
			newLines++
		}
	}

	// Always show at least `m.Height` lines at all times.
	// To do this we can simply pad out a few extra new lines in the view.
	for range m.height {
		s.WriteString(oraclePromptView(*m, displayLine))
		displayLine++

		// Write end of buffer content
		leftGutter := string(m.EndOfBufferCharacter)
		rightGapWidth := m.Width() - uniseg.StringWidth(leftGutter) + widestLineNumber
		rightGap := strings.Repeat(" ", max(0, rightGapWidth))
		s.WriteString(styles.computedEndOfBuffer().Render(leftGutter + rightGap))
		s.WriteRune('\n')
	}

	return s.String()
}

// View renders the text area in its current state.
func oracleView(m Model) string {
	// XXX: This is a workaround for the case where the viewport hasn't
	// been initialized yet like during the initial render. In that case,
	// we need to render the view again because Update hasn't been called
	// yet to set the content of the viewport.
	vp := *m.viewport // private copy: the oracle must not touch the model under test
	m.viewport = &vp
	m.viewport.SetContent(oracleViewLines(&m))
	view := m.viewport.View()
	styles := m.activeStyle()
	return styles.Base.Render(view)
}

// promptView renders a single line of the prompt.
func oraclePromptView(m Model, displayLine int) (prompt string) {
	prompt = m.Prompt
	if m.promptFunc == nil {
		return prompt
	}
	prompt = m.promptFunc(PromptInfo{
		LineNumber: displayLine,
		Focused:    m.focus,
	})
	width := lipgloss.Width(prompt)
	if width < m.promptWidth {
		prompt = fmt.Sprintf("%*s%s", m.promptWidth-width, "", prompt)
	}

	return m.activeStyle().computedPrompt().Render(prompt)
}

// lineNumberView renders the line number.
//
// If the argument is less than 0, a space styled as a line number is returned
// instead. Such cases are used for soft-wrapped lines.
//
// The second argument indicates whether this line number is for a 'cursorline'
// line number.
func oracleLineNumberView(m Model, n int, isCursorLine bool) (str string) {
	if !m.ShowLineNumbers {
		return ""
	}

	if n <= 0 {
		str = " "
	} else {
		str = strconv.Itoa(n)
	}

	// XXX: is textStyle really necessary here?
	textStyle := m.activeStyle().computedText()
	lineNumberStyle := m.activeStyle().computedLineNumber()
	if isCursorLine {
		textStyle = m.activeStyle().computedCursorLine()
		lineNumberStyle = m.activeStyle().computedCursorLineNumber()
	}

	// Format line number dynamically based on the maximum number of lines.
	digits := len(strconv.Itoa(m.MaxHeight))
	str = fmt.Sprintf(" %*v ", digits, str)

	return textStyle.Render(lineNumberStyle.Render(str))
}

// placeholderView returns the prompt and placeholder, if any.
func oraclePlaceholderView(m Model) string {
	var (
		s      strings.Builder
		p      = m.Placeholder
		styles = m.activeStyle()
	)
	// word wrap lines
	pwordwrap := ansi.Wordwrap(p, m.width, "")
	// hard wrap lines (handles lines that could not be word wrapped)
	pwrap := ansi.Hardwrap(pwordwrap, m.width, true)
	// split string by new lines
	plines := strings.Split(strings.TrimSpace(pwrap), "\n")

	for i := range m.height {
		isLineNumber := len(plines) > i

		lineStyle := styles.computedPlaceholder()
		if len(plines) > i {
			lineStyle = styles.computedCursorLine()
		}

		// render prompt
		prompt := oraclePromptView(m, i)
		prompt = styles.computedPrompt().Render(prompt)
		s.WriteString(lineStyle.Render(prompt))

		// when show line numbers enabled:
		// - render line number for only the cursor line
		// - indent other placeholder lines
		// this is consistent with vim with line numbers enabled
		if m.ShowLineNumbers {
			var ln int

			switch {
			case i == 0:
				ln = i + 1
				fallthrough
			case len(plines) > i:
				s.WriteString(oracleLineNumberView(m, ln, isLineNumber))
			default:
			}
		}

		switch {
		// first line
		case i == 0:
			// first character of first line as cursor with character
			m.virtualCursor.TextStyle = styles.computedPlaceholder()

			ch, rest, _, _ := uniseg.FirstGraphemeClusterInString(plines[0], 0)
			m.virtualCursor.SetChar(ch)
			s.WriteString(lineStyle.Render(m.virtualCursor.View()))

			// the rest of the first line
			s.WriteString(lineStyle.Render(styles.computedPlaceholder().Render(rest)))

			// extend the first line with spaces to fill the width, so that
			// the entire line is filled when cursorline is enabled.
			gap := strings.Repeat(" ", max(0, m.width-lipgloss.Width(plines[0])))
			s.WriteString(lineStyle.Render(gap))
		// remaining lines
		case len(plines) > i:
			// current line placeholder text
			if len(plines) > i {
				placeholderLine := plines[i]
				gap := strings.Repeat(" ", max(0, m.width-uniseg.StringWidth(plines[i])))
				s.WriteString(lineStyle.Render(placeholderLine + gap))
			}
		default:
			// end of line buffer character
			eob := styles.computedEndOfBuffer().Render(string(m.EndOfBufferCharacter))
			s.WriteString(eob)
		}

		// terminate with new line
		s.WriteRune('\n')
	}

	m.viewport.SetContent(s.String())
	return styles.Base.Render(m.viewport.View())
}

// Blink returns the blink command for the virtual cursor.
func oracleCursorLineNumber(m Model) int {
	line := 0
	for i := range m.row {
		// Calculate the number of lines that the current line will be split
		// into.
		line += len(oracleWrap(m.value[i], m.width))
	}
	line += oracleLineInfo(m).RowOffset
	return line
}

// totalVisualLines returns the total number of display lines across all
// logical lines, accounting for soft wraps.
