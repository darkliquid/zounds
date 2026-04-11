package tags

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/darkliquid/zounds/pkg/core"
)

const llmTaggerVersion = "0.1.0"

type LLMTagger struct {
	Endpoint     string
	APIKey       string
	Model        string
	HTTPClient   *http.Client
	MaxPredicted int
	SystemPrompt string
}

func NewLLMTagger(endpoint, apiKey, model string) LLMTagger {
	return LLMTagger{
		Endpoint:     strings.TrimRight(endpoint, "/"),
		APIKey:       apiKey,
		Model:        model,
		MaxPredicted: 8,
		SystemPrompt: "You label audio samples. Return only a comma-separated list of short lowercase tags describing vibe, genre, texture, or use. Do not explain.",
	}
}

func (LLMTagger) Name() string {
	return "llm"
}

func (LLMTagger) Version() string {
	return llmTaggerVersion
}

func (t LLMTagger) Tags(ctx context.Context, sample core.Sample, result core.AnalysisResult) ([]core.Tag, error) {
	if strings.TrimSpace(t.Endpoint) == "" {
		return nil, fmt.Errorf("llm tagger endpoint is required")
	}
	if strings.TrimSpace(t.Model) == "" {
		return nil, fmt.Errorf("llm tagger model is required")
	}

	payload := map[string]any{
		"model": t.Model,
		"messages": []map[string]string{
			{
				"role":    "system",
				"content": t.systemPrompt(),
			},
			{
				"role":    "user",
				"content": buildLLMPrompt(sample, result),
			},
		},
		"temperature": 0.2,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal llm request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.Endpoint+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build llm request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(t.APIKey) != "" {
		req.Header.Set("Authorization", "Bearer "+t.APIKey)
	}

	client := t.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send llm request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("llm request failed: status %s", resp.Status)
	}

	var completion chatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&completion); err != nil {
		return nil, fmt.Errorf("decode llm response: %w", err)
	}
	if len(completion.Choices) == 0 {
		return nil, fmt.Errorf("llm response contained no choices")
	}

	return parseLLMTags(completion.Choices[0].Message.Content, t.maxPredicted()), nil
}

type chatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (t LLMTagger) systemPrompt() string {
	if strings.TrimSpace(t.SystemPrompt) != "" {
		return t.SystemPrompt
	}
	return "You label audio samples. Return only a comma-separated list of short lowercase tags describing vibe, genre, texture, or use. Do not explain."
}

func (t LLMTagger) maxPredicted() int {
	if t.MaxPredicted <= 0 {
		return 8
	}
	return t.MaxPredicted
}

func buildLLMPrompt(sample core.Sample, result core.AnalysisResult) string {
	var lines []string
	lines = append(lines,
		"Suggest descriptive tags for this audio sample.",
		"Prefer mood, genre, texture, and instrument/use labels.",
		fmt.Sprintf("path: %s", sample.Path),
		fmt.Sprintf("filename: %s", sample.FileName),
	)

	if len(result.Tags) > 0 {
		existing := make([]string, 0, len(result.Tags))
		for _, tag := range result.Tags {
			name := tag.NormalizedName
			if name == "" {
				name = core.NormalizeTagName(tag.Name)
			}
			if name != "" {
				existing = append(existing, name)
			}
		}
		sort.Strings(existing)
		lines = append(lines, fmt.Sprintf("existing_tags: %s", strings.Join(existing, ", ")))
	}

	if len(result.Attributes) > 0 {
		keys := make([]string, 0, len(result.Attributes))
		for key := range result.Attributes {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			lines = append(lines, fmt.Sprintf("attribute.%s: %s", key, result.Attributes[key]))
		}
	}

	if len(result.Metrics) > 0 {
		keys := make([]string, 0, len(result.Metrics))
		for key := range result.Metrics {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			lines = append(lines, fmt.Sprintf("metric.%s: %.6f", key, result.Metrics[key]))
		}
	}

	return strings.Join(lines, "\n")
}

func parseLLMTags(content string, limit int) []core.Tag {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "[")
	content = strings.TrimSuffix(content, "]")

	parts := strings.FieldsFunc(content, func(r rune) bool {
		switch r {
		case ',', '\n', ';':
			return true
		default:
			return false
		}
	})

	seen := map[string]struct{}{}
	out := make([]core.Tag, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(strings.Trim(part, `"'`))
		name := core.NormalizeTagName(part)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, core.Tag{
			Name:           name,
			NormalizedName: name,
			Source:         "llm",
			Confidence:     0.75,
		})
		if len(out) == limit {
			break
		}
	}
	return out
}
