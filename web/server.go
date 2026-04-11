package web

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/darkliquid/zounds/pkg/core"
	"github.com/darkliquid/zounds/pkg/db"
)

type Server struct {
	repo *db.Repository
	mux  *http.ServeMux
}

func NewServer(repo *db.Repository) (*Server, error) {
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		return nil, fmt.Errorf("load static assets: %w", err)
	}

	s := &Server{
		repo: repo,
		mux:  http.NewServeMux(),
	}

	s.mux.Handle("/api/health", http.HandlerFunc(s.handleHealth))
	s.mux.Handle("/api/samples", http.HandlerFunc(s.handleSamples))
	s.mux.Handle("/api/samples/", http.HandlerFunc(s.handleSampleRoutes))
	s.mux.Handle("/api/tags", http.HandlerFunc(s.handleTags))
	s.mux.Handle("/api/clusters", http.HandlerFunc(s.handleClusters))
	s.mux.Handle("/", http.FileServer(http.FS(staticFS)))

	return s, nil
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

type sampleResponse struct {
	core.Sample
	Tags []core.Tag `json:"tags,omitempty"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleSamples(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()
	tag := strings.TrimSpace(r.URL.Query().Get("tag"))

	var (
		samples []core.Sample
		err     error
	)
	if tag != "" {
		samples, err = s.repo.FindSamplesByTag(ctx, tag)
	} else {
		samples, err = s.repo.ListSamples(ctx)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	out := make([]sampleResponse, 0, len(samples))
	for _, sample := range samples {
		tags, err := s.repo.ListTagsForSample(ctx, sample.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		out = append(out, sampleResponse{Sample: sample, Tags: tags})
	}

	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleSampleRoutes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rest := strings.TrimPrefix(r.URL.Path, "/api/samples/")
	rest = strings.Trim(rest, "/")
	if rest == "" {
		http.NotFound(w, r)
		return
	}

	parts := strings.Split(rest, "/")
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		http.Error(w, "invalid sample id", http.StatusBadRequest)
		return
	}

	switch {
	case len(parts) == 1:
		s.handleSampleByID(w, r, id)
	case len(parts) == 2 && parts[1] == "audio":
		s.handleSampleAudio(w, r, id)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleSampleByID(w http.ResponseWriter, r *http.Request, id int64) {
	ctx := r.Context()
	sample, err := s.repo.FindSampleByID(ctx, id)
	if err != nil {
		if err == sql.ErrNoRows {
			http.NotFound(w, r)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	tags, err := s.repo.ListTagsForSample(ctx, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, sampleResponse{Sample: sample, Tags: tags})
}

func (s *Server) handleSampleAudio(w http.ResponseWriter, r *http.Request, id int64) {
	sample, err := s.repo.FindSampleByID(r.Context(), id)
	if err != nil {
		if err == sql.ErrNoRows {
			http.NotFound(w, r)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	http.ServeFile(w, r, sample.Path)
}

func (s *Server) handleTags(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tags, err := s.repo.ListTagUsage(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, tags)
}

func (s *Server) handleClusters(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	method := strings.TrimSpace(r.URL.Query().Get("method"))
	if method == "" {
		method = "kmeans"
	}

	clusters, err := s.repo.ListClustersByMethod(r.Context(), method)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	type clusterResponse struct {
		core.Cluster
		Samples []int64 `json:"samples,omitempty"`
	}

	out := make([]clusterResponse, 0, len(clusters))
	for _, cluster := range clusters {
		members, err := s.repo.ListClusterMembers(r.Context(), cluster.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		samples := make([]int64, 0, len(members))
		for _, member := range members {
			samples = append(samples, member.SampleID)
		}
		out = append(out, clusterResponse{Cluster: cluster, Samples: samples})
	}

	writeJSON(w, http.StatusOK, out)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func ListenAndServe(ctx context.Context, addr string, repo *db.Repository) error {
	server, err := NewServer(repo)
	if err != nil {
		return err
	}

	httpServer := &http.Server{
		Addr:    addr,
		Handler: server,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- httpServer.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		_ = httpServer.Shutdown(context.Background())
		return ctx.Err()
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

func staticAssetPath(name string) string {
	return path.Clean("/" + strings.TrimSpace(name))
}
