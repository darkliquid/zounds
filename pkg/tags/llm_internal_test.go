package tags

import "testing"

func TestParseLLMTagsDedupesAndLimits(t *testing.T) {
	t.Parallel()

	got := parseLLMTags("Cyberpunk, dark, cyberpunk, glitch", 2)
	if len(got) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(got))
	}
	if got[0].NormalizedName != "cyberpunk" {
		t.Fatalf("expected first tag cyberpunk, got %q", got[0].NormalizedName)
	}
	if got[1].NormalizedName != "dark" {
		t.Fatalf("expected second tag dark, got %q", got[1].NormalizedName)
	}
}
