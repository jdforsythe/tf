package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"
)

// set by goreleaser at release time
var version = "dev"

func main() {
	tfBin := os.Getenv("TF_BIN")
	if tfBin == "" {
		tfBin = "terraform"
	}
	args := os.Args[1:]

	if len(args) == 1 && (args[0] == "--wrapper-version" || args[0] == "wrapper-version") {
		fmt.Println("tf wrapper", version)
		return
	}

	subIdx := -1
	for i, a := range args {
		if !strings.HasPrefix(a, "-") {
			subIdx = i
			break
		}
	}

	interactive := term.IsTerminal(int(os.Stdout.Fd())) && term.IsTerminal(int(os.Stdin.Fd()))
	if subIdx == -1 || !interactive {
		passthrough(tfBin, args)
	}

	sub := args[subIdx]
	switch sub {
	case "plan", "apply", "destroy":
	default:
		passthrough(tfBin, args)
	}

	o := parseArgs(sub, args[subIdx+1:])
	o.globals = args[:subIdx]

	cfg := config{tfBin: tfBin, sub: sub, o: o}
	switch {
	case o.planFileArg != "":
		cfg.planPath = o.planFileArg
		cfg.skipPlan = true
		cfg.keepPlan = true
	case o.planOut != "":
		cfg.planPath = o.planOut
		cfg.keepPlan = true
	default:
		cfg.planPath = filepath.Join(os.TempDir(), fmt.Sprintf("tf-pretty-%d.tfplan", os.Getpid()))
	}

	m := newModel(cfg)
	p := tea.NewProgram(m, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, "tf:", err)
		os.Exit(1)
	}

	fm := finalModel.(model)
	if !cfg.keepPlan {
		_ = os.Remove(cfg.planPath)
	}

	for _, line := range fm.final {
		fmt.Println(line)
	}
	os.Exit(fm.exitCode)
}

// passthrough hands the invocation to the real terraform untouched.
func passthrough(tfBin string, args []string) {
	cmd := exec.Command(tfBin, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err == nil {
		os.Exit(0)
	}
	if ee, ok := err.(*exec.ExitError); ok {
		os.Exit(ee.ExitCode())
	}
	fmt.Fprintln(os.Stderr, "tf:", err)
	os.Exit(1)
}
