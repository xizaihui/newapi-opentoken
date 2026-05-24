package claude

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

// SanitizeClaudeRawBody patches an Anthropic /v1/messages JSON body in place
// so requests sent through the pass-through path (PassThroughRequestEnabled or
// ChannelSetting.PassThroughBodyEnabled) get the same treatment as the
// converted path.
//
// Fix #a: drops assistant `thinking` content blocks that are missing a
// non-empty `signature`. Anthropic rejects replayed thinking blocks whose
// signature was stripped or rewritten with:
//
//	"messages.N.content.M: Invalid `signature` in `thinking` block"
//
// Fix #c: removes `thinking` and `output_config` when the request model is a
// haiku variant and `thinking.type == "adaptive"`, because haiku rejects
// adaptive thinking with:
//
//	"adaptive thinking is not supported on this model"
//
// The function is fail-open: any parse / marshal error returns the original
// bytes unchanged so a malformed body never blocks the request. The bool
// return indicates whether any change was actually applied.
func SanitizeClaudeRawBody(raw []byte) ([]byte, bool) {
	if len(raw) == 0 {
		return raw, false
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		// Not valid JSON or not a JSON object — let the upstream decide.
		return raw, false
	}

	changed := false

	// Fix #a: walk messages[].content[] and drop unsigned thinking blocks
	// from assistant turns.
	if msgs, ok := payload["messages"].([]interface{}); ok {
		for i, m := range msgs {
			msgMap, ok := m.(map[string]interface{})
			if !ok {
				continue
			}
			role, _ := msgMap["role"].(string)
			if role != "assistant" {
				continue
			}
			arr, ok := msgMap["content"].([]interface{})
			if !ok {
				continue
			}
			filtered := make([]interface{}, 0, len(arr))
			dropped := 0
			for _, block := range arr {
				blockMap, ok := block.(map[string]interface{})
				if !ok {
					filtered = append(filtered, block)
					continue
				}
				if blockType, _ := blockMap["type"].(string); blockType == "thinking" {
					sig, _ := blockMap["signature"].(string)
					if sig == "" {
						dropped++
						continue
					}
				}
				filtered = append(filtered, block)
			}
			if dropped > 0 {
				common.SysLog(fmt.Sprintf("[claude-sanitize] (passthrough) dropped %d unsigned thinking block(s) from assistant message #%d", dropped, i))
				// Anthropic requires at least one block in the array — fall back
				// to a single empty text block to keep the request well-formed.
				if len(filtered) == 0 {
					filtered = []interface{}{map[string]interface{}{"type": "text", "text": " "}}
				}
				msgMap["content"] = filtered
				changed = true
			}
		}
	}

	// Fix #c: haiku doesn't support adaptive thinking — strip the config so
	// the request is accepted as a normal completion call.
	model, _ := payload["model"].(string)
	if strings.Contains(model, "haiku") {
		if thinking, ok := payload["thinking"].(map[string]interface{}); ok {
			if ttype, _ := thinking["type"].(string); ttype == "adaptive" {
				common.SysLog(fmt.Sprintf("[claude-sanitize] (passthrough) dropping adaptive thinking on unsupported model=%s", model))
				delete(payload, "thinking")
				delete(payload, "output_config")
				changed = true
			}
		}
	}

	// Fix #b: Opus 4.7 rejects temperature/top_p/top_k sampling params and
	// context_management. Strip them so the request reaches Anthropic intact.
	if strings.HasPrefix(model, "claude-opus-4-7") {
		for _, k := range []string{"temperature", "top_p", "top_k"} {
			if _, ok := payload[k]; ok {
				delete(payload, k)
				changed = true
			}
		}
		if _, ok := payload["context_management"]; ok {
			delete(payload, "context_management")
			changed = true
		}
		// Fix #f: thinking.enabled → adaptive
		if thinking, ok := payload["thinking"].(map[string]interface{}); ok {
			if ttype, _ := thinking["type"].(string); ttype == "enabled" {
				thinking["type"] = "adaptive"
				if _, hasDisp := thinking["display"]; !hasDisp {
					thinking["display"] = "summarized"
				}
				delete(thinking, "budget_tokens")
				changed = true
			}
		}
		if changed {
			common.SysLog(fmt.Sprintf("[claude-sanitize] (passthrough) opus-4-7 strict params strip on model=%s", model))
		}
	}

	// Fix #d: Sonnet 4.6 / 4.7 / Haiku 4.5 reject temperature+top_p together.
	// Keep temperature, drop top_p.
	if strings.HasPrefix(model, "claude-sonnet-4-6") ||
		strings.HasPrefix(model, "claude-sonnet-4-7") ||
		strings.HasPrefix(model, "claude-haiku-4-5") {
		_, hasTemp := payload["temperature"]
		_, hasTopP := payload["top_p"]
		if hasTemp && hasTopP {
			delete(payload, "top_p")
			common.SysLog(fmt.Sprintf("[claude-sanitize] (passthrough) drop top_p (temp+top_p both set) on model=%s", model))
			changed = true
		}
	}

	if !changed {
		return raw, false
	}

	out, err := json.Marshal(payload)
	if err != nil {
		// Marshal should never fail after a successful unmarshal of a
		// map[string]interface{}, but be defensive: return original on error.
		common.SysLog(fmt.Sprintf("[claude-sanitize] (passthrough) marshal failed, falling back to original: %v", err))
		return raw, false
	}
	return out, true
}
