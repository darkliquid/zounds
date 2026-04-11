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

func TestCLAPTaggerParsesClassifierResponse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/classify" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload["filename"] != "impact.wav" {
			t.Fatalf("unexpected filename payload: %#v", payload)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tags": []string{"Cyberpunk", "dark", "cyberpunk"},
		})
	}))
	defer server.Close()

	tagger := tags.NewCLAPTagger(server.URL, "secret", []string{"cyberpunk", "dark"})
	tagger.HTTPClient = server.Client()

	got, err := tagger.Tags(context.Background(), core.Sample{
		Path:     "/tmp/impact.wav",
		FileName: "impact.wav",
	}, core.AnalysisResult{
		Metrics: map[string]float64{"spectral_flatness": 0.3},
	})
	if err != nil {
		t.Fatalf("tagger.Tags returned error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(got))
	}
	if got[0].NormalizedName != "cyberpunk" || got[1].NormalizedName != "dark" {
		t.Fatalf("unexpected tags %#v", got)
	}
}

func TestCLAPTaggerRequiresEndpoint(t *testing.T) {
	t.Parallel()

	tagger := tags.NewCLAPTagger("", "", nil)
	if _, err := tagger.Tags(context.Background(), core.Sample{}, core.AnalysisResult{}); err == nil {
		t.Fatal("expected endpoint validation error")
	}
}
