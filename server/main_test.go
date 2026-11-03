package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func fixture() Input {
	return Input{Diff: "- log.Info(\"old event\")\n+ log.Info(\"new event\")", Monitors: []Monitor{{Name: "coffee", Query: "logs(\"old event\")"}}, Mode: "demo"}
}
func TestDemoAndEvidence(t *testing.T) {
	in := fixture()
	out := demo(in)
	if out.Findings[0].Status != "potential_impact" {
		t.Fatal(out)
	}
	if err := validate(in, out); err != nil {
		t.Fatal(err)
	}
	out.Findings[0].Evidence = "fabricated"
	if validate(in, out) == nil {
		t.Fatal("accepted invented evidence")
	}
}
func TestReaddedString(t *testing.T) {
	in := fixture()
	in.Diff += "\n+ log.Info(\"old event\")"
	if demo(in).Findings[0].Status == "potential_impact" {
		t.Fatal("flagged text still present")
	}
}
func TestValidation(t *testing.T) {
	in := fixture()
	out := demo(in)
	out.Findings = append(out.Findings, out.Findings[0])
	if validate(in, out) == nil {
		t.Fatal("accepted duplicate")
	}
	out = demo(in)
	out.Findings[0].Query = "changed"
	if validate(in, out) == nil {
		t.Fatal("accepted altered query")
	}
}
func TestInvalidRequest(t *testing.T) {
	r := httptest.NewRequest("POST", "/api/analyze", strings.NewReader(`{"diff":"x","monitors":[] ,"mode":"demo"}`))
	w := httptest.NewRecorder()
	analyze(w, r)
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
}
func TestModelContract(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test")
	t.Setenv("OPENAI_MODEL", "test-model")
	in := fixture()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["store"] != false || r.Header.Get("Authorization") != "Bearer test" {
			t.Error("request contract")
		}
		report, _ := json.Marshal(demo(in))
		json.NewEncoder(w).Encode(map[string]any{"status": "completed", "output": []any{map[string]any{"content": []any{map[string]any{"type": "output_text", "text": string(report)}}}}})
	}))
	defer s.Close()
	prior := endpoint
	endpoint = s.URL
	defer func() { endpoint = prior }()
	out, err := modelReport(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if err = validate(in, out); err != nil {
		t.Fatal(err)
	}
}

func TestRepairInvalidEvidenceOnce(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test")
	t.Setenv("OPENAI_MODEL", "test-model")
	in := fixture()
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		out := demo(in)
		if calls == 1 {
			out.Findings[0].Evidence = "invented evidence"
		} else if !strings.Contains(body["input"].(string), "validation_error") {
			t.Error("missing repair feedback")
		}
		report, _ := json.Marshal(out)
		json.NewEncoder(w).Encode(map[string]any{"status": "completed", "output": []any{map[string]any{"content": []any{map[string]any{"type": "output_text", "text": string(report)}}}}})
	}))
	defer s.Close()
	prior := endpoint
	endpoint = s.URL
	defer func() { endpoint = prior }()
	out, err := modelReport(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("expected two calls, got %d", calls)
	}
	if err = validate(in, out); err != nil {
		t.Fatal(err)
	}
}

func TestRepairStillRejectsInventedEvidence(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test")
	t.Setenv("OPENAI_MODEL", "test-model")
	in := fixture()
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		out := demo(in)
		out.Findings[0].Evidence = "invented"
		raw, _ := json.Marshal(out)
		json.NewEncoder(w).Encode(map[string]any{"status": "completed", "output": []any{map[string]any{"content": []any{map[string]any{"type": "output_text", "text": string(raw)}}}}})
	}))
	defer s.Close()
	prior := endpoint
	endpoint = s.URL
	defer func() { endpoint = prior }()
	if _, err := modelReport(context.Background(), in); err == nil {
		t.Fatal("accepted invalid repair")
	}
	if calls != 2 {
		t.Fatalf("unbounded retry: %d", calls)
	}
}
