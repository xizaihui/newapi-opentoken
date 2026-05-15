package claude

import (
	"encoding/json"
	"strings"

	"github.com/QuantumNous/new-api/dto"
)

// SanitizeRequestForAnthropic normalizes a ClaudeRequest before it is sent to
// the Anthropic Claude official API. This function only runs from the claude
// package's adaptor path. AWS Bedrock requests live in relay/channel/aws and
// have a separate adaptor; they are not affected by this code.
//
// Three classes of HTTP 400 errors are handled:
//
//  1. `temperature` and `top_p` cannot both be specified (Claude 4.6+ models).
//  2. `thinking.type=enabled` is rejected by Claude opus-4-7+; the upstream
//     requires adaptive thinking with output_config.effort.
//  3. Historical assistant messages may carry `thinking` blocks whose
//     signature was damaged in transit (missing, null, non-string, or empty).
//     Anthropic rejects those as `Invalid signature in thinking block`. We
//     drop the offending blocks so the request still succeeds.
//
// Sanitization is a best-effort safety net. Channels that opt into pass-through
// mode (channel.pass_through_body_enabled or global PassThroughRequestEnabled)
// bypass this entirely because ConvertClaudeRequest is not invoked.
func SanitizeRequestForAnthropic(request *dto.ClaudeRequest) {
	if request == nil {
		return
	}

	model := request.Model

	// (1) temperature + top_p mutual-exclusion on Claude 4.6+
	if request.Temperature != nil && request.TopP != nil && claudeRejectsTempAndTopP(model) {
		request.TopP = nil
	}

	// (2) thinking.type=enabled -> adaptive on opus-4-7+
	if request.Thinking != nil && request.Thinking.Type == "enabled" &&
		strings.HasPrefix(model, "claude-opus-4-7") {
		effort := budgetToEffort(request.Thinking.GetBudgetTokens())
		request.Thinking = &dto.Thinking{
			Type:    "adaptive",
			Display: "summarized",
		}
		if len(request.OutputConfig) == 0 {
			request.OutputConfig = json.RawMessage(`{"effort":"` + effort + `"}`)
		}
		// opus-4-7 with adaptive thinking also rejects non-default sampling.
		request.Temperature = nil
		request.TopP = nil
		request.TopK = nil
	}

	// (3) Strip thinking blocks with damaged signatures from assistant turns.
	stripInvalidThinkingBlocks(request)
}

// claudeRejectsTempAndTopP returns true for models that return HTTP 400 when
// both `temperature` and `top_p` are sent. Anthropic enforces this on the
// 4.6 generation onwards.
func claudeRejectsTempAndTopP(model string) bool {
	switch {
	case strings.HasPrefix(model, "claude-sonnet-4-6"):
		return true
	case strings.HasPrefix(model, "claude-opus-4-6"):
		return true
	case strings.HasPrefix(model, "claude-opus-4-7"):
		return true
	}
	return false
}

// budgetToEffort maps a legacy thinking.budget_tokens value to the
// output_config.effort vocabulary used by adaptive thinking.
func budgetToEffort(budget int) string {
	switch {
	case budget <= 0:
		return "medium"
	case budget <= 2048:
		return "low"
	case budget <= 4096:
		return "medium"
	case budget <= 8192:
		return "high"
	default:
		return "xhigh"
	}
}

// stripInvalidThinkingBlocks scans assistant messages and drops thinking
// blocks whose signature field is missing, null, non-string, or empty.
// Such blocks would be rejected by Claude with `Invalid signature in
// thinking block` 400 errors.
func stripInvalidThinkingBlocks(request *dto.ClaudeRequest) {
	for mi, message := range request.Messages {
		if message.Role != "assistant" {
			continue
		}
		if message.IsStringContent() {
			continue
		}
		cleaned, modified := scrubThinkingFromContent(message.Content)
		if modified {
			request.Messages[mi].Content = cleaned
		}
	}
}

// scrubThinkingFromContent walks over a ClaudeMessage.Content value and
// removes thinking blocks with damaged signatures. It supports both the
// JSON-decoded shape ([]interface{}) and the typed shape
// ([]dto.ClaudeMediaMessage) so it works no matter how the request was
// constructed upstream.
func scrubThinkingFromContent(content any) (any, bool) {
	switch arr := content.(type) {
	case []any:
		cleaned := make([]any, 0, len(arr))
		modified := false
		for _, item := range arr {
			block, ok := item.(map[string]any)
			if !ok {
				cleaned = append(cleaned, item)
				continue
			}
			if !isThinkingBlock(block) {
				cleaned = append(cleaned, item)
				continue
			}
			if isValidSignature(block["signature"]) {
				cleaned = append(cleaned, item)
				continue
			}
			modified = true // drop this block
		}
		if !modified {
			return content, false
		}
		if len(cleaned) == 0 {
			cleaned = append(cleaned, map[string]any{
				"type": "text",
				"text": "...",
			})
		}
		return cleaned, true
	case []dto.ClaudeMediaMessage:
		cleaned := make([]dto.ClaudeMediaMessage, 0, len(arr))
		modified := false
		for _, item := range arr {
			if item.Type != "thinking" {
				cleaned = append(cleaned, item)
				continue
			}
			if strings.TrimSpace(item.Signature) != "" {
				cleaned = append(cleaned, item)
				continue
			}
			modified = true
		}
		if !modified {
			return content, false
		}
		if len(cleaned) == 0 {
			placeholder := "..."
			cleaned = append(cleaned, dto.ClaudeMediaMessage{
				Type: "text",
				Text: &placeholder,
			})
		}
		return cleaned, true
	}
	return content, false
}

func isThinkingBlock(block map[string]any) bool {
	t, _ := block["type"].(string)
	return t == "thinking"
}

func isValidSignature(v any) bool {
	if v == nil {
		return false
	}
	s, ok := v.(string)
	if !ok {
		return false
	}
	return strings.TrimSpace(s) != ""
}
