package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRewriteRequestReplacesDefaultSystemKeywords(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "string system mentions opencode",
			body: `{"system":"You are OpenCode, an AI coding tool."}`,
			want: "You are Antigravity, an AI coding tool.",
		},
		{
			name: "array system mentions claude code",
			body: `{"system":[{"type":"text","text":"Run as Claude Code."}]}`,
			want: "Run as Antigravity.",
		},
		{
			name: "case insensitive codex",
			body: `{"system":"route this CODEX session"}`,
			want: "route this Antigravity session",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, rewritten := rewriteRequestBody([]byte(tt.body))
			if !rewritten {
				t.Fatalf("rewritten = false, want true")
			}
			if !containsSystemText(t, got, tt.want) {
				t.Fatalf("rewritten body = %s, want system text %q", got, tt.want)
			}
		})
	}
}

func TestBuiltInKeywordPresetCoversMainstreamCodingToolsAndAgents(t *testing.T) {
	defer restoreDefaultFilterConfig(t)
	applyFilterConfig(defaultFilterConfig())

	for _, mapping := range defaultRewriteMappings {
		keyword := mapping.Match
		t.Run(keyword, func(t *testing.T) {
			decision := classifyRequest([]byte(`{"system":"You are ` + keyword + `."}`))
			if !decision.Blocked {
				t.Fatalf("%q was not detected by built-in preset", keyword)
			}
			got, rewritten := rewriteRequestBody([]byte(`{"system":"You are ` + keyword + `."}`))
			if !rewritten || !strings.Contains(string(got), "You are Antigravity.") {
				t.Fatalf("%q rewrite = %s, changed=%v", keyword, got, rewritten)
			}
		})
	}
}

func TestDefaultFilterModeBlocksAndRewriteMustBeSelected(t *testing.T) {
	cfg, err := parseFilterConfigYAML(nil)
	if err != nil {
		t.Fatalf("parse default config: %v", err)
	}
	if cfg.Mode != filterModeBlock {
		t.Fatalf("default mode = %q, want block", cfg.Mode)
	}

	cfg, err = parseFilterConfigYAML([]byte("mode: rewrite\n"))
	if err != nil {
		t.Fatalf("parse rewrite config: %v", err)
	}
	if cfg.Mode != filterModeRewrite {
		t.Fatalf("configured mode = %q, want rewrite", cfg.Mode)
	}
}

func TestLongerBuiltInNamesAreRewrittenBeforeShortAliases(t *testing.T) {
	got, rewritten := rewriteRequestBody([]byte(`{"system":"Run GitHub Copilot CLI and OpenAI Codex."}`))
	if !rewritten {
		t.Fatal("rewritten = false, want true")
	}
	if string(got) != `{"system":"Run Antigravity and Antigravity."}` {
		t.Fatalf("body = %s, want complete product names replaced once", got)
	}
}

func TestRewriteRequestIgnoresKeywordsOutsideSystem(t *testing.T) {
	body := []byte(`{
		"messages":[{"role":"user","content":"please compare OpenCode and Codex"}],
		"input":"Claude Code is mentioned by the user"
	}`)
	got, rewritten := rewriteRequestBody(body)
	if rewritten {
		t.Fatalf("rewritten = true, want false; body=%s", got)
	}
}

func TestTargetModelsFiltering(t *testing.T) {
	defer restoreDefaultFilterConfig(t)

	applyFilterConfig(filterConfig{
		Mode:               filterModeRewrite,
		UseDefaultKeywords: true,
		TargetModels:       []string{"antigravity*", "gemini-2.5-*"},
	})

	// Match antigravity
	if !modelMatchesTargets("antigravity/claude-3-7-sonnet", []string{"antigravity*", "gemini-2.5-*"}) {
		t.Fatal("expected antigravity/claude-3-7-sonnet to match")
	}

	// Match gemini
	if !modelMatchesTargets("gemini-2.5-pro", []string{"antigravity*", "gemini-2.5-*"}) {
		t.Fatal("expected gemini-2.5-pro to match")
	}

	// Non-match openai
	if modelMatchesTargets("gpt-4o", []string{"antigravity*", "gemini-2.5-*"}) {
		t.Fatal("gpt-4o should not match")
	}

	// Empty target models means match all
	if !modelMatchesTargets("gpt-4o", nil) {
		t.Fatal("empty targets should match any model")
	}
}

func TestRewriteRequestAllowsCleanInvalidAndStructuralBodies(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "clean json",
			body: `{"system":"You are Antigravity.","messages":[{"role":"user","content":"hello"}]}`,
		},
		{
			name: "invalid json",
			body: `{`,
		},
		{
			name: "empty body",
			body: ``,
		},
		{
			name: "prompt cache key",
			body: `{"prompt_cache_key":"session-cache","system":"plain"}`,
		},
		{
			name: "metadata user id",
			body: `{"metadata":{"user_id":"user-123"},"system":"plain"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, rewritten := rewriteRequestBody([]byte(tt.body))
			if rewritten {
				t.Fatalf("rewritten = true, want false; body=%s", got)
			}
		})
	}
}

func containsSystemText(t *testing.T, body []byte, want string) bool {
	t.Helper()

	var root any
	if err := json.Unmarshal(body, &root); err != nil {
		t.Fatalf("decode rewritten body: %v", err)
	}

	found := false
	walkJSON(root, func(path []string, value any) bool {
		if len(path) == 0 || path[len(path)-1] != "system" {
			return true
		}
		found = strings.Contains(collectText(value), want)
		return !found
	})
	return found
}
