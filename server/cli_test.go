package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIReportsAndEscaping(t *testing.T) {
	dir := t.TempDir()
	diff := filepath.Join(dir, "change.diff")
	inventory := filepath.Join(dir, "monitors.json")
	output := filepath.Join(dir, "report.json")
	summary := filepath.Join(dir, "report.md")
	in := fixture()
	in.Monitors[0].Name = "<img src=x>"
	raw, _ := json.Marshal(in.Monitors)
	if err := os.WriteFile(diff, []byte(in.Diff), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inventory, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := runCLI(diff, inventory, "demo", output, summary); err != nil {
		t.Fatal(err)
	}
	report, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Mode   string `json:"mode"`
		Report Report `json:"report"`
	}
	if err = json.Unmarshal(report, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Mode != "demo" || envelope.Report.Findings[0].Status != "potential_impact" {
		t.Fatal(string(report))
	}
	text, err := os.ReadFile(summary)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(text), "<img src=x>") || !strings.Contains(string(text), "&lt;img src=x&gt;") {
		t.Fatal("unescaped input in summary")
	}
	if err = runCLI(diff, inventory, "invalid", output, summary); err == nil {
		t.Fatal("accepted invalid mode")
	}
}
func TestCLIInputValidation(t *testing.T) {
	in := fixture()
	in.Monitors = append(in.Monitors, in.Monitors[0])
	if validateInput(in) == nil {
		t.Fatal("accepted duplicate names")
	}
	in = fixture()
	in.Diff = strings.Repeat("x", 100001)
	if validateInput(in) == nil {
		t.Fatal("accepted oversized diff")
	}
}
