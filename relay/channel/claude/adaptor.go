package claude

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	channelconstant "github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

type Adaptor struct {
}

func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	// Fix #a: remove empty text blocks AND unsigned thinking blocks (multi-turn replay
	// often drops the original signature, which Anthropic then rejects with 400).
	sanitizeClaudeMessages(request.Messages)
	stripDeprecatedFields(request)
	// Fix #c: haiku models do not support adaptive thinking; strip the config so the
	// upstream returns 200 instead of 400 "adaptive thinking is not supported on this model".
	fixUnsupportedThinking(request)
	return request, nil
}

// sanitizeClaudeMessages mutates in place: removes empty text blocks, drops unsigned
// thinking blocks on assistant turns, and ensures each message has at least one block.
func sanitizeClaudeMessages(messages []dto.ClaudeMessage) {
	for i := range messages {
		msg := &messages[i]
		// String content: empty string -> single space
		if s, ok := msg.Content.(string); ok {
			if s == "" {
				msg.Content = " "
			}
			continue
		}
		// Array content
		arr, ok := msg.Content.([]interface{})
		if !ok {
			continue
		}
		filtered := make([]interface{}, 0, len(arr))
		droppedThinking := 0
		for _, block := range arr {
			blockMap, ok := block.(map[string]interface{})
			if !ok {
				filtered = append(filtered, block)
				continue
			}
			blockType, _ := blockMap["type"].(string)
			if blockType == "text" {
				text, _ := blockMap["text"].(string)
				if text == "" {
					continue
				}
			}
			// Fix #a: drop assistant thinking blocks missing a signature. Anthropic
			// requires the original signature to be replayed verbatim; clients that
			// strip or rewrite it cause "Invalid `signature` in `thinking` block".
			if blockType == "thinking" && msg.Role == "assistant" {
				// Fix #a (aggressive): drop ALL assistant thinking blocks unconditionally.
				// Anthropic rejects any thinking block whose signature was modified
				// in transit (missing, truncated, re-encoded). The signature check
				// alone misses these cases. Thinking is single-turn and clients don't
				// depend on replaying it, so stripping is safe.
				droppedThinking++
				continue
			}
			filtered = append(filtered, block)
		}
		if droppedThinking > 0 {
			common.SysLog(fmt.Sprintf("[claude-sanitize] dropped %d unsigned thinking block(s) from assistant message #%d", droppedThinking, i))
		}
		if len(filtered) == 0 {
			filtered = []interface{}{map[string]interface{}{"type": "text", "text": " "}}
		}
		msg.Content = filtered
	}
}

// fixUnsupportedThinking removes thinking config on models that do not support it.
// Fix #c: haiku-4-5* rejects adaptive thinking (400 "adaptive thinking is not
// supported on this model"). We strip Thinking (and the companion OutputConfig
// effort hint) so the request is accepted as a normal completion call.
func fixUnsupportedThinking(request *dto.ClaudeRequest) {
	if request == nil || request.Thinking == nil {
		return
	}
	model := request.Model
	if strings.Contains(model, "haiku") && request.Thinking.Type == "adaptive" {
		common.SysLog(fmt.Sprintf("[claude-sanitize] dropping adaptive thinking on unsupported model=%s", model))
		request.Thinking = nil
		request.OutputConfig = nil
	}
}

// stripDeprecatedFields removes request fields that Anthropic now rejects.
// Fix #d: 'output_format' was deprecated in favor of 'output_config.format';
// newer SDKs upgraded but legacy clients still send it. We drop it so the
// upstream stops returning 400 deprecated-field errors.
func stripDeprecatedFields(request *dto.ClaudeRequest) {
	if request == nil {
		return
	}
	if len(request.OutputFormat) > 0 {
		common.SysLog("[claude-sanitize] stripping deprecated output_format field")
		request.OutputFormat = nil
	}
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	// Fall back to the default Anthropic base URL when the channel has no base_url
	// configured (the test-channel path constructs RelayInfo without applying the
	// channel-type default, so an empty value would otherwise produce a malformed
	// request URL like "/v1/messages").
	baseURL := info.ChannelBaseUrl
	if baseURL == "" {
		baseURL = channelconstant.ChannelBaseURLs[channelconstant.ChannelTypeAnthropic]
	}
	requestURL := fmt.Sprintf("%s/v1/messages", baseURL)
	if !shouldAppendClaudeBetaQuery(info) {
		return requestURL, nil
	}

	parsedURL, err := url.Parse(requestURL)
	if err != nil {
		return "", err
	}
	query := parsedURL.Query()
	query.Set("beta", "true")
	parsedURL.RawQuery = query.Encode()
	return parsedURL.String(), nil
}

func shouldAppendClaudeBetaQuery(info *relaycommon.RelayInfo) bool {
	if info == nil {
		return false
	}
	if info.IsClaudeBetaQuery {
		return true
	}
	if info.ChannelOtherSettings.ClaudeBetaQuery {
		return true
	}
	return false
}

func CommonClaudeHeadersOperation(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) {
	// common headers operation
	anthropicBeta := c.Request.Header.Get("anthropic-beta")
	if anthropicBeta != "" {
		req.Set("anthropic-beta", anthropicBeta)
	}
	model_setting.GetClaudeSettings().WriteHeaders(info.OriginModelName, req)
	// Note: Bedrock beta flag filtering is now done only in AWS adaptor (aws/dto.go).
	// Anthropic-type channels (type=14) may proxy to either Anthropic API or Bedrock,
	// and we cannot assume Bedrock constraints apply here.
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("x-api-key", info.ApiKey)
	anthropicVersion := c.Request.Header.Get("anthropic-version")
	if anthropicVersion == "" {
		anthropicVersion = "2023-06-01"
	}
	req.Set("anthropic-version", anthropicVersion)
	CommonClaudeHeadersOperation(c, req, info)
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	return RequestOpenAI2ClaudeMessage(c, *request)
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, nil
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	// TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	info.FinalRequestRelayFormat = types.RelayFormatClaude
	if info.IsStream {
		return ClaudeStreamHandler(c, resp, info)
	} else {
		return ClaudeHandler(c, resp, info)
	}
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
