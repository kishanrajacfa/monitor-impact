package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

//go:embed prompt.txt
var prompt string

type Monitor struct {
	Name  string `json:"name"`
	Query string `json:"query"`
}
type Input struct {
	Diff     string    `json:"diff"`
	Monitors []Monitor `json:"monitors"`
	Mode     string    `json:"mode"`
}
type Finding struct {
	Name           string `json:"name"`
	Query          string `json:"query"`
	Status         string `json:"status"`
	Evidence       string `json:"evidence"`
	Reason         string `json:"reason"`
	NextStep       string `json:"next_step"`
	MissingContext string `json:"missing_context"`
}
type Report struct {
	Findings []Finding `json:"findings"`
}

var client = &http.Client{Timeout: 90 * time.Second}
var endpoint = "https://api.openai.com/v1/responses"

func main() {
	diffPath := flag.String("diff", "", "Analyze a diff file instead of starting the web server")
	monitorsPath := flag.String("monitors", "monitors.json", "Monitor JSON inventory")
	mode := flag.String("mode", "ai", "ai or demo")
	output := flag.String("output", "report.json", "JSON report path")
	summary := flag.String("summary", "report.md", "Readable report path")
	flag.Parse()
	if *diffPath != "" {
		if err := runCLI(*diffPath, *monitorsPath, *mode, *output, *summary); err != nil {
			log.Fatal(err)
		}
		return
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]bool{"ai_configured": os.Getenv("OPENAI_API_KEY") != "" && os.Getenv("OPENAI_MODEL") != ""})
	})
	mux.HandleFunc("POST /api/analyze", analyze)
	mux.Handle("/", http.FileServer(http.Dir("web/dist")))
	log.Println("Signal listening at http://127.0.0.1:8080")
	log.Fatal((&http.Server{Addr: "127.0.0.1:8080", Handler: mux, ReadHeaderTimeout: 5 * time.Second}).ListenAndServe())
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
func analyze(w http.ResponseWriter, r *http.Request) {
	var in Input
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 200000))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		fail(w, 400, "Invalid request JSON or input exceeds 200 KB.")
		return
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		fail(w, 400, "Expected a single JSON object.")
		return
	}
	if err := validateInput(in); err != nil {
		fail(w, 400, err.Error())
		return
	}
	var report Report
	var err error
	switch in.Mode {
	case "demo":
		report = demo(in)
	case "ai":
		report, err = modelReport(r.Context(), in)
	default:
		fail(w, 400, "Choose demo or ai mode.")
		return
	}
	if err != nil {
		fail(w, 502, err.Error())
		return
	}
	if err = validate(in, report); err != nil {
		fail(w, 502, "Report failed evidence validation: "+err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"mode": in.Mode, "report": report})
}

// Demo deliberately implements only exact quoted-string matching, not AI reasoning.
func demo(in Input) Report {
	out := Report{Findings: []Finding{}}
	quoted := regexp.MustCompile(`"([^"\n]+)"`)
	var removed, added []string
	for _, line := range strings.Split(in.Diff, "\n") {
		if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			removed = append(removed, line)
		}
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			added = append(added, line)
		}
	}
	for _, m := range in.Monitors {
		f := Finding{Name: m.Name, Query: m.Query, Status: "insufficient_context", Reason: "Demo checks only exact double-quoted strings removed from code and present in a query.", NextStep: "Use AI mode or review this monitor manually.", MissingContext: "Runtime logs, query semantics, and code outside this diff are unavailable."}
		for _, line := range removed {
			for _, match := range quoted.FindAllStringSubmatch(line, -1) {
				s := match[1]
				if strings.Contains(m.Query, s) && !strings.Contains(strings.Join(added, "\n"), s) {
					f.Status = "potential_impact"
					f.Evidence = line
					f.Reason = "A removed quoted string appears in this query and does not appear on an added line. This is a heuristic, not proof of impact."
					f.NextStep = "Confirm whether the event is still emitted, inspect new production logs, then verify or revise the query."
				}
			}
		}
		out.Findings = append(out.Findings, f)
	}
	return out
}
func validate(in Input, out Report) error {
	if len(out.Findings) != len(in.Monitors) {
		return errors.New("expected one assessment per monitor")
	}
	expected := map[string]string{}
	for _, m := range in.Monitors {
		expected[m.Name] = m.Query
	}
	for _, f := range out.Findings {
		q, ok := expected[f.Name]
		if !ok || q != f.Query {
			return errors.New("unknown, duplicate, or altered monitor")
		}
		delete(expected, f.Name)
		if f.Status != "potential_impact" && f.Status != "no_evidence" && f.Status != "insufficient_context" {
			return errors.New("invalid status")
		}
		if f.Status == "potential_impact" && (strings.TrimSpace(f.Evidence) == "" || !strings.Contains(in.Diff, f.Evidence)) {
			return errors.New("impact evidence must quote the supplied diff")
		}
		if strings.TrimSpace(f.Reason) == "" || strings.TrimSpace(f.NextStep) == "" {
			return errors.New("missing explanation or next step")
		}
	}
	return nil
}
func modelReport(ctx context.Context, in Input) (Report, error) {
	var out Report
	key, model := os.Getenv("OPENAI_API_KEY"), os.Getenv("OPENAI_MODEL")
	if key == "" || model == "" {
		return out, errors.New("Set OPENAI_API_KEY and OPENAI_MODEL in the server environment, then restart.")
	}
	properties := map[string]any{}
	fields := []string{"name", "query", "status", "evidence", "reason", "next_step", "missing_context"}
	for _, k := range fields {
		properties[k] = map[string]any{"type": "string"}
	}
	properties["status"] = map[string]any{"type": "string", "enum": []string{"potential_impact", "no_evidence", "insufficient_context"}}
	schema := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"findings"}, "properties": map[string]any{"findings": map[string]any{"type": "array", "items": map[string]any{"type": "object", "additionalProperties": false, "required": fields, "properties": properties}}}}
	data, _ := json.Marshal(map[string]any{"diff": in.Diff, "monitors": in.Monitors})
	body, _ := json.Marshal(map[string]any{"model": model, "instructions": prompt, "input": string(data), "store": false, "max_output_tokens": 12000, "text": map[string]any{"format": map[string]any{"type": "json_schema", "name": "monitor_report", "strict": true, "schema": schema}}})
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return out, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return out, errors.New("Model request failed or timed out. Retry when the connection is available.")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return out, fmt.Errorf("Model API returned HTTP %d. Check model access, credentials, or quota in your server configuration.", resp.StatusCode)
	}
	var envelope struct {
		Status string `json:"status"`
		Output []struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 2000000)).Decode(&envelope); err != nil {
		return out, errors.New("Invalid model response")
	}
	if envelope.Status != "completed" {
		return out, errors.New("Model response was incomplete; try fewer monitors or a smaller diff")
	}
	var result strings.Builder
	for _, item := range envelope.Output {
		for _, c := range item.Content {
			if c.Type == "refusal" {
				return out, errors.New("The model declined this input")
			}
			if c.Type == "output_text" {
				result.WriteString(c.Text)
			}
		}
	}
	if err = json.Unmarshal([]byte(result.String()), &out); err != nil {
		return out, errors.New("Model did not return a usable report")
	}
	return out, nil
}

func validateInput(in Input) error {
	if strings.TrimSpace(in.Diff) == "" || len(in.Diff) > 100000 || len(in.Monitors) == 0 || len(in.Monitors) > 100 {
		return errors.New("Supply a diff up to 100 KB and 1–100 monitors.")
	}
	seen := map[string]bool{}
	for _, m := range in.Monitors {
		if strings.TrimSpace(m.Name) == "" || strings.TrimSpace(m.Query) == "" || seen[m.Name] {
			return errors.New("Each monitor needs a unique name and a nonempty query.")
		}
		seen[m.Name] = true
	}
	return nil
}
