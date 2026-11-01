package rename_test

import (
	"os"
	"path/filepath"
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

func TestSlugSanitizesNamespacesAndPathTraversal(t *testing.T) {
	t.Parallel()

	data := rename.BuildTemplateData(core.Sample{
		FileName:  "kick.v2.wav",
		Extension: "wav",
		Format:    core.FormatWAV,
	}, []core.Tag{
		{Name: "instrument:sub_bass", NormalizedName: "instrument:sub bass", Source: "rules"},
	}, map[string]string{
		"unsafe": "../../etc/passwd:evil*name?",
	})

	rendered, err := rename.RenderTemplate(`{{slug (index .Tags 0)}}_{{slug .Attributes.unsafe}}_{{slug .Stem}}.{{.Extension}}`, data)
	if err != nil {
		t.Fatalf("render template: %v", err)
	}
	if rendered != "instrument-sub-bass_etc-passwd-evil-name_kick.v2.wav" {
		t.Fatalf("unexpected rendered template: %q", rendered)
	}
}

func TestResolveTargetPreservesExtensionForDottedStemAndRejectsTraversal(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	sample := core.Sample{
		SourceRoot: root,
		Path:       filepath.Join(root, "Drums", "kick.v2.wav"),
		FileName:   "kick.v2.wav",
		Extension:  "wav",
		Format:     core.FormatWAV,
	}

	dest, err := rename.ResolveTarget(sample, "renamed/kick.v2")
	if err != nil {
		t.Fatalf("ResolveTarget dotted stem: %v", err)
	}
	want := filepath.Join(root, "renamed", "kick.v2.wav")
	if dest != want {
		t.Fatalf("ResolveTarget = %q, want %q", dest, want)
	}

	if _, err := rename.ResolveTarget(sample, "../outside/kick.wav"); err == nil {
		t.Fatal("expected error when relative rename target escapes SourceRoot")
	}
}

func TestValidatePlanRejectsDuplicateTargetsAndExistingFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	existingTarget := filepath.Join(root, "existing.wav")
	if err := os.WriteFile(existingTarget, []byte("audio"), 0o644); err != nil {
		t.Fatalf("write existing target: %v", err)
	}

	dupOps := []rename.RenameOp{
		{Sample: core.Sample{ID: 1, Path: filepath.Join(root, "a.wav")}, Destination: filepath.Join(root, "out", "same.wav")},
		{Sample: core.Sample{ID: 2, Path: filepath.Join(root, "b.wav")}, Destination: filepath.Join(root, "out", "same.wav")},
	}
	if err := rename.ValidatePlan(dupOps); err == nil {
		t.Fatal("expected duplicate destination error")
	}

	overwriteOps := []rename.RenameOp{
		{Sample: core.Sample{ID: 1, Path: filepath.Join(root, "a.wav")}, Destination: existingTarget},
	}
	if err := rename.ValidatePlan(overwriteOps); err == nil {
		t.Fatal("expected existing destination overwrite error")
	}
}
