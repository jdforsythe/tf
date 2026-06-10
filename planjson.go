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

type ChangeRepr struct {
	Actions         []string `json:"actions"`
	Before          any      `json:"before"`
	After           any      `json:"after"`
	AfterUnknown    any      `json:"after_unknown"`
	BeforeSensitive any      `json:"before_sensitive"`
	AfterSensitive  any      `json:"after_sensitive"`
	ReplacePaths    [][]any  `json:"replace_paths"`
}

const (
	actCreate  = "create"
	actUpdate  = "update"
	actDelete  = "delete"
	actReplace = "replace"
	actRead    = "read"
	actNoop    = "no-op"
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

// counts of real changes in a plan
type planCounts struct {
	add, change, destroy, replace int
}

func countPlan(p *Plan) planCounts {
	var c planCounts
	for _, rc := range p.ResourceChanges {
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

func (c planCounts) total() int { return c.add + c.change + c.destroy + c.replace }
