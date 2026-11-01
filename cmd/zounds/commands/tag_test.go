package commands_test

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/zounds/cmd/zounds/commands"
	zaudio "github.com/darkliquid/zounds/pkg/audio"
	"github.com/darkliquid/zounds/pkg/audio/wav"
	"github.com/darkliquid/zounds/pkg/db"
)

func TestTagCommandAutoSkipsGenericPathTags(t *testing.T) {
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
	for _, unwanted := range []string{"drums", "impact"} {
		if _, ok := names[unwanted]; ok {
			t.Fatalf("unexpected generic path tag %q in %v", unwanted, names)
		}
	}
	if len(names) != 0 {
		t.Fatalf("expected no generic path-derived tags, got %v", names)
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
	rulePath := filepath.Join(t.TempDir(), "sub-rules.json")
	samplePath := filepath.Join(root, "Synth", "tone.wav")
	writeSineWAVFixture(t, samplePath, 55, 1.0)

	err := os.WriteFile(rulePath, []byte(`{
  "rules": [
    {
      "tag": "sub",
      "expr": "Metrics[\"frequency_hz\"] >= 20 && Metrics[\"frequency_hz\"] < 80 && Metrics[\"confidence\"] > 0.5",
      "confidence": 0.9
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
		t.Fatalf("unexpected unconfigured rule tag in %v", names)
	}
}

func TestTagCommandAutoSupportsPathBasedCustomRules(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "path-rules.db")
	rulePath := filepath.Join(t.TempDir(), "path-rules.json")
	samplePath := filepath.Join(root, "Black+Octopus+Sound", "Didgeridoo Loops", "loop.wav")
	writeWAVFixture(t, samplePath)

	err := os.WriteFile(rulePath, []byte(`{
  "rules": [
    {
      "tag": "didgeridoo",
      "expr": "Sample.RelativePath contains \"Didgeridoo Loops\""
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
	if _, ok := names["didgeridoo"]; !ok {
		t.Fatalf("missing path-based custom rule tag in %v", names)
	}
	if _, ok := names["black"]; ok {
		t.Fatalf("unexpected generic path tag in %v", names)
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

func TestTagCommandAutoSkipsUndecodableSamplesInBatch(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "tag-skip.db")
	rulePath := filepath.Join(t.TempDir(), "rules.json")
	validWav := filepath.Join(root, "tone.wav")
	unsupportedAAC := filepath.Join(root, "voice.aac")

	writeSineWAVFixture(t, validWav, 55, 1.0)
	if err := os.WriteFile(unsupportedAAC, []byte("fake-aac"), 0o644); err != nil {
		t.Fatalf("write aac: %v", err)
	}
	if err := os.WriteFile(rulePath, []byte(`{"rules":[{"tag":"instrument:sub","expr":"Metrics[\"frequency_hz\"] < 80"}]}`), 0o644); err != nil {
		t.Fatalf("write rules: %v", err)
	}

	scan := commands.NewRootCommand()
	scan.SetArgs([]string{"--db", dbPath, "scan", root})
	if err := scan.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute scan: %v", err)
	}

	tagCmd := commands.NewRootCommand()
	var stdout, stderr bytes.Buffer
	tagCmd.SetOut(&stdout)
	tagCmd.SetErr(&stderr)
	tagCmd.SetArgs([]string{"--db", dbPath, "tag", "--auto", "--rule-file", rulePath})
	if err := tagCmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("expected tag --auto batch to skip undecodable .aac, got: %v", err)
	}

	if !strings.Contains(stdout.String(), "processed 1 samples and applied 1 tags") {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), unsupportedAAC) {
		t.Fatalf("expected warning for %q in stderr %q", unsupportedAAC, stderr.String())
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

func writeSineWAVFixtureWithRate(t *testing.T, path string, frequency, seconds float64, sampleRate int) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}

	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create sine wav fixture: %v", err)
	}

	frames := int(float64(sampleRate) * seconds)
	data := make([]float64, frames)
	for i := range data {
		data[i] = 0.6 * math.Sin(2*math.Pi*frequency*float64(i)/float64(sampleRate))
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

func TestTagCommandKNN(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "tag-knn.db")
	sample1Path := filepath.Join(root, "kick1.wav")
	sample2Path := filepath.Join(root, "kick2.wav")
	writeSineWAVFixture(t, sample1Path, 60, 0.5)
	writeSineWAVFixture(t, sample2Path, 60, 0.5)

	// Scan
	scan := commands.NewRootCommand()
	scan.SetArgs([]string{"--db", dbPath, "scan", root})
	if err := scan.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("scan: %v", err)
	}

	// Analyze
	analyze := commands.NewRootCommand()
	analyze.SetArgs([]string{"--db", dbPath, "analyze", "--all"})
	if err := analyze.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("analyze: %v", err)
	}

	// Manually tag sample 1
	tagCmd := commands.NewRootCommand()
	tagCmd.SetArgs([]string{"--db", dbPath, "tag", "--path", sample1Path, "--add", "instrument:kick"})
	if err := tagCmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("manual tag: %v", err)
	}

	// Auto-tag with --knn on sample 2
	autoTag := commands.NewRootCommand()
	var out bytes.Buffer
	autoTag.SetOut(&out)
	autoTag.SetErr(&out)
	autoTag.SetArgs([]string{"--db", dbPath, "tag", "--auto", "--path", sample2Path, "--knn", "--knn-min-score", "0.1"})
	if err := autoTag.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("auto tag with knn: %v", err)
	}

	repo := openRepoForTest(t, dbPath)
	s2, err := repo.FindSampleByPath(context.Background(), sample2Path)
	if err != nil {
		t.Fatalf("find sample 2: %v", err)
	}
	tags, err := repo.ListTagsForSample(context.Background(), s2.ID)
	if err != nil {
		t.Fatalf("list tags: %v", err)
	}
	found := false
	for _, tag := range tags {
		if tag.NormalizedName == "instrument:kick" && tag.Source == "local" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected propagated tag 'instrument:kick' from knn on sample 2, got %+v", tags)
	}
}

func TestTagCommandLLM(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]string{
						"content": "instrument:synth, character:ambient",
					},
				},
			},
		})
	}))
	defer ts.Close()

	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "tag-llm.db")
	samplePath := filepath.Join(root, "pad.wav")
	writeSineWAVFixture(t, samplePath, 440, 0.5)

	scan := commands.NewRootCommand()
	scan.SetArgs([]string{"--db", dbPath, "scan", root})
	if err := scan.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("scan: %v", err)
	}

	autoTag := commands.NewRootCommand()
	var out bytes.Buffer
	autoTag.SetOut(&out)
	autoTag.SetErr(&out)
	autoTag.SetArgs([]string{
		"--db", dbPath,
		"tag", "--auto",
		"--path", samplePath,
		"--llm-endpoint", ts.URL,
		"--llm-model", "test-model",
	})
	if err := autoTag.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("auto tag with llm: %v", err)
	}

	repo := openRepoForTest(t, dbPath)
	s, err := repo.FindSampleByPath(context.Background(), samplePath)
	if err != nil {
		t.Fatalf("find sample: %v", err)
	}
	tags, err := repo.ListTagsForSample(context.Background(), s.ID)
	if err != nil {
		t.Fatalf("list tags: %v", err)
	}
	tagMap := make(map[string]string)
	for _, tag := range tags {
		tagMap[tag.NormalizedName] = tag.Source
	}

	for _, want := range []string{"instrument:synth", "character:ambient"} {
		if source, ok := tagMap[want]; !ok || source != "llm" {
			t.Errorf("missing or non-LLM tag %q in %v", want, tagMap)
		}
	}
}
