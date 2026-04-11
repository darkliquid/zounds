package rename_test

import (
	"testing"

	"github.com/darkliquid/zounds/pkg/core"
	"github.com/darkliquid/zounds/pkg/rename"
)

func TestBuildTemplateDataCollectsTagsBySource(t *testing.T) {
	t.Parallel()

	data := rename.BuildTemplateData(core.Sample{
		ID:           7,
		FileName:     "impact.wav",
		Extension:    "wav",
		Format:       core.FormatWAV,
		Path:         "/samples/impact.wav",
		RelativePath: "impact.wav",
		SourceRoot:   "/samples",
		SizeBytes:    1234,
		Metadata:     map[string]string{"artist": "Example"},
	}, []core.Tag{
		{Name: "Dark", NormalizedName: "dark", Source: "rules"},
		{Name: "Drums", NormalizedName: "drums", Source: "path"},
	}, map[string]string{"key": "C minor"})

	if len(data.Tags) != 2 || data.Tags[0] != "dark" || data.Tags[1] != "drums" {
		t.Fatalf("unexpected tags: %+v", data.Tags)
	}
	if got := data.TagsBySource["rules"]; len(got) != 1 || got[0] != "dark" {
		t.Fatalf("unexpected rules tags: %+v", got)
	}
	if data.Attributes["key"] != "C minor" {
		t.Fatalf("unexpected attributes: %+v", data.Attributes)
	}
}

func TestRenderTemplateUsesHelpersAndFields(t *testing.T) {
	t.Parallel()

	data := rename.BuildTemplateData(core.Sample{
		FileName:  "Huge Pad.wav",
		Extension: "wav",
		Format:    core.FormatWAV,
	}, []core.Tag{
		{Name: "Dark", NormalizedName: "dark", Source: "rules"},
		{Name: "Pad", NormalizedName: "pad", Source: "rules"},
	}, map[string]string{"key": "F# minor"})

	rendered, err := rename.RenderTemplate(`{{slug .Stem}}_{{join .Tags "_" }}_{{lower .Attributes.key}}.{{.Extension}}`, data)
	if err != nil {
		t.Fatalf("render template: %v", err)
	}
	if rendered != "huge-pad_dark_pad_f# minor.wav" {
		t.Fatalf("unexpected rendered template: %q", rendered)
	}
}
