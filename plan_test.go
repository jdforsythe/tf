package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func loadPlan(t *testing.T, name string) *Plan {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	var p Plan
	if err := json.Unmarshal(b, &p); err != nil {
		t.Fatal(err)
	}
	return &p
}

func newTestModel(sub string, autoApprove bool) model {
	m := newModel(config{tfBin: "terraform", sub: sub, planPath: "tfplan", skipPlan: true})
	m.cfg.o.autoApprove = autoApprove
	m.stage = stageShowing
	return m
}

func showPlan(t *testing.T, m model, name string) model {
	t.Helper()
	out, _ := m.handleShow(showMsg{plan: loadPlan(t, name)})
	return out.(model)
}

func TestCountPlanImportOnly(t *testing.T) {
	c := countPlan(loadPlan(t, "import-only.plan.json"))
	if c.imports != 2 || c.add+c.change+c.destroy+c.replace != 0 {
		t.Fatalf("got %+v, want 2 imports and nothing else", c)
	}
	if !c.hasChanges() {
		t.Fatal("import-only plan must count as having changes")
	}
}

func TestCountPlanImportAndUpdate(t *testing.T) {
	c := countPlan(loadPlan(t, "import-and-update.plan.json"))
	if c.imports != 2 || c.change != 1 || c.add != 0 || c.destroy != 0 || c.replace != 0 {
		t.Fatalf("got %+v, want 2 imports and 1 change", c)
	}
}

func TestImportOnlyPlanIsNotEmpty(t *testing.T) {
	m := showPlan(t, newTestModel("apply", false), "import-only.plan.json")
	if m.stage != stageReview {
		t.Fatalf("stage = %v, want review", m.stage)
	}
	for _, l := range m.final {
		if strings.Contains(l, "No changes") {
			t.Fatalf("import-only plan reported as empty: %q", l)
		}
	}
	if m.quitting {
		t.Fatal("must not quit before the approval gate")
	}
}

func TestImportOnlyPlanApprovalRunsApply(t *testing.T) {
	m := showPlan(t, newTestModel("apply", false), "import-only.plan.json")
	var next tea.Model = m
	for _, k := range []string{"a", "y"} {
		next, _ = next.(model).handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)})
	}
	if got := next.(model).stage; got != stageApplying {
		t.Fatalf("stage = %v, want applying", got)
	}
}

func TestImportOnlyPlanAutoApproveRunsApply(t *testing.T) {
	m := showPlan(t, newTestModel("apply", true), "import-only.plan.json")
	if m.stage != stageApplying {
		t.Fatalf("stage = %v, want applying", m.stage)
	}
}

func TestImportOnlyPlanTree(t *testing.T) {
	m := showPlan(t, newTestModel("apply", false), "import-only.plan.json")
	var sections []string
	nodes := 0
	for _, r := range m.rows {
		switch r.kind {
		case rowSection:
			sections = append(sections, r.text)
		case rowNode:
			nodes++
			if r.action != actImport {
				t.Errorf("node action = %q, want %q", r.action, actImport)
			}
		}
	}
	if len(sections) != 1 || sections[0] != "⇣ import (2)" {
		t.Fatalf("sections = %q, want [\"⇣ import (2)\"]", sections)
	}
	if nodes != 2 {
		t.Fatalf("nodes = %d, want 2", nodes)
	}
	for _, n := range m.nodes {
		if n.expanded {
			t.Errorf("%s expanded by default", n.addr)
		}
	}
	m.setAllExpanded(true)
	var detail []string
	for _, r := range m.rows {
		if r.kind == rowDetail {
			detail = append(detail, r.text)
		}
	}
	if len(detail) != 2 || !strings.Contains(detail[0], "11111111-1111-1111-1111-111111111111") {
		t.Fatalf("detail = %q, want one import id line per resource", detail)
	}
}

func TestImportAndUpdateTree(t *testing.T) {
	m := showPlan(t, newTestModel("apply", false), "import-and-update.plan.json")
	var sections []string
	for _, r := range m.rows {
		if r.kind == rowSection {
			sections = append(sections, r.text)
		}
	}
	want := []string{"⇣ import (1)", "～ update (1)"}
	if strings.Join(sections, "|") != strings.Join(want, "|") {
		t.Fatalf("sections = %q, want %q", sections, want)
	}
	var updateTitle string
	for _, r := range m.rows {
		if r.kind == rowNode && r.action == actUpdate {
			updateTitle = r.text
		}
	}
	if !strings.Contains(updateTitle, "imported_and_updated") || !strings.Contains(updateTitle, "import") {
		t.Fatalf("update node should be flagged as an import, got %q", updateTitle)
	}
}

func TestPlanRecapMentionsImports(t *testing.T) {
	m := showPlan(t, newTestModel("plan", false), "import-only.plan.json")
	m.planRecap()
	if got := strings.Join(m.final, "\n"); !strings.Contains(got, "2 to import, 0 to add, 0 to change, 0 to destroy") {
		t.Fatalf("recap = %q", got)
	}

	m = showPlan(t, newTestModel("plan", false), "import-and-update.plan.json")
	m.planRecap()
	if got := strings.Join(m.final, "\n"); !strings.Contains(got, "2 to import, 0 to add, 1 to change, 0 to destroy") {
		t.Fatalf("recap = %q", got)
	}
}

func TestPlanRecapOmitsImportsWhenNone(t *testing.T) {
	m := newTestModel("plan", false)
	m.counts = planCounts{add: 1}
	m.planRecap()
	if got := strings.Join(m.final, "\n"); strings.Contains(got, "import") {
		t.Fatalf("recap = %q, must not mention imports", got)
	}
}

func TestReviewHeaderShowsImports(t *testing.T) {
	m := showPlan(t, newTestModel("apply", false), "import-only.plan.json")
	if got := m.viewReview(); !strings.Contains(got, "2 import") {
		t.Fatalf("header missing import count:\n%s", got)
	}
}

func TestApplyCompleteRecapShowsImported(t *testing.T) {
	m := showPlan(t, newTestModel("apply", true), "import-only.plan.json")
	m.summary = &ChangeSummary{Import: 2}
	out, _ := m.handleExit(exitMsg{phase: "apply", code: 0})
	got := strings.Join(out.(model).final, "\n")
	if !strings.Contains(got, "2 imported, 0 added, 0 changed, 0 destroyed") {
		t.Fatalf("recap = %q", got)
	}
}

func TestEmptyPlanStillReportsNoChanges(t *testing.T) {
	m := newTestModel("apply", false)
	out, _ := m.handleShow(showMsg{plan: &Plan{}})
	m = out.(model)
	if !m.quitting || !strings.Contains(strings.Join(m.final, "\n"), "No changes") {
		t.Fatalf("empty plan should quit with No changes, got final=%q", m.final)
	}
}
