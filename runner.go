package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
)

type evMsg struct{ ev *Event }

type exitMsg struct {
	phase string // "plan" | "apply"
	code  int
	err   error
}

type showMsg struct {
	plan *Plan
	err  error
}

type tickMsg struct{}

// procHolder lets the (value-copied) tea model signal the live process.
type procHolder struct {
	mu  sync.Mutex
	cmd *exec.Cmd
}

func (p *procHolder) set(c *exec.Cmd) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cmd = c
}

func (p *procHolder) signal(sig os.Signal) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Signal(sig)
	}
}

func (p *procHolder) kill() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
}

// startTF launches terraform with -json streaming output, parsing each line
// into an Event and pushing it onto ch. Sends exitMsg when done.
func startTF(bin string, args []string, phase string, ch chan tea.Msg, holder *procHolder) error {
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), "TF_IN_AUTOMATION=1")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	holder.set(cmd)

	go func() {
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
		for sc.Scan() {
			line := bytes.TrimSpace(sc.Bytes())
			if len(line) == 0 || line[0] != '{' {
				continue
			}
			if ev := parseEvent(line); ev != nil {
				ch <- evMsg{ev}
			}
		}
		werr := cmd.Wait()
		code := 0
		if werr != nil {
			code = 1
			if ee, ok := werr.(*exec.ExitError); ok {
				code = ee.ExitCode()
			}
		}
		if code != 0 {
			if s := strings.TrimSpace(stderr.String()); s != "" {
				ch <- evMsg{&Event{
					Level: "error",
					Type:  "diagnostic",
					Diagnostic: &Diagnostic{
						Severity: "error",
						Summary:  "terraform exited with an error",
						Detail:   s,
					},
				}}
			}
		}
		ch <- exitMsg{phase: phase, code: code, err: werr}
	}()
	return nil
}

func runShowCmd(bin string, globals []string, planPath string) tea.Cmd {
	return func() tea.Msg {
		args := append(append([]string{}, globals...), "show", "-json", planPath)
		cmd := exec.Command(bin, args...)
		cmd.Env = append(os.Environ(), "TF_IN_AUTOMATION=1")
		out, err := cmd.Output()
		if err != nil {
			detail := err.Error()
			if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
				detail = string(ee.Stderr)
			}
			return showMsg{err: &showError{detail}}
		}
		var p Plan
		if err := json.Unmarshal(out, &p); err != nil {
			return showMsg{err: err}
		}
		return showMsg{plan: &p}
	}
}

type showError struct{ s string }

func (e *showError) Error() string { return e.s }

func waitFor(ch chan tea.Msg) tea.Cmd {
	return func() tea.Msg { return <-ch }
}
