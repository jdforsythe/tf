package main

// Structures for `terraform show -json <planfile>` output. Only the fields
// the review tree needs.

type Plan struct {
	ResourceChanges []*ResourceChange      `json:"resource_changes"`
	OutputChanges   map[string]*ChangeRepr `json:"output_changes"`
	ResourceDrift   []*ResourceChange      `json:"resource_drift"`
}

type ResourceChange struct {
	Address      string      `json:"address"`
	Change       *ChangeRepr `json:"change"`
	ActionReason string      `json:"action_reason"`
}

// Importing is set on a resource change that terraform will import into
// state. An import-only change has actions ["no-op"]; an import combined with
// a configuration change carries that change's actions as well.
type Importing struct {
	ID string `json:"id"`
}

type ChangeRepr struct {
	Actions         []string   `json:"actions"`
	Before          any        `json:"before"`
	After           any        `json:"after"`
	AfterUnknown    any        `json:"after_unknown"`
	BeforeSensitive any        `json:"before_sensitive"`
	AfterSensitive  any        `json:"after_sensitive"`
	ReplacePaths    [][]any    `json:"replace_paths"`
	Importing       *Importing `json:"importing"`
}

const (
	actCreate  = "create"
	actUpdate  = "update"
	actDelete  = "delete"
	actReplace = "replace"
	actRead    = "read"
	actNoop    = "no-op"
	// actImport is not a terraform action: it is how an import-only change
	// (actions ["no-op"] plus `importing`) is grouped in the review tree.
	actImport = "import"
)

func actionOf(actions []string) string {
	if len(actions) == 2 {
		return actReplace
	}
	if len(actions) == 1 {
		switch actions[0] {
		case "create", "update", "delete", "read", "no-op":
			return actions[0]
		}
	}
	return actNoop
}

// action is the review-tree action of a resource change: like actionOf, but
// an import-only change is actImport rather than a no-op.
func (rc *ResourceChange) action() string {
	a := actionOf(rc.Change.Actions)
	if a == actNoop && rc.Change.Importing != nil {
		return actImport
	}
	return a
}

// counts of real changes in a plan. imports counts every resource being
// imported, including ones that are also updated or replaced, matching
// terraform's "N to import".
type planCounts struct {
	add, change, destroy, replace, imports int
}

func countPlan(p *Plan) planCounts {
	var c planCounts
	for _, rc := range p.ResourceChanges {
		if rc.Change.Importing != nil {
			c.imports++
		}
		switch actionOf(rc.Change.Actions) {
		case actCreate:
			c.add++
		case actUpdate:
			c.change++
		case actDelete:
			c.destroy++
		case actReplace:
			c.replace++
		}
	}
	return c
}

// total is the number of resources that report per-resource progress while
// applying. Imports are excluded: terraform emits no apply hooks for them.
func (c planCounts) total() int { return c.add + c.change + c.destroy + c.replace }

func (c planCounts) hasChanges() bool { return c.total()+c.imports > 0 }
