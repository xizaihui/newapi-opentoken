package service

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/dto"
)

func mustUnmarshalClaude(t *testing.T, body string) *dto.ClaudeRequest {
	t.Helper()
	var r dto.ClaudeRequest
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	return &r
}

func TestDetectClaude_PlainNoCacheControl(t *testing.T) {
	req := mustUnmarshalClaude(t, `{
		"model": "claude-opus-4-7",
		"system": "you are helpful",
		"messages": [
			{"role": "user", "content": "hi"}
		]
	}`)
	scan := DetectClaudeCacheControl(req)
	if scan.Declared {
		t.Errorf("expected Declared=false, got scan=%+v", scan)
	}
}

func TestDetectClaude_SystemBlockWithCacheControl(t *testing.T) {
	req := mustUnmarshalClaude(t, `{
		"model": "claude-opus-4-7",
		"system": [
			{"type": "text", "text": "you are helpful", "cache_control": {"type": "ephemeral"}}
		],
		"messages": [{"role": "user", "content": "hi"}]
	}`)
	scan := DetectClaudeCacheControl(req)
	if !scan.Declared || scan.InSystem != 1 || scan.TotalMarks != 1 {
		t.Errorf("expected InSystem=1, got %+v", scan)
	}
}

func TestDetectClaude_MessageContentBlockWithCacheControl(t *testing.T) {
	req := mustUnmarshalClaude(t, `{
		"model": "claude-opus-4-7",
		"messages": [
			{"role": "user", "content": [
				{"type": "text", "text": "long context here", "cache_control": {"type": "ephemeral"}},
				{"type": "text", "text": "second turn"}
			]}
		]
	}`)
	scan := DetectClaudeCacheControl(req)
	if !scan.Declared || scan.InMessages != 1 || scan.TotalMarks != 1 {
		t.Errorf("expected InMessages=1, got %+v", scan)
	}
}

func TestDetectClaude_ToolWithCacheControl(t *testing.T) {
	req := mustUnmarshalClaude(t, `{
		"model": "claude-opus-4-7",
		"tools": [
			{"name": "calc", "description": "...", "input_schema": {}, "cache_control": {"type": "ephemeral"}}
		],
		"messages": [{"role": "user", "content": "hi"}]
	}`)
	scan := DetectClaudeCacheControl(req)
	if !scan.Declared || scan.InTools != 1 || scan.TotalMarks != 1 {
		t.Errorf("expected InTools=1, got %+v", scan)
	}
}

func TestDetectClaude_MultiLocation(t *testing.T) {
	req := mustUnmarshalClaude(t, `{
		"model": "claude-opus-4-7",
		"system": [
			{"type": "text", "text": "sys", "cache_control": {"type": "ephemeral"}}
		],
		"messages": [
			{"role": "user", "content": [
				{"type": "text", "text": "msg1", "cache_control": {"type": "ephemeral"}},
				{"type": "text", "text": "msg2", "cache_control": {"type": "ephemeral"}}
			]}
		],
		"tools": [
			{"name": "t1", "cache_control": {"type": "ephemeral"}}
		]
	}`)
	scan := DetectClaudeCacheControl(req)
	if !scan.Declared {
		t.Fatal("expected Declared=true")
	}
	if scan.InSystem != 1 || scan.InMessages != 2 || scan.InTools != 1 || scan.TotalMarks != 4 {
		t.Errorf("unexpected counts: %+v", scan)
	}
}

func TestDetectClaude_NullCacheControlIgnored(t *testing.T) {
	req := mustUnmarshalClaude(t, `{
		"model": "claude-opus-4-7",
		"system": [
			{"type": "text", "text": "sys", "cache_control": null}
		],
		"messages": [{"role": "user", "content": "hi"}]
	}`)
	scan := DetectClaudeCacheControl(req)
	if scan.Declared {
		t.Errorf("expected Declared=false (null should not count), got %+v", scan)
	}
}

func TestDetectClaude_NilSafe(t *testing.T) {
	scan := DetectClaudeCacheControl(nil)
	if scan.Declared || scan.TotalMarks != 0 {
		t.Errorf("nil should produce zero, got %+v", scan)
	}
}

func TestDetectOpenAI_MessageContentCacheControl(t *testing.T) {
	body := `{
		"model": "claude-opus-4-7",
		"messages": [
			{"role": "user", "content": [
				{"type": "text", "text": "ctx", "cache_control": {"type": "ephemeral"}}
			]}
		]
	}`
	var r dto.GeneralOpenAIRequest
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	scan := DetectOpenAICacheControl(&r)
	if !scan.Declared || scan.InMessages != 1 {
		t.Errorf("expected InMessages=1, got %+v", scan)
	}
}

func TestDetectOpenAI_PlainText(t *testing.T) {
	body := `{
		"model": "gpt-4",
		"messages": [{"role": "user", "content": "hi"}]
	}`
	var r dto.GeneralOpenAIRequest
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	scan := DetectOpenAICacheControl(&r)
	if scan.Declared {
		t.Errorf("expected Declared=false, got %+v", scan)
	}
}

func TestMergeIntoOther_Declared(t *testing.T) {
	s := CacheControlScan{Declared: true, TotalMarks: 3, InSystem: 1, InMessages: 2}
	other := map[string]any{}
	s.MergeIntoOther(other)
	if other["cache_control_declared"] != true {
		t.Errorf("expected declared=true, got %v", other["cache_control_declared"])
	}
	if other["cache_control_marks"] != 3 {
		t.Errorf("expected marks=3, got %v", other["cache_control_marks"])
	}
	if other["cache_control_in_system"] != 1 {
		t.Errorf("expected in_system=1, got %v", other["cache_control_in_system"])
	}
	if other["cache_control_in_messages"] != 2 {
		t.Errorf("expected in_messages=2, got %v", other["cache_control_in_messages"])
	}
	if _, exists := other["cache_control_in_tools"]; exists {
		t.Errorf("should not write in_tools when 0")
	}
}

func TestMergeIntoOther_NotDeclared(t *testing.T) {
	s := CacheControlScan{Declared: false}
	other := map[string]any{}
	s.MergeIntoOther(other)
	if other["cache_control_declared"] != false {
		t.Errorf("expected declared=false written, got %v", other["cache_control_declared"])
	}
	if len(other) != 1 {
		t.Errorf("expected only declared key, got %d keys", len(other))
	}
}
