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
	if err := repo.AttachTagToSample(context.Background(), sampleID, tagID); err != nil {
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
		FileName string
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

func TestListenAndServeVerboseLogsBindAndRequests(t *testing.T) {
	t.Parallel()

	repo := testRepository(t)
	serverLogger := &bytes.Buffer{}
	logger := log.New(serverLogger, "", 0)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	addr := listener.Addr().String()
	_ = listener.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- web.ListenAndServe(ctx, addr, repo, logger)
	}()

	var healthErr error
	for range 50 {
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
