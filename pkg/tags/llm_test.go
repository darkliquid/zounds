package tags_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/darkliquid/zounds/pkg/core"
	"github.com/darkliquid/zounds/pkg/tags"
)

func TestLLMTaggerParsesOpenAICompatibleResponse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Fatalf("unexpected authorization header %q", got)
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload["model"] != "gpt-test" {
			t.Fatalf("unexpected model %#v", payload["model"])
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"content": "cyberpunk, dark, glitch",
					},
				},
			},
		})
	}))
	defer server.Close()

	tagger := tags.NewLLMTagger(server.URL, "secret", "gpt-test")
	tagger.HTTPClient = server.Client()

	got, err := tagger.Tags(context.Background(), core.Sample{
		Path:     "/samples/synth/impact.wav",
		FileName: "impact.wav",
	}, core.AnalysisResult{
		Metrics: map[string]float64{"spectral_centroid_hz": 1240},
	})
	if err != nil {
		t.Fatalf("Tags returned error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 tags, got %d", len(got))
	}
	if got[0].NormalizedName != "cyberpunk" {
		t.Fatalf("expected first tag cyberpunk, got %q", got[0].NormalizedName)
	}
	if got[0].Source != "llm" {
		t.Fatalf("expected llm source, got %q", got[0].Source)
	}
}

func TestLLMTaggerRequiresEndpointAndModel(t *testing.T) {
	t.Parallel()

	tagger := tags.NewLLMTagger("", "", "")
	if _, err := tagger.Tags(context.Background(), core.Sample{}, core.AnalysisResult{}); err == nil {
		t.Fatal("expected validation error")
	}
}
