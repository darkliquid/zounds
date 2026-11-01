package web_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	zaudio "github.com/darkliquid/zounds/pkg/audio"
	zaiff "github.com/darkliquid/zounds/pkg/audio/aiff"
	"github.com/darkliquid/zounds/pkg/core"
	"github.com/darkliquid/zounds/pkg/db"
	"github.com/darkliquid/zounds/web"
)

func TestServerListsSamplesAndTags(t *testing.T) {
	t.Parallel()

	repo := testRepository(t)
	sampleID := insertSample(t, repo, "impact.wav")
	tagID, err := repo.EnsureTag(context.Background(), core.Tag{
		Name:           "dark",
		NormalizedName: "dark",
		Source:         "rules",
		Confidence:     0.7,
	})
	if err != nil {
		t.Fatalf("EnsureTag returned error: %v", err)
	}
	if err := repo.AttachTagToSample(context.Background(), sampleID, tagID, "rules", 0.7); err != nil {
		t.Fatalf("AttachTagToSample returned error: %v", err)
	}

	server, err := web.NewServer(repo)
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/samples", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var payload []struct {
		ID   int64      `json:"id"`
		Tags []core.Tag `json:"tags"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload) != 1 {
		t.Fatalf("expected 1 sample, got %d", len(payload))
	}
	if len(payload[0].Tags) != 1 || payload[0].Tags[0].NormalizedName != "dark" {
		t.Fatalf("unexpected tags %#v", payload[0].Tags)
	}
}

func TestServerFiltersSamplesByQuery(t *testing.T) {
	t.Parallel()

	repo := testRepository(t)
	insertSample(t, repo, "impact.wav")
	insertSample(t, repo, "pad.wav")

	server, err := web.NewServer(repo)
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/samples?q=impact", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var payload []struct {
		FileName string `json:"file_name"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload) != 1 {
		t.Fatalf("expected 1 matching sample, got %d", len(payload))
	}
	if payload[0].FileName != "impact.wav" {
		t.Fatalf("expected impact.wav, got %q", payload[0].FileName)
	}
}

func TestServerFiltersSamplesByMusicalFacets(t *testing.T) {
	t.Parallel()

	repo := testRepository(t)
	ctx := context.Background()

	id1 := insertSample(t, repo, "kick.wav")
	id2 := insertSample(t, repo, "drumloop.wav")

	bpm1 := 128.0
	bpm2 := 174.0
	dur1 := 0.5
	dur2 := 4.0
	tonic1 := "C"
	tonic2 := "F#"
	mode1 := "major"
	mode2 := "minor"
	chord1 := "Cmaj7"
	chord2 := "F#m7"
	loop1 := "oneshot"
	loop2 := "loop"

	if err := repo.UpsertSampleAnalysis(ctx, core.SampleAnalysis{
		SampleID:        id1,
		BPM:             &bpm1,
		DurationSeconds: &dur1,
		KeyTonic:        &tonic1,
		KeyMode:         &mode1,
		Chord:           &chord1,
		LoopClass:       &loop1,
	}); err != nil {
		t.Fatalf("UpsertSampleAnalysis 1: %v", err)
	}

	if err := repo.UpsertSampleAnalysis(ctx, core.SampleAnalysis{
		SampleID:        id2,
		BPM:             &bpm2,
		DurationSeconds: &dur2,
		KeyTonic:        &tonic2,
		KeyMode:         &mode2,
		Chord:           &chord2,
		LoopClass:       &loop2,
	}); err != nil {
		t.Fatalf("UpsertSampleAnalysis 2: %v", err)
	}

	server, err := web.NewServer(repo)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	type sampleResp struct {
		ID       int64                `json:"id"`
		FileName string               `json:"file_name"`
		Analysis *core.SampleAnalysis `json:"analysis"`
	}

	fetch := func(query string) []sampleResp {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/samples?"+query, nil)
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for query %q, got %d", query, rec.Code)
		}
		var out []sampleResp
		if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return out
	}

	// 1. Filter by type=loop
	loops := fetch("type=loop")
	if len(loops) != 1 || loops[0].FileName != "drumloop.wav" {
		t.Fatalf("expected [drumloop.wav] for type=loop, got %#v", loops)
	}
	if loops[0].Analysis == nil || loops[0].Analysis.LoopClass == nil || *loops[0].Analysis.LoopClass != "loop" {
		t.Fatalf("expected analysis to be populated with loop_class=loop, got %#v", loops[0].Analysis)
	}

	// 2. Filter by BPM range [120, 130]
	tempoMatched := fetch("bpm_min=120&bpm_max=130")
	if len(tempoMatched) != 1 || tempoMatched[0].FileName != "kick.wav" {
		t.Fatalf("expected [kick.wav] for bpm range, got %#v", tempoMatched)
	}

	// 3. Filter by key
	keyMatched := fetch("key=c+major")
	if len(keyMatched) != 1 || keyMatched[0].FileName != "kick.wav" {
		t.Fatalf("expected [kick.wav] for key=c+major, got %#v", keyMatched)
	}

	// 4. Filter by chord
	chordMatched := fetch("chord=f%23m")
	if len(chordMatched) != 1 || chordMatched[0].FileName != "drumloop.wav" {
		t.Fatalf("expected [drumloop.wav] for chord=f#m, got %#v", chordMatched)
	}

	// 5. Filter by duration
	durMatched := fetch("duration_min=1.0")
	if len(durMatched) != 1 || durMatched[0].FileName != "drumloop.wav" {
		t.Fatalf("expected [drumloop.wav] for duration_min=1.0, got %#v", durMatched)
	}
}

func TestServerStreamsSampleAudio(t *testing.T) {
	t.Parallel()

	repo := testRepository(t)
	tempDir := t.TempDir()
	audioPath := filepath.Join(tempDir, "tone.wav")
	if err := os.WriteFile(audioPath, []byte("RIFFtest"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	sampleID, err := repo.UpsertSample(context.Background(), core.Sample{
		SourceRoot:   tempDir,
		Path:         audioPath,
		RelativePath: "tone.wav",
		FileName:     "tone.wav",
		Extension:    "wav",
		Format:       core.FormatWAV,
		SizeBytes:    8,
		ModifiedAt:   time.Now(),
		ScannedAt:    time.Now(),
	})
	if err != nil {
		t.Fatalf("UpsertSample returned error: %v", err)
	}

	server, err := web.NewServer(repo)
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/samples/"+itoa(sampleID)+"/audio", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "RIFFtest" {
		t.Fatalf("unexpected body %q", rec.Body.String())
	}
}

func TestServerTranscodesAIFFPreviewToWAV(t *testing.T) {
	t.Parallel()

	repo := testRepository(t)
	tempDir := t.TempDir()
	audioPath := filepath.Join(tempDir, "bell.aiff")

	file, err := os.Create(audioPath)
	if err != nil {
		t.Fatalf("Create aiff: %v", err)
	}

	codec := zaiff.New()
	pcm := zaudio.PCMBuffer{
		SampleRate: 44100,
		Channels:   1,
		BitDepth:   16,
		Data:       []float64{0.0, 0.5, 0.0, -0.5, 0.0},
	}
	if err := codec.Encode(context.Background(), file, pcm); err != nil {
		t.Fatalf("encode aiff: %v", err)
	}
	_ = file.Close()

	sampleID, err := repo.UpsertSample(context.Background(), core.Sample{
		SourceRoot:   tempDir,
		Path:         audioPath,
		RelativePath: "bell.aiff",
		FileName:     "bell.aiff",
		Extension:    "aiff",
		Format:       core.FormatAIFF,
		SizeBytes:    1024,
		ModifiedAt:   time.Now(),
		ScannedAt:    time.Now(),
	})
	if err != nil {
		t.Fatalf("UpsertSample: %v", err)
	}

	server, err := web.NewServer(repo)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	// 1. Initial request: triggers on-the-fly transcoding
	req := httptest.NewRequest(http.MethodGet, "/api/samples/"+itoa(sampleID)+"/audio", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "audio/wav" {
		t.Fatalf("expected audio/wav content-type, got %q", ct)
	}
	body := rec.Body.Bytes()
	if !bytes.HasPrefix(body, []byte("RIFF")) {
		t.Fatalf("expected RIFF header, got %q", body[:min(len(body), 4)])
	}
	if !bytes.Contains(body, []byte("WAVE")) {
		t.Fatalf("expected WAVE header in body")
	}

	// 2. Second request: hits cached transcoded WAV
	req2 := httptest.NewRequest(http.MethodGet, "/api/samples/"+itoa(sampleID)+"/audio", nil)
	rec2 := httptest.NewRecorder()
	server.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200 on cache hit, got %d", rec2.Code)
	}
	if !bytes.Equal(rec2.Body.Bytes(), body) {
		t.Fatalf("expected cached bytes to match initial transcode")
	}
}

func TestServerFindSimilarSamples(t *testing.T) {
	t.Parallel()

	repo := testRepository(t)
	ctx := context.Background()

	refID := insertSample(t, repo, "kick-ref.wav")
	nearID := insertSample(t, repo, "kick-near.wav")
	otherID := insertSample(t, repo, "snare.wav")
	farID := insertSample(t, repo, "synth-far.wav")

	tagID, err := repo.EnsureTag(ctx, core.Tag{Name: "punchy", NormalizedName: "punchy"})
	if err != nil {
		t.Fatalf("EnsureTag: %v", err)
	}
	if err := repo.AttachTagToSample(ctx, nearID, tagID, "rules", 0.95); err != nil {
		t.Fatalf("AttachTagToSample: %v", err)
	}

	// Insert feature vectors for acoustic similarity
	if _, err := repo.InsertFeatureVector(ctx, core.FeatureVector{
		SampleID:   refID,
		Namespace:  "acoustic",
		Version:    "1",
		Dimensions: 3,
		Values:     []float64{10.0, 100.0, 50.0},
	}); err != nil {
		t.Fatalf("insert ref vector: %v", err)
	}
	if _, err := repo.InsertFeatureVector(ctx, core.FeatureVector{
		SampleID:   nearID,
		Namespace:  "acoustic",
		Version:    "1",
		Dimensions: 3,
		Values:     []float64{12.0, 95.0, 52.0},
	}); err != nil {
		t.Fatalf("insert near vector: %v", err)
	}
	if _, err := repo.InsertFeatureVector(ctx, core.FeatureVector{
		SampleID:   otherID,
		Namespace:  "acoustic",
		Version:    "1",
		Dimensions: 3,
		Values:     []float64{50.0, 10.0, 80.0},
	}); err != nil {
		t.Fatalf("insert other vector: %v", err)
	}
	if _, err := repo.InsertFeatureVector(ctx, core.FeatureVector{
		SampleID:   farID,
		Namespace:  "acoustic",
		Version:    "1",
		Dimensions: 3,
		Values:     []float64{100.0, 20.0, 10.0},
	}); err != nil {
		t.Fatalf("insert far vector: %v", err)
	}

	server, err := web.NewServer(repo)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	// 1. Valid similar query
	req := httptest.NewRequest(http.MethodGet, "/api/samples/"+itoa(refID)+"/similar?metric=acoustic&threshold=-1", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var payload []struct {
		ID         int64      `json:"id"`
		FileName   string     `json:"file_name"`
		Tags       []core.Tag `json:"tags"`
		Similarity float64    `json:"similarity"`
		Distance   float64    `json:"distance"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode similar response: %v", err)
	}

	if len(payload) != 3 {
		t.Fatalf("expected 3 similar samples (excluding ref), got %d", len(payload))
	}
	if payload[0].FileName != "kick-near.wav" {
		t.Fatalf("expected kick-near.wav as top match, got %q", payload[0].FileName)
	}
	if payload[0].Similarity < 0.95 {
		t.Fatalf("expected high similarity for kick-near.wav, got %v", payload[0].Similarity)
	}
	if len(payload[0].Tags) != 1 || payload[0].Tags[0].NormalizedName != "punchy" {
		t.Fatalf("expected tags attached to similar match: %#v", payload[0].Tags)
	}
	if payload[1].FileName != "synth-far.wav" {
		t.Fatalf("expected synth-far.wav as second match, got %q", payload[1].FileName)
	}

	// 2. Non-existent sample ID returns 404
	req404 := httptest.NewRequest(http.MethodGet, "/api/samples/99999/similar", nil)
	rec404 := httptest.NewRecorder()
	server.ServeHTTP(rec404, req404)

	if rec404.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown sample id, got %d", rec404.Code)
	}
}

func TestServerReturnsClusterByID(t *testing.T) {
	t.Parallel()

	repo := testRepository(t)
	sampleID := insertSample(t, repo, "tone.wav")
	clusterID, err := repo.InsertCluster(context.Background(), core.Cluster{
		Method:     "kmeans",
		Label:      "Cluster 1",
		Parameters: map[string]float64{"k": 1},
	})
	if err != nil {
		t.Fatalf("InsertCluster returned error: %v", err)
	}
	if err := repo.InsertClusterMember(context.Background(), db.ClusterMember{
		ClusterID: clusterID,
		SampleID:  sampleID,
		Score:     1,
	}); err != nil {
		t.Fatalf("InsertClusterMember returned error: %v", err)
	}

	server, err := web.NewServer(repo)
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/clusters/"+itoa(clusterID), nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var payload struct {
		ID      int64
		Samples []int64
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.ID == 0 || len(payload.Samples) != 1 || payload.Samples[0] != sampleID {
		t.Fatalf("unexpected cluster payload %#v", payload)
	}
}

func TestServerProjectsClustersForVisualization(t *testing.T) {
	t.Parallel()

	repo := testRepository(t)
	sample1 := insertSample(t, repo, "tone-1.wav")
	sample2 := insertSample(t, repo, "tone-2.wav")
	_, err := repo.ReplaceFeatureVector(context.Background(), core.FeatureVector{
		SampleID:   sample1,
		Namespace:  "analysis",
		Version:    "test",
		Values:     []float64{0, 0, 0},
		Dimensions: 3,
		CreatedAt:  time.Now(),
	})
	if err != nil {
		t.Fatalf("ReplaceFeatureVector sample1 returned error: %v", err)
	}
	_, err = repo.ReplaceFeatureVector(context.Background(), core.FeatureVector{
		SampleID:   sample2,
		Namespace:  "analysis",
		Version:    "test",
		Values:     []float64{5, 5, 0},
		Dimensions: 3,
		CreatedAt:  time.Now(),
	})
	if err != nil {
		t.Fatalf("ReplaceFeatureVector sample2 returned error: %v", err)
	}

	clusterID, err := repo.InsertCluster(context.Background(), core.Cluster{
		Method:     "kmeans",
		Label:      "Cluster 1",
		Parameters: map[string]float64{"k": 1},
	})
	if err != nil {
		t.Fatalf("InsertCluster returned error: %v", err)
	}
	for _, sampleID := range []int64{sample1, sample2} {
		if err := repo.InsertClusterMember(context.Background(), db.ClusterMember{
			ClusterID: clusterID,
			SampleID:  sampleID,
			Score:     1,
		}); err != nil {
			t.Fatalf("InsertClusterMember returned error: %v", err)
		}
	}

	server, err := web.NewServer(repo)
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/clusters?projection=tsne", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var payload []struct {
		ID      int64
		X       float64
		Y       float64
		Members []struct {
			ID int64
			X  float64
			Y  float64
		}
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload) != 1 || payload[0].ID == 0 {
		t.Fatalf("unexpected cluster payload %#v", payload)
	}
	if len(payload[0].Members) != 2 {
		t.Fatalf("expected projected cluster members, got %#v", payload[0].Members)
	}
}

func TestServerServesPersistedProjectionsWithoutRecomputing(t *testing.T) {
	t.Parallel()

	repo := testRepository(t)
	sampleID := insertSample(t, repo, "tone.wav")

	clusterID, err := repo.InsertCluster(context.Background(), core.Cluster{
		Method: "kmeans",
		Label:  "Cluster 1",
	})
	if err != nil {
		t.Fatalf("InsertCluster: %v", err)
	}
	if err := repo.InsertClusterMember(context.Background(), db.ClusterMember{
		ClusterID: clusterID,
		SampleID:  sampleID,
		Score:     1.0,
	}); err != nil {
		t.Fatalf("InsertClusterMember: %v", err)
	}

	// Persist known projection coordinate directly to sample_projections
	if err := repo.UpsertSampleProjection(context.Background(), core.SampleProjection{
		SampleID:  sampleID,
		Method:    "tsne",
		Namespace: "analysis",
		X:         42.5,
		Y:         -84.5,
	}); err != nil {
		t.Fatalf("UpsertSampleProjection: %v", err)
	}

	server, err := web.NewServer(repo)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/clusters?projection=tsne", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var payload []struct {
		X       float64
		Y       float64
		Members []struct {
			X float64
			Y float64
		}
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(payload) != 1 || len(payload[0].Members) != 1 {
		t.Fatalf("expected 1 cluster with 1 member, got %#v", payload)
	}
	if payload[0].Members[0].X != 42.5 || payload[0].Members[0].Y != -84.5 {
		t.Fatalf("expected persisted coordinates (42.5, -84.5), got (%f, %f)",
			payload[0].Members[0].X, payload[0].Members[0].Y)
	}
}

func TestServerSerializesZeroZeroCoordinatesInJSON(t *testing.T) {
	t.Parallel()

	repo := testRepository(t)
	sampleID := insertSample(t, repo, "tone.wav")

	clusterID, err := repo.InsertCluster(context.Background(), core.Cluster{
		Method: "kmeans",
		Label:  "Cluster 1",
	})
	if err != nil {
		t.Fatalf("InsertCluster: %v", err)
	}
	if err := repo.InsertClusterMember(context.Background(), db.ClusterMember{
		ClusterID: clusterID,
		SampleID:  sampleID,
		Score:     1.0,
	}); err != nil {
		t.Fatalf("InsertClusterMember: %v", err)
	}

	// Persist 0.0, 0.0 coordinates
	if err := repo.UpsertSampleProjection(context.Background(), core.SampleProjection{
		SampleID:  sampleID,
		Method:    "tsne",
		Namespace: "analysis",
		X:         0.0,
		Y:         0.0,
	}); err != nil {
		t.Fatalf("UpsertSampleProjection: %v", err)
	}

	server, err := web.NewServer(repo)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/clusters?projection=tsne", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	// Ensure "x":0 and "y":0 are present and not omitted
	if !strings.Contains(body, `"x":0`) && !strings.Contains(body, `"x":0.0`) {
		t.Fatalf("expected JSON body to contain '\"x\":0', got:\n%s", body)
	}
	if !strings.Contains(body, `"y":0`) && !strings.Contains(body, `"y":0.0`) {
		t.Fatalf("expected JSON body to contain '\"y\":0', got:\n%s", body)
	}
}

func TestListenAndServeVerboseLogsBindAndRequests(t *testing.T) {
	t.Parallel()

	repo := testRepository(t)
	serverLogger := &bytes.Buffer{}
	logger := log.New(serverLogger, "", 0)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	defer func() { _ = listener.Close() }()
	addr := listener.Addr().String()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- web.Serve(ctx, listener, repo, logger)
	}()

	var healthErr error
	for range 50 {
		select {
		case err := <-errCh:
			t.Fatalf("Serve exited early with: %v", err)
		default:
		}
		resp, err := http.Get("http://" + addr + "/api/health")
		if err == nil {
			_ = resp.Body.Close()
			healthErr = nil
			break
		}
		healthErr = err
		time.Sleep(20 * time.Millisecond)
	}
	if healthErr != nil {
		t.Fatalf("health request never succeeded: %v", healthErr)
	}

	cancel()
	if err := <-errCh; err != context.Canceled {
		t.Fatalf("expected context canceled, got %v", err)
	}

	logOutput := serverLogger.String()
	for _, want := range []string{"serving web ui on http://" + addr, "GET /api/health 200"} {
		if !strings.Contains(logOutput, want) {
			t.Fatalf("expected %q in log output %q", want, logOutput)
		}
	}
}

func testRepository(t *testing.T) *db.Repository {
	t.Helper()

	database, err := db.Open(context.Background(), db.Options{
		Path: filepath.Join(t.TempDir(), "zounds.db"),
	})
	if err != nil {
		t.Fatalf("db.Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return db.NewRepository(database)
}

func insertSample(t *testing.T, repo *db.Repository, name string) int64 {
	t.Helper()

	tempDir := t.TempDir()
	sampleID, err := repo.UpsertSample(context.Background(), core.Sample{
		SourceRoot:   tempDir,
		Path:         filepath.Join(tempDir, name),
		RelativePath: name,
		FileName:     name,
		Extension:    "wav",
		Format:       core.FormatWAV,
		SizeBytes:    42,
		ModifiedAt:   time.Now(),
		ScannedAt:    time.Now(),
	})
	if err != nil {
		t.Fatalf("UpsertSample returned error: %v", err)
	}
	return sampleID
}

func itoa(value int64) string {
	return strconv.FormatInt(value, 10)
}
