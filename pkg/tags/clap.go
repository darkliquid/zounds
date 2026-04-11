package tags

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/darkliquid/zounds/pkg/core"
)

const clapTaggerVersion = "0.1.0"

var defaultCLAPLabels = []string{
	"ambient", "analogue", "atmospheric", "bass", "bell", "bright", "cinematic",
	"classical", "cyberpunk", "dark", "distorted", "drone", "epic", "glitch",
	"industrial", "lead", "lofi", "metallic", "pad", "percussive", "plucked",
	"retro", "sub", "texture", "vocal",
}

type CLAPTagger struct {
	Endpoint     string
	APIKey       string
	HTTPClient   *http.Client
	Labels       []string
	MaxPredicted int
}

func NewCLAPTagger(endpoint, apiKey string, labels []string) CLAPTagger {
	if len(labels) == 0 {
		labels = append([]string(nil), defaultCLAPLabels...)
	}
	return CLAPTagger{
		Endpoint:     strings.TrimRight(endpoint, "/"),
		APIKey:       apiKey,
		Labels:       append([]string(nil), labels...),
		MaxPredicted: 5,
	}
}

func (CLAPTagger) Name() string {
	return "clap"
}

func (CLAPTagger) Version() string {
	return clapTaggerVersion
}

// CLAPTagger assumes an external CLAP-compatible classifier service and is
// informed by CLAP (Wu et al., 2023) and FineLAP (Li et al., 2026).
func (t CLAPTagger) Tags(ctx context.Context, sample core.Sample, result core.AnalysisResult) ([]core.Tag, error) {
	if strings.TrimSpace(t.Endpoint) == "" {
		return nil, fmt.Errorf("clap tagger endpoint is required")
	}

	payload := map[string]any{
		"path":       sample.Path,
		"filename":   sample.FileName,
		"labels":     t.labels(),
		"max_tags":   t.maxPredicted(),
		"metrics":    result.Metrics,
		"attributes": result.Attributes,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal clap request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.Endpoint+"/classify", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build clap request: %w", err)
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
		return nil, fmt.Errorf("send clap request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("clap request failed: status %s", resp.Status)
	}

	var parsed struct {
		Tags []string `json:"tags"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decode clap response: %w", err)
	}

	out := make([]core.Tag, 0, len(parsed.Tags))
	seen := map[string]struct{}{}
	for _, raw := range parsed.Tags {
		name := core.NormalizeTagName(raw)
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
			Source:         "clap",
			Confidence:     0.8,
		})
		if len(out) == t.maxPredicted() {
			break
		}
	}
	return out, nil
}

func (t CLAPTagger) labels() []string {
	if len(t.Labels) == 0 {
		return append([]string(nil), defaultCLAPLabels...)
	}
	return append([]string(nil), t.Labels...)
}

func (t CLAPTagger) maxPredicted() int {
	if t.MaxPredicted <= 0 {
		return 5
	}
	return t.MaxPredicted
}
