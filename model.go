package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type stage int

const (
	stagePlanning stage = iota
	stageShowing
	stageReview
	stageApplying
	stageDone
)

type taskStatus int

const (
	taskRunning taskStatus = iota
	taskDone
	taskFailed
)

type task struct {
	addr    string
	action  string // hook action: create/update/delete/read/refresh/...
	status  taskStatus
	start   time.Time
	doneAt  time.Time
	elapsed float64 // from terraform, seconds
	note    string  // e.g. provisioner name
}

const doneLinger = 1500 * time.Millisecond

type config struct {
	tfBin    string
	sub      string // plan | apply | destroy
	o        opts
	planPath string
	keepPlan bool
	skipPlan bool // apply was given an existing plan file
}

// tree node in the review stage
type node struct {
	addr     string
	action   string
	reason   string
	expanded bool
	detail   []diffLine
}

type rowKind int

const (
	rowBlank rowKind = iota
	rowSection
	rowNode
	rowDetail
)

type row struct {
	kind    rowKind
	text    string
	nodeIdx int
	sym     byte   // for detail rows
	action  string // for node rows
	forces  bool
}

type model struct {
	cfg    config
	events chan tea.Msg
	proc   *procHolder

	stage  stage
	width  int
	height int
	spin   int

	// progress stages (plan refresh + apply)
	tasks       map[string]*task
	order       []string
	startedAt   time.Time
	completed   int
	failed      int
	total       int // apply only: planned change count
	refreshed   int
	plannedSeen int
	diags       []*Diagnostic
	summary     *ChangeSummary
	outputs     map[string]*OutputMeta

	// review stage
	plan    *Plan
	counts  planCounts
	nodes   []*node
	rows    []row
	cursor  int
	offset  int
	confirm bool

	// lifecycle
	interrupted bool
	applied     bool
	quitting    bool
	exitCode    int
	final       []string // recap printed after the TUI exits
}

func newModel(cfg config) model {
	m := model{
		cfg:     cfg,
		events:  make(chan tea.Msg, 1024),
		proc:    &procHolder{},
		tasks:   map[string]*task{},
		outputs: map[string]*OutputMeta{},
		width:   100,
		height:  30,
	}
	if cfg.skipPlan {
		m.stage = stageShowing
	} else {
		m.stage = stagePlanning
	}
	m.startedAt = time.Now()
	return m
}

func tickCmd() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg { return tickMsg{} })
}

func (m model) Init() tea.Cmd {
	if m.cfg.skipPlan {
		return tea.Batch(runShowCmd(m.cfg.tfBin, m.cfg.o.globals, m.cfg.planPath), tickCmd())
	}
	return tea.Batch(m.startPlanCmd(), waitFor(m.events), tickCmd())
}

func (m model) startPlanCmd() tea.Cmd {
	args := append([]string{}, m.cfg.o.globals...)
	args = append(args, "plan", "-input=false", "-json", "-out="+m.cfg.planPath)
	if m.cfg.sub == "destroy" {
		args = append(args, "-destroy")
	}
	args = append(args, m.cfg.o.shared...)
	args = append(args, m.cfg.o.planArgs...)
	return func() tea.Msg {
		if err := startTF(m.cfg.tfBin, args, "plan", m.events, m.proc); err != nil {
			return exitMsg{phase: "plan", code: 1, err: err}
		}
		return nil
	}
}

func (m model) startApplyCmd() tea.Cmd {
	args := append([]string{}, m.cfg.o.globals...)
	args = append(args, "apply", "-input=false", "-json")
	args = append(args, m.cfg.o.shared...)
	args = append(args, m.cfg.planPath)
	return func() tea.Msg {
		if err := startTF(m.cfg.tfBin, args, "apply", m.events, m.proc); err != nil {
			return exitMsg{phase: "apply", code: 1, err: err}
		}
		return nil
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if msg.Width > 0 {
			m.width = msg.Width
		}
		if msg.Height > 0 {
			m.height = msg.Height
		}
		m.clampScroll()
		return m, nil

	case tickMsg:
		m.spin++
		m.reapTasks()
		if m.stage == stagePlanning || m.stage == stageShowing || m.stage == stageApplying {
			return m, tickCmd()
		}
		return m, nil

	case evMsg:
		m.handleEvent(msg.ev)
		return m, waitFor(m.events)

	case exitMsg:
		return m.handleExit(msg)

	case showMsg:
		return m.handleShow(msg)

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *model) handleEvent(ev *Event) {
	switch {
	case ev.Type == "diagnostic" && ev.Diagnostic != nil:
		m.diags = append(m.diags, ev.Diagnostic)
	case ev.Type == "change_summary" && ev.Changes != nil:
		m.summary = ev.Changes
	case ev.Type == "planned_change":
		m.plannedSeen++
	case ev.Type == "outputs":
		for k, v := range ev.Outputs {
			m.outputs[k] = v
		}
	case ev.Hook != nil && ev.Hook.Resource != nil:
		addr := ev.Hook.Resource.Addr
		switch {
		case strings.HasSuffix(ev.Type, "_start"):
			action := ev.Hook.Action
			if action == "" {
				if strings.HasPrefix(ev.Type, "refresh") {
					action = "refresh"
				} else if strings.HasPrefix(ev.Type, "provision") {
					action = "provision"
				}
			}
			if strings.HasPrefix(ev.Type, "provision") {
				if t, ok := m.tasks[addr]; ok {
					t.note = "provisioning"
				}
				return
			}
			if _, ok := m.tasks[addr]; !ok {
				m.order = append(m.order, addr)
			}
			m.tasks[addr] = &task{addr: addr, action: action, status: taskRunning, start: time.Now()}
		case strings.HasSuffix(ev.Type, "_complete"):
			if strings.HasPrefix(ev.Type, "provision") {
				if t, ok := m.tasks[addr]; ok {
					t.note = ""
				}
				return
			}
			if t, ok := m.tasks[addr]; ok {
				t.status = taskDone
				t.doneAt = time.Now()
				t.elapsed = ev.Hook.ElapsedSeconds
			}
			if ev.Type == "refresh_complete" {
				m.refreshed++
			}
			if ev.Type == "apply_complete" && m.stage == stageApplying {
				m.completed++
			}
		case strings.HasSuffix(ev.Type, "_errored"):
			if t, ok := m.tasks[addr]; ok {
				t.status = taskFailed
				t.doneAt = time.Now()
				t.elapsed = ev.Hook.ElapsedSeconds
			} else {
				m.order = append(m.order, addr)
				m.tasks[addr] = &task{addr: addr, action: ev.Hook.Action, status: taskFailed, start: time.Now(), doneAt: time.Now()}
			}
			if m.stage == stageApplying {
				m.failed++
			}
		}
	}
}

func (m *model) reapTasks() {
	now := time.Now()
	keep := m.order[:0]
	for _, addr := range m.order {
		t := m.tasks[addr]
		if t.status == taskDone && now.Sub(t.doneAt) > doneLinger {
			delete(m.tasks, addr)
			continue
		}
		keep = append(keep, addr)
	}
	m.order = keep
}

func (m model) handleExit(msg exitMsg) (tea.Model, tea.Cmd) {
	switch msg.phase {
	case "plan":
		if msg.code != 0 {
			m.exitCode = 1
			if m.interrupted {
				m.final = append(m.final, stYellow.Render("Plan interrupted."))
			} else {
				m.final = append(m.final, stRedBold.Render("✗ Plan failed."))
			}
			m.appendDiagRecap()
			m.quitting = true
			m.stage = stageDone
			return m, tea.Quit
		}
		m.stage = stageShowing
		return m, runShowCmd(m.cfg.tfBin, m.cfg.o.globals, m.cfg.planPath)

	case "apply":
		m.stage = stageDone
		m.quitting = true
		elapsed := time.Since(m.startedAt)
		if msg.code != 0 {
			m.exitCode = 1
			if m.interrupted {
				m.final = append(m.final, stYellow.Render(fmt.Sprintf("Apply interrupted after %s. %d completed, %d failed.", fmtDur(elapsed), m.completed, m.failed)))
			} else {
				m.final = append(m.final, stRedBold.Render(fmt.Sprintf("✗ Apply failed after %s. %d completed, %d failed.", fmtDur(elapsed), m.completed, m.failed)))
			}
		} else {
			m.applied = true
			if m.cfg.sub == "destroy" {
				n := m.completed
				if s := m.summary; s != nil {
					n = s.Remove
				}
				m.final = append(m.final, stGreen.Render(fmt.Sprintf("✓ Destroy complete in %s — %s destroyed.", fmtDur(elapsed), plural(n, "resource"))))
			} else {
				if s := m.summary; s != nil {
					m.final = append(m.final, stGreen.Render(fmt.Sprintf("✓ Apply complete in %s — %d added, %d changed, %d destroyed.", fmtDur(elapsed), s.Add, s.Change, s.Remove)))
				} else {
					m.final = append(m.final, stGreen.Render(fmt.Sprintf("✓ Apply complete in %s — %s.", fmtDur(elapsed), plural(m.completed, "resource"))))
				}
				m.appendOutputsRecap()
			}
		}
		m.appendDiagRecap()
		return m, tea.Quit
	}
	return m, nil
}

func (m model) handleShow(msg showMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.exitCode = 1
		m.final = append(m.final, stRedBold.Render("✗ Failed to read plan: "+msg.err.Error()))
		m.quitting = true
		m.stage = stageDone
		return m, tea.Quit
	}
	m.plan = msg.plan
	m.counts = countPlan(msg.plan)
	m.buildTree()

	outputChanges := 0
	for _, ch := range m.plan.OutputChanges {
		if actionOf(ch.Actions) != actNoop {
			outputChanges++
		}
	}

	if m.counts.total() == 0 && outputChanges == 0 {
		m.final = append(m.final, stGreen.Render("✓ No changes. Infrastructure matches the configuration."))
		m.appendDiagRecap()
		m.quitting = true
		m.stage = stageDone
		return m, tea.Quit
	}

	if (m.cfg.sub == "apply" || m.cfg.sub == "destroy") && m.cfg.o.autoApprove {
		return m.beginApply()
	}
	m.stage = stageReview
	return m, nil
}

func (m model) beginApply() (tea.Model, tea.Cmd) {
	m.stage = stageApplying
	m.tasks = map[string]*task{}
	m.order = nil
	m.completed, m.failed = 0, 0
	m.total = m.counts.total()
	m.startedAt = time.Now()
	m.diags = nil
	return m, tea.Batch(m.startApplyCmd(), waitFor(m.events), tickCmd())
}

// ---- review tree ----

func (m *model) buildTree() {
	m.nodes = nil
	add := func(rc *ResourceChange, action string) {
		m.nodes = append(m.nodes, &node{
			addr:   rc.Address,
			action: action,
			reason: prettifyReason(rc.ActionReason),
			detail: diffResource(rc.Change),
		})
	}
	byAction := map[string][]*ResourceChange{}
	for _, rc := range m.plan.ResourceChanges {
		a := actionOf(rc.Change.Actions)
		if a == actNoop {
			continue
		}
		byAction[a] = append(byAction[a], rc)
	}
	for _, a := range []string{actCreate, actUpdate, actReplace, actDelete, actRead} {
		rcs := byAction[a]
		sort.Slice(rcs, func(i, j int) bool { return rcs[i].Address < rcs[j].Address })
		for _, rc := range rcs {
			add(rc, a)
		}
	}
	m.rebuildRows()
}

func sectionTitle(action string, n int) string {
	switch action {
	case actCreate:
		return fmt.Sprintf("＋ create (%d)", n)
	case actUpdate:
		return fmt.Sprintf("～ update (%d)", n)
	case actReplace:
		return fmt.Sprintf("± replace (%d)", n)
	case actDelete:
		return fmt.Sprintf("－ destroy (%d)", n)
	case actRead:
		return fmt.Sprintf("↻ read (%d)", n)
	}
	return action
}

func (m *model) rebuildRows() {
	m.rows = m.rows[:0]
	counts := map[string]int{}
	for _, n := range m.nodes {
		counts[n.action]++
	}
	last := ""
	for i, n := range m.nodes {
		if n.action != last {
			if last != "" {
				m.rows = append(m.rows, row{kind: rowBlank})
			}
			m.rows = append(m.rows, row{kind: rowSection, text: sectionTitle(n.action, counts[n.action]), nodeIdx: -1})
			last = n.action
		}
		caret := "▸"
		if n.expanded {
			caret = "▾"
		}
		title := n.addr
		if n.reason != "" {
			title += "  (" + n.reason + ")"
		}
		m.rows = append(m.rows, row{kind: rowNode, text: caret + " " + title, nodeIdx: i, action: n.action})
		if n.expanded {
			if len(n.detail) == 0 {
				m.rows = append(m.rows, row{kind: rowDetail, text: "(no attribute changes)", nodeIdx: i, sym: ' '})
			}
			for _, d := range n.detail {
				m.rows = append(m.rows, row{kind: rowDetail, text: d.text, nodeIdx: i, sym: d.sym, forces: d.forces})
			}
		}
	}

	// outputs section
	if len(m.plan.OutputChanges) > 0 {
		names := make([]string, 0, len(m.plan.OutputChanges))
		for name, ch := range m.plan.OutputChanges {
			if actionOf(ch.Actions) != actNoop {
				names = append(names, name)
			}
		}
		if len(names) > 0 {
			sort.Strings(names)
			m.rows = append(m.rows, row{kind: rowBlank}, row{kind: rowSection, text: fmt.Sprintf("⇒ outputs (%d)", len(names)), nodeIdx: -1})
			for _, name := range names {
				ch := m.plan.OutputChanges[name]
				sym := byte('~')
				switch actionOf(ch.Actions) {
				case actCreate:
					sym = '+'
				case actDelete:
					sym = '-'
				}
				oldStr, newStr := renderVal(ch.Before), renderVal(ch.After)
				if truePaths(ch.AfterUnknown)[""] {
					newStr = "(known after apply)"
				}
				if b, ok := ch.AfterSensitive.(bool); ok && b {
					newStr = "(sensitive)"
				}
				if b, ok := ch.BeforeSensitive.(bool); ok && b {
					oldStr = "(sensitive)"
				}
				text := name + " = " + newStr
				if sym == '~' {
					text = name + ": " + oldStr + " → " + newStr
				} else if sym == '-' {
					text = name + " (was " + oldStr + ")"
				}
				m.rows = append(m.rows, row{kind: rowDetail, text: text, nodeIdx: -1, sym: sym})
			}
		}
	}

	// drift section
	drift := []*ResourceChange{}
	for _, rc := range m.plan.ResourceDrift {
		if actionOf(rc.Change.Actions) != actNoop {
			drift = append(drift, rc)
		}
	}
	if len(drift) > 0 {
		m.rows = append(m.rows, row{kind: rowBlank}, row{kind: rowSection, text: fmt.Sprintf("≠ drift detected outside terraform (%d)", len(drift)), nodeIdx: -1})
		for _, rc := range drift {
			m.rows = append(m.rows, row{kind: rowDetail, text: rc.Address, nodeIdx: -1, sym: '~'})
		}
	}

	if m.cursor >= len(m.rows) {
		m.cursor = len(m.rows) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	m.clampScroll()
}

func prettifyReason(r string) string {
	if r == "" {
		return ""
	}
	r = strings.TrimPrefix(r, "replace_")
	return strings.ReplaceAll(r, "_", " ")
}

func (m *model) bodyHeight() int {
	h := m.height - 5
	if h < 3 {
		h = 3
	}
	return h
}

func (m *model) clampScroll() {
	h := m.bodyHeight()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+h {
		m.offset = m.cursor - h + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

func (m *model) moveCursor(delta int) {
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.rows) {
		m.cursor = len(m.rows) - 1
	}
	m.clampScroll()
}

func (m *model) toggleAt(cursor int) {
	if cursor < 0 || cursor >= len(m.rows) {
		return
	}
	r := m.rows[cursor]
	if r.nodeIdx < 0 {
		return
	}
	n := m.nodes[r.nodeIdx]
	n.expanded = !n.expanded
	// keep the cursor on the node's header row
	m.rebuildRows()
	for i, rr := range m.rows {
		if rr.kind == rowNode && rr.nodeIdx == r.nodeIdx {
			m.cursor = i
			break
		}
	}
	m.clampScroll()
}

func (m *model) setAllExpanded(v bool) {
	for _, n := range m.nodes {
		n.expanded = v
	}
	m.rebuildRows()
}

// ---- keys ----

func (m model) handleKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := k.String()

	// interrupt handling for running stages
	if key == "ctrl+c" || (key == "q" && m.stage != stageReview) {
		switch m.stage {
		case stagePlanning, stageApplying:
			if !m.interrupted {
				m.interrupted = true
				m.proc.signal(os.Interrupt)
				return m, nil
			}
			m.proc.kill()
			return m, nil
		case stageShowing:
			m.quitting = true
			return m, tea.Quit
		default:
			m.quitting = true
			return m, tea.Quit
		}
	}

	if m.stage != stageReview {
		return m, nil
	}

	if m.confirm {
		switch key {
		case "y", "Y", "enter":
			m.confirm = false
			return m.beginApply()
		default:
			m.confirm = false
			return m, nil
		}
	}

	switch key {
	case "q", "esc":
		m.planRecap()
		m.quitting = true
		return m, tea.Quit
	case "up", "k":
		m.moveCursor(-1)
	case "down", "j":
		m.moveCursor(1)
	case "pgup", "ctrl+u":
		m.moveCursor(-m.bodyHeight())
	case "pgdown", "ctrl+d":
		m.moveCursor(m.bodyHeight())
	case "g", "home":
		m.cursor = 0
		m.clampScroll()
	case "G", "end":
		m.cursor = len(m.rows) - 1
		m.clampScroll()
	case "enter", " ", "l", "h", "right", "left":
		m.toggleAt(m.cursor)
	case "e":
		m.setAllExpanded(true)
	case "c":
		m.setAllExpanded(false)
	case "a":
		m.confirm = true
	}
	return m, nil
}

// ---- recaps ----

func (m *model) planRecap() {
	c := m.counts
	line := fmt.Sprintf("Plan: %d to add, %d to change, %d to destroy", c.add+c.replace, c.change, c.destroy+c.replace)
	if c.replace > 0 {
		line += fmt.Sprintf(" (%d replaced)", c.replace)
	}
	m.final = append(m.final, stBold.Render(line+"."))
	if m.cfg.keepPlan {
		m.final = append(m.final, stDim.Render("Plan saved to "+m.cfg.planPath+" — apply with: tf apply "+m.cfg.planPath))
	}
	m.appendDiagRecap()
}

func (m *model) appendDiagRecap() {
	for _, d := range m.diags {
		m.final = append(m.final, renderDiagLines(d, m.width)...)
	}
}

func (m *model) appendOutputsRecap() {
	if len(m.outputs) == 0 {
		return
	}
	names := make([]string, 0, len(m.outputs))
	for k := range m.outputs {
		names = append(names, k)
	}
	sort.Strings(names)
	var lines []string
	for _, name := range names {
		o := m.outputs[name]
		val := "(sensitive)"
		if !o.Sensitive {
			val = truncRunes(string(o.Value), 120)
		}
		if strings.TrimSpace(val) == "" || val == "null" {
			continue
		}
		lines = append(lines, "  "+stCyan.Render(name)+" = "+val)
	}
	if len(lines) > 0 {
		m.final = append(m.final, stBold.Render("Outputs:"))
		m.final = append(m.final, lines...)
	}
}
