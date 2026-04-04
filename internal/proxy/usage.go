package proxy

import (
	"encoding/json"
)

// UsageInfo holds token usage extracted from inference responses.
type UsageInfo struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ExtractUsage parses usage information from an OpenAI-compatible response body.
func ExtractUsage(body []byte) *UsageInfo {
	var resp struct {
		Usage *UsageInfo `json:"usage"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil
	}
	return resp.Usage
}
