package commands_test

import (
	"bytes"
	"context"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/zounds/cmd/zounds/commands"
	zaudio "github.com/darkliquid/zounds/pkg/audio"
	"github.com/darkliquid/zounds/pkg/audio/wav"
	"github.com/darkliquid/zounds/pkg/db"
)

func TestTagCommandAutoAppliesPathTags(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "tags.db")
	samplePath := filepath.Join(root, "Drums", "Dark Hits", "impact.wav")
	writeWAVFixture(t, samplePath)

	scan := commands.NewRootCommand()
	scan.SetArgs([]string{"--db", dbPath, "scan", root})
	if err := scan.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute scan: %v", err)
	}

	tagCmd := commands.NewRootCommand()
	var out bytes.Buffer
	tagCmd.SetOut(&out)
	tagCmd.SetErr(&out)
	tagCmd.SetArgs([]string{"--db", dbPath, "tag", "--auto"})
	if err := tagCmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute auto tag: %v", err)
	}
	if !strings.Contains(out.String(), "processed 1 samples") {
		t.Fatalf("unexpected output: %q", out.String())
	}

	repo := openRepoForTest(t, dbPath)
	sample, err := repo.FindSampleByPath(context.Background(), samplePath)
	if err != nil {
		t.Fatalf("find sample: %v", err)
	}
	got, err := repo.ListTagsForSample(context.Background(), sample.ID)
	if err != nil {
		t.Fatalf("list sample tags: %v", err)
	}

	names := make(map[string]struct{}, len(got))
	for _, tag := range got {
		names[tag.NormalizedName] = struct{}{}
	}
	for _, expected := range []string{"drums", "dark", "hits", "impact"} {
		if _, ok := names[expected]; !ok {
			t.Fatalf("missing auto tag %q in %v", expected, names)
		}
	}
}

func TestTagCommandVerboseShowsPerSampleProgress(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "tags-verbose.db")
	samplePath := filepath.Join(root, "Drums", "Dark Hits", "impact.wav")
	writeWAVFixture(t, samplePath)

	scan := commands.NewRootCommand()
	scan.SetArgs([]string{"--db", dbPath, "scan", root})
	if err := scan.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute scan: %v", err)
	}

	tagCmd := commands.NewRootCommand()
	var out bytes.Buffer
	tagCmd.SetOut(&out)
	tagCmd.SetErr(&out)
	tagCmd.SetArgs([]string{"--verbose", "--db", dbPath, "tag", "--auto"})
	if err := tagCmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute auto tag: %v", err)
	}

	output := out.String()
	for _, want := range []string{
		"verbose: tagging sample " + samplePath,
		"verbose: generated ",
		"processed 1 samples and applied ",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected %q in output %q", want, output)
		}
	}
}

func TestTagCommandAutoAppliesRuleTags(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "rules.db")
	samplePath := filepath.Join(root, "Synth", "tone.wav")
	writeSineWAVFixture(t, samplePath, 55, 1.0)

	scan := commands.NewRootCommand()
	scan.SetArgs([]string{"--db", dbPath, "scan", root})
	if err := scan.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute scan: %v", err)
	}

	tagCmd := commands.NewRootCommand()
	tagCmd.SetArgs([]string{"--db", dbPath, "tag", "--auto"})
	if err := tagCmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute auto tag: %v", err)
	}

	repo := openRepoForTest(t, dbPath)
	sample, err := repo.FindSampleByPath(context.Background(), samplePath)
	if err != nil {
		t.Fatalf("find sample: %v", err)
	}
	got, err := repo.ListTagsForSample(context.Background(), sample.ID)
	if err != nil {
		t.Fatalf("list sample tags: %v", err)
	}

	names := make(map[string]struct{}, len(got))
	for _, tag := range got {
		names[tag.NormalizedName] = struct{}{}
	}
	if _, ok := names["sub"]; !ok {
		t.Fatalf("missing rule tag %q in %v", "sub", names)
	}
}

func TestTagCommandAutoUsesCustomRuleFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "custom-rules.db")
	rulePath := filepath.Join(t.TempDir(), "rules.json")
	samplePath := filepath.Join(root, "Synth", "tone.wav")
	writeSineWAVFixture(t, samplePath, 220, 1.0)

	err := os.WriteFile(rulePath, []byte(`{
  "rules": [
    {
      "tag": "custom bass",
      "expr": "Metrics[\"frequency_hz\"] >= 200 && Metrics[\"frequency_hz\"] <= 240 && Metrics[\"confidence\"] > 0.5",
      "confidence": 0.88
    }
  ]
}`), 0o644)
	if err != nil {
		t.Fatalf("write rule file: %v", err)
	}

	scan := commands.NewRootCommand()
	scan.SetArgs([]string{"--db", dbPath, "scan", root})
	if err := scan.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute scan: %v", err)
	}

	tagCmd := commands.NewRootCommand()
	tagCmd.SetArgs([]string{"--db", dbPath, "tag", "--auto", "--rule-file", rulePath})
	if err := tagCmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute auto tag: %v", err)
	}

	repo := openRepoForTest(t, dbPath)
	sample, err := repo.FindSampleByPath(context.Background(), samplePath)
	if err != nil {
		t.Fatalf("find sample: %v", err)
	}
	got, err := repo.ListTagsForSample(context.Background(), sample.ID)
	if err != nil {
		t.Fatalf("list sample tags: %v", err)
	}

	names := make(map[string]struct{}, len(got))
	for _, tag := range got {
		names[tag.NormalizedName] = struct{}{}
	}
	if _, ok := names["custom bass"]; !ok {
		t.Fatalf("missing custom rule tag in %v", names)
	}
	if _, ok := names["sub"]; ok {
		t.Fatalf("expected custom rule file to replace defaults, got %v", names)
	}
}

func TestTagCommandAddAndRemoveManualTags(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "manual.db")
	samplePath := filepath.Join(root, "OneShots", "laser.wav")
	writeWAVFixture(t, samplePath)

	scan := commands.NewRootCommand()
	scan.SetArgs([]string{"--db", dbPath, "scan", root})
	if err := scan.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute scan: %v", err)
	}

	add := commands.NewRootCommand()
	add.SetArgs([]string{"--db", dbPath, "tag", "--add", "cyberpunk", "--path", samplePath})
	if err := add.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute add tag: %v", err)
	}

	repo := openRepoForTest(t, dbPath)
	sample, err := repo.FindSampleByPath(context.Background(), samplePath)
	if err != nil {
		t.Fatalf("find sample: %v", err)
	}
	tagsBefore, err := repo.ListTagsForSample(context.Background(), sample.ID)
	if err != nil {
		t.Fatalf("list tags before remove: %v", err)
	}
	if len(tagsBefore) != 1 || tagsBefore[0].NormalizedName != "cyberpunk" {
		t.Fatalf("unexpected tags before remove: %v", tagsBefore)
	}

	remove := commands.NewRootCommand()
	remove.SetArgs([]string{"--db", dbPath, "tag", "--remove", "cyberpunk", "--path", samplePath})
	if err := remove.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute remove tag: %v", err)
	}

	tagsAfter, err := repo.ListTagsForSample(context.Background(), sample.ID)
	if err != nil {
		t.Fatalf("list tags after remove: %v", err)
	}
	if len(tagsAfter) != 0 {
		t.Fatalf("expected no tags after remove, got %v", tagsAfter)
	}
}

func writeWAVFixture(t *testing.T, path string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}

	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create wav fixture: %v", err)
	}

	buffer := zaudio.PCMBuffer{
		SampleRate: 44100,
		Channels:   1,
		BitDepth:   16,
		Data:       make([]float64, 4410),
	}
	if err := wav.New().Encode(context.Background(), file, buffer); err != nil {
		t.Fatalf("encode wav fixture: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close wav fixture: %v", err)
	}
}

func writeSineWAVFixture(t *testing.T, path string, frequency, seconds float64) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}

	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create sine wav fixture: %v", err)
	}

	const sampleRate = 44100
	frames := int(float64(sampleRate) * seconds)
	data := make([]float64, frames)
	for i := range data {
		data[i] = 0.6 * math.Sin(2*math.Pi*frequency*float64(i)/sampleRate)
	}

	buffer := zaudio.PCMBuffer{
		SampleRate: sampleRate,
		Channels:   1,
		BitDepth:   16,
		Data:       data,
	}
	if err := wav.New().Encode(context.Background(), file, buffer); err != nil {
		t.Fatalf("encode sine wav fixture: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close sine wav fixture: %v", err)
	}
}

func openRepoForTest(t *testing.T, dbPath string) *db.Repository {
	t.Helper()

	database, err := db.Open(context.Background(), db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		_ = database.Close()
	})

	return db.NewRepository(database)
}
