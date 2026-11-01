package db_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/darkliquid/zounds/pkg/core"
	"github.com/darkliquid/zounds/pkg/db"
)

func testRepo(t *testing.T) *db.Repository {
	t.Helper()
	database, err := db.Open(context.Background(), db.Options{Path: filepath.Join(t.TempDir(), "zounds.db")})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return db.NewRepository(database)
}

func insertTestSample(t *testing.T, repo *db.Repository, path string) int64 {
	t.Helper()
	id, err := repo.UpsertSample(context.Background(), core.Sample{
		SourceRoot:   "/library",
		Path:         path,
		RelativePath: path[len("/library/"):],
		FileName:     path[len("/library/"):],
		Extension:    "wav",
		Format:       core.FormatWAV,
		SizeBytes:    1024,
	})
	if err != nil {
		t.Fatalf("upsert sample %s: %v", path, err)
	}
	return id
}

func TestPerSampleTagProvenanceIsIsolated(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := testRepo(t)
	sampleA := insertTestSample(t, repo, "/library/a.wav")
	sampleB := insertTestSample(t, repo, "/library/b.wav")

	tagID, err := repo.EnsureTag(ctx, core.Tag{Name: "instrument:kick", NormalizedName: "instrument:kick"})
	if err != nil {
		t.Fatalf("ensure tag: %v", err)
	}

	if err := repo.AttachTagToSample(ctx, sampleA, tagID, "rules", 0.9); err != nil {
		t.Fatalf("attach tag to sample A: %v", err)
	}
	if err := repo.AttachTagToSample(ctx, sampleB, tagID, "clap", 0.42); err != nil {
		t.Fatalf("attach tag to sample B: %v", err)
	}

	tagsA, err := repo.ListTagsForSample(ctx, sampleA)
	if err != nil {
		t.Fatalf("list tags for sample A: %v", err)
	}
	if len(tagsA) != 1 {
		t.Fatalf("expected 1 tag for sample A, got %d", len(tagsA))
	}
	if tagsA[0].Source != "rules" || tagsA[0].Confidence != 0.9 {
		t.Fatalf("sample A provenance overwritten: got source=%q confidence=%v", tagsA[0].Source, tagsA[0].Confidence)
	}

	tagsB, err := repo.ListTagsForSample(ctx, sampleB)
	if err != nil {
		t.Fatalf("list tags for sample B: %v", err)
	}
	if tagsB[0].Source != "clap" || tagsB[0].Confidence != 0.42 {
		t.Fatalf("sample B provenance wrong: got source=%q confidence=%v", tagsB[0].Source, tagsB[0].Confidence)
	}

	if err := repo.AttachTagToSample(ctx, sampleA, tagID, "manual", 1.0); err != nil {
		t.Fatalf("re-attach tag to sample A: %v", err)
	}
	tagsA, err = repo.ListTagsForSample(ctx, sampleA)
	if err != nil {
		t.Fatalf("list tags for sample A after re-attach: %v", err)
	}
	if tagsA[0].Source != "manual" || tagsA[0].Confidence != 1.0 {
		t.Fatalf("re-attach did not update sample A provenance: got source=%q confidence=%v", tagsA[0].Source, tagsA[0].Confidence)
	}
	tagsB, err = repo.ListTagsForSample(ctx, sampleB)
	if err != nil {
		t.Fatalf("list tags for sample B after sample A re-attach: %v", err)
	}
	if tagsB[0].Source != "clap" || tagsB[0].Confidence != 0.42 {
		t.Fatalf("re-attaching to sample A clobbered sample B provenance: got source=%q confidence=%v", tagsB[0].Source, tagsB[0].Confidence)
	}
}

func TestSampleAnalysisUpsertRoundTrip(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := testRepo(t)
	sampleID := insertTestSample(t, repo, "/library/snare.wav")

	duration := 0.83
	sampleRate := int64(44100)
	channels := int64(2)
	bpm := 128.0
	bpmConfidence := 0.97
	keyTonic := "C"
	keyMode := "minor"
	loopClass := "loop"
	lufs := -9.5
	truePeak := -1.2

	analysis := core.SampleAnalysis{
		SampleID:        sampleID,
		DurationSeconds: &duration,
		SampleRate:      &sampleRate,
		Channels:        &channels,
		BPM:             &bpm,
		BPMConfidence:   &bpmConfidence,
		KeyTonic:        &keyTonic,
		KeyMode:         &keyMode,
		LoopClass:       &loopClass,
		LUFSIntegrated:  &lufs,
		TruePeakDBTP:    &truePeak,
		Metrics:         map[string]float64{"spectral_centroid_hz": 1840.5},
		Attributes:      map[string]string{"note_name": "F#2"},
	}
	if err := repo.UpsertSampleAnalysis(ctx, analysis); err != nil {
		t.Fatalf("upsert analysis: %v", err)
	}

	got, err := repo.GetSampleAnalysis(ctx, sampleID)
	if err != nil {
		t.Fatalf("get analysis: %v", err)
	}
	if got.DurationSeconds == nil || *got.DurationSeconds != duration {
		t.Fatalf("duration mismatch: %+v", got.DurationSeconds)
	}
	if got.BPM == nil || *got.BPM != bpm {
		t.Fatalf("bpm mismatch: %+v", got.BPM)
	}
	if got.KeyTonic == nil || *got.KeyTonic != "C" {
		t.Fatalf("key tonic mismatch: %+v", got.KeyTonic)
	}
	if got.Metrics["spectral_centroid_hz"] != 1840.5 {
		t.Fatalf("metrics mismatch: %v", got.Metrics)
	}
	if got.Attributes["note_name"] != "F#2" {
		t.Fatalf("attributes mismatch: %v", got.Attributes)
	}
	if got.BitDepth != nil || got.Chord != nil || got.PitchHz != nil {
		t.Fatalf("unset nullable fields should be nil, got bitDepth=%+v chord=%+v pitchHz=%+v", got.BitDepth, got.Chord, got.PitchHz)
	}

	updated := core.SampleAnalysis{SampleID: sampleID, Attributes: map[string]string{"note_name": "G2"}}
	if err := repo.UpsertSampleAnalysis(ctx, updated); err != nil {
		t.Fatalf("re-upsert analysis: %v", err)
	}
	got, err = repo.GetSampleAnalysis(ctx, sampleID)
	if err != nil {
		t.Fatalf("get analysis after re-upsert: %v", err)
	}
	if got.BPM != nil {
		t.Fatalf("expected bpm cleared by full upsert, got %+v", got.BPM)
	}
	if got.Attributes["note_name"] != "G2" {
		t.Fatalf("attributes not updated: %v", got.Attributes)
	}

	if err := repo.DeleteSampleAnalysis(ctx, sampleID); err != nil {
		t.Fatalf("delete analysis: %v", err)
	}
	if _, err := repo.GetSampleAnalysis(ctx, sampleID); err != sql.ErrNoRows {
		t.Fatalf("expected sql.ErrNoRows after delete, got %v", err)
	}
}

func TestListSampleAnalysesBySampleIDs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := testRepo(t)

	s1 := insertTestSample(t, repo, "/library/s1.wav")
	s2 := insertTestSample(t, repo, "/library/s2.wav")
	s3 := insertTestSample(t, repo, "/library/s3.wav")

	bpm1 := 120.0
	bpm2 := 140.0
	if err := repo.UpsertSampleAnalysis(ctx, core.SampleAnalysis{SampleID: s1, BPM: &bpm1}); err != nil {
		t.Fatalf("upsert s1: %v", err)
	}
	if err := repo.UpsertSampleAnalysis(ctx, core.SampleAnalysis{SampleID: s2, BPM: &bpm2}); err != nil {
		t.Fatalf("upsert s2: %v", err)
	}

	// Empty sampleIDs returns empty map
	emptyMap, err := repo.ListSampleAnalysesBySampleIDs(ctx, nil)
	if err != nil {
		t.Fatalf("ListSampleAnalysesBySampleIDs nil: %v", err)
	}
	if len(emptyMap) != 0 {
		t.Fatalf("expected empty map, got %d items", len(emptyMap))
	}

	// Query s1, s2, s3 (s3 has no analysis)
	res, err := repo.ListSampleAnalysesBySampleIDs(ctx, []int64{s1, s2, s3})
	if err != nil {
		t.Fatalf("ListSampleAnalysesBySampleIDs: %v", err)
	}
	if len(res) != 2 {
		t.Fatalf("expected 2 analyses, got %d", len(res))
	}
	if res[s1].BPM == nil || *res[s1].BPM != 120.0 {
		t.Fatalf("expected s1 BPM 120, got %#v", res[s1].BPM)
	}
	if res[s2].BPM == nil || *res[s2].BPM != 140.0 {
		t.Fatalf("expected s2 BPM 140, got %#v", res[s2].BPM)
	}
	if _, ok := res[s3]; ok {
		t.Fatalf("expected s3 not in result map")
	}
}

func TestSampleProjectionUpsertAndList(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := testRepo(t)
	sampleID := insertTestSample(t, repo, "/library/pad.wav")

	if err := repo.UpsertSampleProjection(ctx, core.SampleProjection{
		SampleID:  sampleID,
		Method:    "tsne",
		Namespace: "acoustic",
		X:         0.0,
		Y:         -3.25,
	}); err != nil {
		t.Fatalf("upsert projection: %v", err)
	}
	if err := repo.UpsertSampleProjection(ctx, core.SampleProjection{
		SampleID:  sampleID,
		Method:    "pca",
		Namespace: "acoustic",
		X:         1.5,
		Y:         2.5,
	}); err != nil {
		t.Fatalf("upsert second projection: %v", err)
	}

	projection, err := repo.GetSampleProjection(ctx, sampleID, "tsne", "acoustic")
	if err != nil {
		t.Fatalf("get projection: %v", err)
	}
	if projection.X != 0.0 || projection.Y != -3.25 {
		t.Fatalf("projection coordinates wrong (zero x must round-trip): x=%v y=%v", projection.X, projection.Y)
	}

	if err := repo.UpsertSampleProjection(ctx, core.SampleProjection{
		SampleID:  sampleID,
		Method:    "tsne",
		Namespace: "acoustic",
		X:         4.0,
		Y:         5.0,
	}); err != nil {
		t.Fatalf("update projection: %v", err)
	}
	projection, err = repo.GetSampleProjection(ctx, sampleID, "tsne", "acoustic")
	if err != nil {
		t.Fatalf("get updated projection: %v", err)
	}
	if projection.X != 4.0 || projection.Y != 5.0 {
		t.Fatalf("projection not updated: x=%v y=%v", projection.X, projection.Y)
	}

	all, err := repo.ListSampleProjections(ctx, "", "acoustic")
	if err != nil {
		t.Fatalf("list projections: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 projections, got %d", len(all))
	}

	filtered, err := repo.ListSampleProjections(ctx, "pca", "acoustic")
	if err != nil {
		t.Fatalf("list filtered projections: %v", err)
	}
	if len(filtered) != 1 || filtered[0].Method != "pca" {
		t.Fatalf("unexpected filtered projections: %+v", filtered)
	}
}

func TestPerceptualHashCRUD(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := testRepo(t)
	sampleID := insertTestSample(t, repo, "/library/loop.wav")

	if err := repo.UpsertPerceptualHash(ctx, core.PerceptualHash{
		SampleID:  sampleID,
		Algorithm: "spectral-8x8",
		Hash:      "9f3a12",
		HashBits:  64,
	}); err != nil {
		t.Fatalf("upsert hash: %v", err)
	}

	hash, err := repo.GetPerceptualHash(ctx, sampleID, "spectral-8x8")
	if err != nil {
		t.Fatalf("get hash: %v", err)
	}
	if hash.Hash != "9f3a12" || hash.HashBits != 64 {
		t.Fatalf("hash mismatch: %+v", hash)
	}

	if err := repo.UpsertPerceptualHash(ctx, core.PerceptualHash{
		SampleID:  sampleID,
		Algorithm: "spectral-8x8",
		Hash:      "001122",
		HashBits:  64,
	}); err != nil {
		t.Fatalf("re-upsert hash: %v", err)
	}
	hash, err = repo.GetPerceptualHash(ctx, sampleID, "spectral-8x8")
	if err != nil {
		t.Fatalf("get updated hash: %v", err)
	}
	if hash.Hash != "001122" {
		t.Fatalf("hash not updated: %+v", hash)
	}

	hashes, err := repo.ListPerceptualHashes(ctx, "spectral-8x8")
	if err != nil {
		t.Fatalf("list hashes: %v", err)
	}
	if len(hashes) != 1 {
		t.Fatalf("expected 1 hash, got %d", len(hashes))
	}

	if _, err := repo.GetPerceptualHash(ctx, sampleID, "missing-algo"); err != sql.ErrNoRows {
		t.Fatalf("expected sql.ErrNoRows for unknown algorithm, got %v", err)
	}
}

func TestMigrateUpgradesLegacyProvenanceDB(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "zounds.db")

	database, err := db.Open(ctx, db.Options{Path: path})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}

	// Simulate a database written before 0002: recreate tags with the old
	// provenance columns and sample_tags without them, then reset
	// schema_migrations so the migration replays.
	if _, err := database.ExecContext(ctx, `
		PRAGMA foreign_keys = OFF;
		CREATE TABLE sample_tags_old (
			sample_id INTEGER NOT NULL,
			tag_id INTEGER NOT NULL,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (sample_id, tag_id)
		);
		INSERT INTO sample_tags_old (sample_id, tag_id)
			SELECT sample_id, tag_id FROM sample_tags;
		DROP TABLE sample_tags;
		ALTER TABLE sample_tags_old RENAME TO sample_tags;

		CREATE TABLE tags_old (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			normalized_name TEXT NOT NULL UNIQUE,
			source TEXT NOT NULL DEFAULT 'unknown',
			confidence REAL NOT NULL DEFAULT 1.0,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		INSERT INTO tags_old (name, normalized_name, source, confidence)
			SELECT name, normalized_name, 'rules', 0.7 FROM tags;
		DROP TABLE tags;
		ALTER TABLE tags_old RENAME TO tags;

		INSERT INTO tags (name, normalized_name, source, confidence)
			VALUES ('legacy', 'legacy', 'legacysource', 0.33);
		INSERT INTO sample_tags (sample_id, tag_id)
			VALUES (1, (SELECT id FROM tags WHERE normalized_name = 'legacy'));
		DELETE FROM schema_migrations WHERE version = '0002_rich_analysis_and_provenance';
		PRAGMA foreign_keys = ON;
	`); err != nil {
		t.Fatalf("rebuild legacy schema: %v", err)
	}
	_ = database.Close()

	reopened, err := db.Open(ctx, db.Options{Path: path})
	if err != nil {
		t.Fatalf("reopen database: %v", err)
	}
	defer func() { _ = reopened.Close() }()

	var source string
	var confidence float64
	err = reopened.QueryRowContext(ctx, `
		SELECT st.source, st.confidence
		FROM sample_tags st
		JOIN tags t ON t.id = st.tag_id
		WHERE t.normalized_name = 'legacy';
	`).Scan(&source, &confidence)
	if err != nil {
		t.Fatalf("read backfilled provenance: %v", err)
	}
	if source != "legacysource" || confidence != 0.33 {
		t.Fatalf("legacy provenance not backfilled: got source=%q confidence=%v", source, confidence)
	}

	var tagsHasSource int
	err = reopened.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM pragma_table_info('tags') WHERE name IN ('source', 'confidence');
	`).Scan(&tagsHasSource)
	if err != nil {
		t.Fatalf("inspect tags columns: %v", err)
	}
	if tagsHasSource != 0 {
		t.Fatalf("tags table still has %d provenance columns", tagsHasSource)
	}
}

func TestFeatureVectorNamespaceUniqueness(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := testRepo(t)
	sampleID := insertTestSample(t, repo, "/library/hat.wav")

	vector := core.FeatureVector{SampleID: sampleID, Namespace: "analysis", Version: "1", Values: []float64{1, 2, 3}}
	if _, err := repo.InsertFeatureVector(ctx, vector); err != nil {
		t.Fatalf("insert feature vector: %v", err)
	}
	if _, err := repo.InsertFeatureVector(ctx, vector); err == nil {
		t.Fatal("expected unique constraint error inserting duplicate (sample_id, namespace)")
	}

	if _, err := repo.ReplaceFeatureVector(ctx, vector); err != nil {
		t.Fatalf("replace feature vector: %v", err)
	}
	vectors, err := repo.ListFeatureVectorsForSample(ctx, sampleID)
	if err != nil {
		t.Fatalf("list feature vectors: %v", err)
	}
	if len(vectors) != 1 {
		t.Fatalf("expected exactly 1 vector after replace, got %d", len(vectors))
	}

	found, err := repo.FindFeatureVector(ctx, sampleID, "analysis")
	if err != nil {
		t.Fatalf("FindFeatureVector: %v", err)
	}
	if found.Namespace != "analysis" || len(found.Values) != 3 {
		t.Fatalf("unexpected found vector: %+v", found)
	}

	if _, err := repo.FindFeatureVector(ctx, sampleID, "nonexistent"); err == nil {
		t.Fatal("expected error finding nonexistent namespace")
	}
}
