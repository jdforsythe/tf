package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func (m model) View() string {
	if m.quitting {
		return ""
	}
	switch m.stage {
	case stagePlanning:
		return m.viewProgress(false)
	case stageShowing:
		return m.viewShowing()
	case stageReview:
		return m.viewReview()
	case stageApplying:
		return m.viewProgress(true)
	}
	return ""
}

func (m model) spinner() string {
	return stCyan.Render(spinnerFrames[m.spin%len(spinnerFrames)])
}

// viewProgress renders the "active list" used by both plan-refresh and apply.
func (m model) viewProgress(applying bool) string {
	var b strings.Builder
	elapsed := time.Since(m.startedAt)

	title := "Planning"
	if m.cfg.sub == "destroy" {
		title = "Planning destroy"
	}
	if applying {
		title = "Applying"
		if m.cfg.sub == "destroy" {
			title = "Destroying"
		}
	}
	if m.interrupted {
		title += " (interrupting — press again to force)"
	}

	b.WriteString("\n " + m.spinner() + " " + stBold.Render(title) + "  " + stDim.Render(fmtDur(elapsed)) + "\n")

	if applying {
		b.WriteString(" " + m.progressLine(elapsed) + "\n")
	} else {
		info := []string{}
		if m.refreshed > 0 {
			info = append(info, fmt.Sprintf("%d refreshed", m.refreshed))
		}
		if m.plannedSeen > 0 {
			info = append(info, stYellow.Render(fmt.Sprintf("%d changes detected", m.plannedSeen)))
		}
		if len(info) > 0 {
			b.WriteString(" " + stDim.Render(strings.Join(info, " • ")) + "\n")
		} else {
			b.WriteString(" " + stDim.Render("refreshing state…") + "\n")
		}
	}
	b.WriteString("\n")

	// active / recently finished tasks
	maxRows := m.height - 8
	if maxRows < 3 {
		maxRows = 3
	}
	nameW := m.width - 22
	if nameW < 20 {
		nameW = 20
	}
	shown := 0
	hidden := 0
	for _, addr := range m.order {
		t := m.tasks[addr]
		if t.status == taskFailed {
			continue // failures render in the diagnostics block below
		}
		if shown >= maxRows {
			hidden++
			continue
		}
		shown++
		name := shorten(addr, nameW)
		switch t.status {
		case taskRunning:
			line := fmt.Sprintf("   %s %s %s", m.spinner(), stYellow.Render(name), stDim.Render(fmtDur(time.Since(t.start))))
			if t.note != "" {
				line += " " + stCyan.Render("("+t.note+")")
			}
			b.WriteString(line + "\n")
		case taskDone:
			dur := ""
			if t.elapsed > 0 {
				dur = stDim.Render(fmtDur(time.Duration(t.elapsed * float64(time.Second))))
			}
			b.WriteString(fmt.Sprintf("   %s %s %s\n", stGreen.Render("✓"), stGreen.Render(name), dur))
		}
	}
	if hidden > 0 {
		b.WriteString("   " + stDim.Render(fmt.Sprintf("… +%d more in flight", hidden)) + "\n")
	}
	if shown == 0 && hidden == 0 {
		b.WriteString("   " + stDim.Render("waiting…") + "\n")
	}

	// sticky failures + diagnostics
	b.WriteString(m.viewFailures(nameW))
	b.WriteString(m.viewDiags(4))

	return b.String()
}

func (m model) progressLine(elapsed time.Duration) string {
	total := m.total
	if total == 0 {
		total = 1
	}
	done := m.completed + m.failed
	barW := 24
	filled := done * barW / total
	if filled > barW {
		filled = barW
	}
	bar := stGreen.Render(strings.Repeat("█", filled)) + stDim.Render(strings.Repeat("░", barW-filled))

	pct := done * 100 / total
	parts := []string{
		bar,
		fmt.Sprintf("%d/%d", done, m.total),
		fmt.Sprintf("%d%%", pct),
	}
	running := 0
	for _, t := range m.tasks {
		if t.status == taskRunning {
			running++
		}
	}
	if running > 0 {
		parts = append(parts, stYellow.Render(fmt.Sprintf("%d active", running)))
	}
	if m.failed > 0 {
		parts = append(parts, stRed.Render(fmt.Sprintf("%d failed", m.failed)))
	}
	// naive ETA from observed completion rate
	if m.completed >= 2 && elapsed > 5*time.Second && done < total {
		rate := elapsed / time.Duration(done)
		eta := rate * time.Duration(total-done)
		parts = append(parts, stDim.Render("~"+fmtDur(eta)+" left"))
	}
	return strings.Join(parts, "  ")
}

func (m model) viewFailures(nameW int) string {
	var b strings.Builder
	for _, addr := range m.order {
		t := m.tasks[addr]
		if t.status != taskFailed {
			continue
		}
		b.WriteString(fmt.Sprintf("   %s %s %s\n", stRedBold.Render("✗"), stRed.Render(shorten(addr, nameW)), stDim.Render("failed")))
	}
	return b.String()
}

func (m model) viewDiags(max int) string {
	if len(m.diags) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n")
	start := 0
	if len(m.diags) > max {
		start = len(m.diags) - max
		b.WriteString(" " + stDim.Render(fmt.Sprintf("(%d earlier diagnostics)", start)) + "\n")
	}
	for _, d := range m.diags[start:] {
		for _, line := range renderDiagLines(d, m.width) {
			b.WriteString(" " + line + "\n")
		}
	}
	return b.String()
}

func renderDiagLines(d *Diagnostic, width int) []string {
	mark := stRedBold.Render("✗ Error:")
	body := stRed
	if d.Severity == "warning" {
		mark = stYellow.Render("⚠ Warning:")
		body = stYellow
	}
	head := mark + " " + body.Render(d.Summary)
	if d.Address != "" {
		head += stDim.Render("  [" + d.Address + "]")
	}
	if d.Range != nil && d.Range.Filename != "" {
		head += stDim.Render(fmt.Sprintf("  %s:%d", d.Range.Filename, d.Range.Start.Line))
	}
	lines := []string{head}
	if d.Detail != "" {
		for _, dl := range strings.Split(strings.TrimSpace(d.Detail), "\n") {
			lines = append(lines, "    "+stDim.Render(truncRunes(dl, width-6)))
		}
	}
	return lines
}

func (m model) viewShowing() string {
	return "\n " + m.spinner() + " " + stBold.Render("Reading plan…") + "\n"
}

// ---- review tree ----

func (m model) viewReview() string {
	var b strings.Builder
	c := m.counts

	title := "Plan"
	if m.cfg.sub == "destroy" {
		title = "Destroy plan"
	}
	parts := []string{}
	if c.add > 0 {
		parts = append(parts, stGreen.Render(fmt.Sprintf("+%d add", c.add)))
	}
	if c.change > 0 {
		parts = append(parts, stYellow.Render(fmt.Sprintf("~%d change", c.change)))
	}
	if c.replace > 0 {
		parts = append(parts, stMagenta.Render(fmt.Sprintf("±%d replace", c.replace)))
	}
	if c.destroy > 0 {
		parts = append(parts, stRed.Render(fmt.Sprintf("-%d destroy", c.destroy)))
	}
	header := " " + stBold.Render(title+":") + " " + strings.Join(parts, "  ")
	if len(m.diags) > 0 {
		warns := 0
		for _, d := range m.diags {
			if d.Severity == "warning" {
				warns++
			}
		}
		if warns > 0 {
			header += "  " + stYellow.Render(fmt.Sprintf("⚠ %d", warns))
		}
	}
	b.WriteString("\n" + header + "\n\n")

	h := m.bodyHeight()
	end := m.offset + h
	if end > len(m.rows) {
		end = len(m.rows)
	}
	for i := m.offset; i < end; i++ {
		b.WriteString(m.renderRow(i) + "\n")
	}
	for i := end - m.offset; i < h; i++ {
		b.WriteString("\n")
	}

	// footer
	var footer string
	if m.confirm {
		prompt := "Apply this plan?"
		style := stGreen
		if m.cfg.sub == "destroy" || c.destroy > 0 || c.replace > 0 {
			n := c.destroy + c.replace
			prompt = fmt.Sprintf("Apply this plan? (%s will be destroyed)", plural(n, "resource"))
			style = stRedBold
		}
		footer = " " + style.Render(prompt) + stBold.Render("  y") + stDim.Render("es / ") + stBold.Render("n") + stDim.Render("o")
	} else {
		scroll := ""
		if len(m.rows) > h {
			scroll = fmt.Sprintf(" %d/%d ", m.cursor+1, len(m.rows))
		}
		footer = " " + stDim.Render("↑↓ move  ⏎ expand  e/c all  a apply  q quit"+scroll)
	}
	b.WriteString(footer)
	return b.String()
}

func (m model) renderRow(i int) string {
	r := m.rows[i]
	w := m.width - 6
	if w < 20 {
		w = 20
	}
	cursorMark := "  "
	isCursor := i == m.cursor
	if isCursor {
		cursorMark = stCyan.Render("│ ")
	}

	var line string
	switch r.kind {
	case rowBlank:
		return ""
	case rowSection:
		return " " + stSection.Render(r.text)
	case rowNode:
		glyph, style := actionGlyph(r.action)
		text := truncRunes(r.text, w)
		if isCursor {
			line = " " + cursorMark + style.Render(glyph) + " " + stCursor.Render(text)
		} else {
			line = " " + cursorMark + style.Render(glyph) + " " + text
		}
		return line
	case rowDetail:
		var style lipgloss.Style
		switch r.sym {
		case '+':
			style = stGreen
		case '-':
			style = stRed
		case '~':
			style = stYellow
		default:
			style = stDim
		}
		text := truncRunes(r.text, w-6)
		out := " " + cursorMark + "    " + style.Render(string(r.sym)+" "+text)
		if r.forces {
			out += " " + stMagenta.Render("# forces replacement")
		}
		return out
	}
	return line
}
