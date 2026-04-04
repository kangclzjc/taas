package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Mock NVIDIA Dynamo Frontend — returns OpenAI-compatible responses without GPU.
func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/v1/chat/completions", handleChatCompletions)
	mux.HandleFunc("/v1/completions", handleCompletions)
	mux.HandleFunc("/v1/embeddings", handleEmbeddings)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	addr := ":8090"
	fmt.Printf("🎭 Mock Dynamo Frontend listening on %s\n", addr)
	fmt.Println("  POST /v1/chat/completions  — mock chat")
	fmt.Println("  POST /v1/completions       — mock completion")
	fmt.Println("  POST /v1/embeddings        — mock embedding")
	http.ListenAndServe(addr, mux)
}

func handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
		MaxTokens   int  `json:"max_tokens"`
		Stream      bool `json:"stream"`
		Temperature float64 `json:"temperature"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "invalid request"}`, 400)
		return
	}

	model := req.Model
	if model == "" {
		model = "mock-model"
	}

	// Simulate processing delay (50-200ms)
	time.Sleep(time.Duration(50+rand.Intn(150)) * time.Millisecond)

	// Get the last user message for mock reply
	userMsg := "Hello"
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == "user" {
			userMsg = req.Messages[i].Content
			break
		}
	}

	reply := generateMockReply(userMsg)
	promptTokens := countTokens(req.Messages)
	completionTokens := len(strings.Fields(reply)) * 2

	if req.Stream {
		handleStreamingResponse(w, model, reply, promptTokens, completionTokens)
		return
	}

	resp := map[string]any{
		"id":      "chatcmpl-" + uuid.New().String()[:8],
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]any{
			{
				"index": 0,
				"message": map[string]string{
					"role":    "assistant",
					"content": reply,
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]int{
			"prompt_tokens":     promptTokens,
			"completion_tokens": completionTokens,
			"total_tokens":      promptTokens + completionTokens,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func handleStreamingResponse(w http.ResponseWriter, model, reply string, promptTokens, completionTokens int) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", 500)
		return
	}

	id := "chatcmpl-" + uuid.New().String()[:8]
	words := strings.Fields(reply)

	for i, word := range words {
		chunk := map[string]any{
			"id":      id,
			"object":  "chat.completion.chunk",
			"created": time.Now().Unix(),
			"model":   model,
			"choices": []map[string]any{
				{
					"index": 0,
					"delta": map[string]string{
						"content": word + " ",
					},
					"finish_reason": nil,
				},
			},
		}
		if i == len(words)-1 {
			chunk["choices"] = []map[string]any{
				{
					"index":         0,
					"delta":         map[string]string{"content": word},
					"finish_reason": "stop",
				},
			}
			chunk["usage"] = map[string]int{
				"prompt_tokens":     promptTokens,
				"completion_tokens": completionTokens,
				"total_tokens":      promptTokens + completionTokens,
			}
		}

		data, _ := json.Marshal(chunk)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
		time.Sleep(time.Duration(30+rand.Intn(70)) * time.Millisecond)
	}

	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

func handleCompletions(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model  string `json:"model"`
		Prompt string `json:"prompt"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	time.Sleep(time.Duration(50+rand.Intn(100)) * time.Millisecond)

	text := "This is a mock completion response for testing purposes. The model would normally generate meaningful text based on the prompt provided."
	promptTokens := len(strings.Fields(req.Prompt)) * 2
	completionTokens := len(strings.Fields(text)) * 2

	resp := map[string]any{
		"id":      "cmpl-" + uuid.New().String()[:8],
		"object":  "text_completion",
		"created": time.Now().Unix(),
		"model":   req.Model,
		"choices": []map[string]any{
			{"index": 0, "text": text, "finish_reason": "stop"},
		},
		"usage": map[string]int{
			"prompt_tokens":     promptTokens,
			"completion_tokens": completionTokens,
			"total_tokens":      promptTokens + completionTokens,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func handleEmbeddings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model string `json:"model"`
		Input any    `json:"input"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	time.Sleep(time.Duration(20+rand.Intn(50)) * time.Millisecond)

	// Generate mock 1536-dim embedding
	embedding := make([]float64, 1536)
	for i := range embedding {
		embedding[i] = rand.Float64()*2 - 1
	}

	resp := map[string]any{
		"object": "list",
		"data": []map[string]any{
			{"object": "embedding", "index": 0, "embedding": embedding},
		},
		"model": req.Model,
		"usage": map[string]int{
			"prompt_tokens": 8,
			"total_tokens":  8,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func generateMockReply(userMsg string) string {
	lower := strings.ToLower(userMsg)
	switch {
	case strings.Contains(lower, "hello") || strings.Contains(lower, "hi"):
		return "Hello! I'm a mock TaaS inference endpoint running without GPU. Everything is working correctly through the full request pipeline: API Gateway → Auth → Token Validation → Rate Limiting → Proxy → Mock Dynamo → Response."
	case strings.Contains(lower, "what") && strings.Contains(lower, "dynamo"):
		return "NVIDIA Dynamo is a framework for deploying large language models with disaggregated prefill and decode stages, enabling efficient GPU utilization and KV cache management across distributed workers."
	case strings.Contains(lower, "test"):
		return "Mock test response received successfully. The TaaS platform is functioning correctly: authentication passed, token validated, rate limits checked, and the request was proxied through to this mock Dynamo backend."
	default:
		return fmt.Sprintf("Mock response for: %q. This confirms the full TaaS inference pipeline is working end-to-end. In production, this would be generated by a real LLM running on NVIDIA Dynamo with GPU acceleration.", userMsg)
	}
}

func countTokens(messages []struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}) int {
	total := 0
	for _, m := range messages {
		total += len(strings.Fields(m.Content))*2 + 4 // rough estimate
	}
	return total
}
