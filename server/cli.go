package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"os"
	"strings"
	"time"
)

func readBounded(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > limit {
		return nil, fmt.Errorf("%s exceeds %d bytes", path, limit)
	}
	return os.ReadFile(path)
}
func runCLI(diffPath, monitorPath, mode, output, summary string) error {
	diff, err := readBounded(diffPath, 100000)
	if err != nil {
		return err
	}
	raw, err := readBounded(monitorPath, 100000)
	if err != nil {
		return err
	}
	in := Input{Diff: string(diff), Mode: mode}
	if err = json.Unmarshal(raw, &in.Monitors); err != nil {
		return fmt.Errorf("invalid monitor inventory: %w", err)
	}
	if err = validateInput(in); err != nil {
		return err
	}
	var report Report
	switch mode {
	case "demo":
		report = demo(in)
	case "ai":
		ctx, cancel := context.WithTimeout(context.Background(), 95*time.Second)
		defer cancel()
		report, err = modelReport(ctx, in)
	default:
		return fmt.Errorf("mode must be ai or demo")
	}
	if err != nil {
		return err
	}
	if err = validate(in, report); err != nil {
		return fmt.Errorf("report evidence validation failed: %w", err)
	}
	model := "none (local heuristic)"
	if mode == "ai" {
		model = os.Getenv("OPENAI_MODEL")
	}
	data, err := json.MarshalIndent(map[string]any{"mode": mode, "model": model, "report": report}, "", "  ")
	if err != nil {
		return err
	}
	if err = os.WriteFile(output, append(data, '\n'), 0600); err != nil {
		return err
	}
	var text strings.Builder
	fmt.Fprintf(&text, "# Monitor impact report\n\nMode: %s | Model: %s\n\nAdvisory only. Verify findings against code and production logs. No detected impact is not a guarantee of safety.\n\n", mode, html.EscapeString(model))
	for _, f := range report.Findings {
		// Escape model/input text so it cannot inject Markdown links, images or HTML.
		fmt.Fprintf(&text, "<pre>%s\nStatus: %s\n\nQuery: %s\n\nDiff evidence: %s\n\nReason: %s\n\nNext step: %s\n\nMissing context: %s</pre>\n\n", html.EscapeString(f.Name), html.EscapeString(f.Status), html.EscapeString(f.Query), html.EscapeString(f.Evidence), html.EscapeString(f.Reason), html.EscapeString(f.NextStep), html.EscapeString(f.MissingContext))
	}
	if err = os.WriteFile(summary, []byte(text.String()), 0600); err != nil {
		return err
	}
	fmt.Printf("Wrote %s and %s (%s mode, %d monitors)\n", output, summary, mode, len(report.Findings))
	return nil
}
