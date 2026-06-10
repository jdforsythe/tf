package main

import "encoding/json"

// Event is one line of terraform's machine-readable UI output (-json).
type Event struct {
	Level      string                 `json:"@level"`
	Message    string                 `json:"@message"`
	Type       string                 `json:"type"`
	Hook       *Hook                  `json:"hook"`
	Diagnostic *Diagnostic            `json:"diagnostic"`
	Changes    *ChangeSummary         `json:"changes"`
	Outputs    map[string]*OutputMeta `json:"outputs"`
}

type Hook struct {
	Resource       *HookResource `json:"resource"`
	Action         string        `json:"action"`
	IDKey          string        `json:"id_key"`
	IDValue        any           `json:"id_value"`
	ElapsedSeconds float64       `json:"elapsed_seconds"`
	Provisioner    string        `json:"provisioner"`
}

type HookResource struct {
	Addr         string `json:"addr"`
	Module       string `json:"module"`
	ResourceType string `json:"resource_type"`
	ResourceName string `json:"resource_name"`
}

type Diagnostic struct {
	Severity string     `json:"severity"`
	Summary  string     `json:"summary"`
	Detail   string     `json:"detail"`
	Address  string     `json:"address"`
	Range    *DiagRange `json:"range"`
}

type DiagRange struct {
	Filename string `json:"filename"`
	Start    struct {
		Line int `json:"line"`
	} `json:"start"`
}

type ChangeSummary struct {
	Add       int    `json:"add"`
	Change    int    `json:"change"`
	Import    int    `json:"import"`
	Remove    int    `json:"remove"`
	Operation string `json:"operation"`
}

type OutputMeta struct {
	Sensitive bool            `json:"sensitive"`
	Value     json.RawMessage `json:"value"`
	Action    string          `json:"action"`
}

func parseEvent(line []byte) *Event {
	var ev Event
	if err := json.Unmarshal(line, &ev); err != nil {
		return nil
	}
	return &ev
}
