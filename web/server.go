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

	"github.com/darkliquid/zounds/pkg/cluster"
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
	s.mux.Handle("/api/tags/", http.HandlerFunc(s.handleTagRoutes))
	s.mux.Handle("/api/clusters", http.HandlerFunc(s.handleClusters))
	s.mux.Handle("/api/clusters/", http.HandlerFunc(s.handleClusterRoutes))
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
	query := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))

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

	if query != "" {
		samples = filterSamplesByQuery(samples, query)
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

func (s *Server) handleTagRoutes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rest := strings.TrimPrefix(r.URL.Path, "/api/tags/")
	rest = strings.Trim(rest, "/")
	if rest == "" {
		http.NotFound(w, r)
		return
	}

	parts := strings.Split(rest, "/")
	if len(parts) != 2 || parts[1] != "samples" {
		http.NotFound(w, r)
		return
	}

	samples, err := s.repo.FindSamplesByTag(r.Context(), parts[0])
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	out := make([]sampleResponse, 0, len(samples))
	for _, sample := range samples {
		tags, err := s.repo.ListTagsForSample(r.Context(), sample.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		out = append(out, sampleResponse{Sample: sample, Tags: tags})
	}

	writeJSON(w, http.StatusOK, out)
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
	projection := strings.TrimSpace(r.URL.Query().Get("projection"))
	if projection == "" {
		projection = "tsne"
	}

	clusters, err := s.repo.ListClustersByMethod(r.Context(), method)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	type clusterResponse struct {
		core.Cluster
		Samples []int64 `json:"samples,omitempty"`
		X       float64 `json:"x,omitempty"`
		Y       float64 `json:"y,omitempty"`
	}

	pointBySample, err := s.projectClusterMembers(r.Context(), projection)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	out := make([]clusterResponse, 0, len(clusters))
	for _, cluster := range clusters {
		members, err := s.repo.ListClusterMembers(r.Context(), cluster.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		samples := make([]int64, 0, len(members))
		var sumX, sumY float64
		for _, member := range members {
			samples = append(samples, member.SampleID)
			if point, ok := pointBySample[member.SampleID]; ok {
				sumX += point.X
				sumY += point.Y
			}
		}
		response := clusterResponse{Cluster: cluster, Samples: samples}
		if len(samples) > 0 {
			response.X = sumX / float64(len(samples))
			response.Y = sumY / float64(len(samples))
		}
		out = append(out, response)
	}

	writeJSON(w, http.StatusOK, out)
}

func (s *Server) projectClusterMembers(ctx context.Context, method string) (map[int64]cluster.ProjectionPoint, error) {
	vectors, err := s.repo.ListFeatureVectors(ctx, "analysis")
	if err != nil {
		return nil, err
	}
	points, err := cluster.Project2DByMethod(vectors, method)
	if err != nil {
		return nil, err
	}
	pointBySample := make(map[int64]cluster.ProjectionPoint, len(points))
	for _, point := range points {
		pointBySample[point.SampleID] = point
	}
	return pointBySample, nil
}

func (s *Server) handleClusterRoutes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rest := strings.TrimPrefix(r.URL.Path, "/api/clusters/")
	rest = strings.Trim(rest, "/")
	if rest == "" {
		http.NotFound(w, r)
		return
	}

	id, err := strconv.ParseInt(rest, 10, 64)
	if err != nil {
		http.Error(w, "invalid cluster id", http.StatusBadRequest)
		return
	}

	cluster, members, found, err := s.findClusterByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if !found {
		http.NotFound(w, r)
		return
	}

	type clusterResponse struct {
		core.Cluster
		Samples []int64 `json:"samples,omitempty"`
	}
	samples := make([]int64, 0, len(members))
	for _, member := range members {
		samples = append(samples, member.SampleID)
	}
	writeJSON(w, http.StatusOK, clusterResponse{Cluster: cluster, Samples: samples})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func filterSamplesByQuery(samples []core.Sample, query string) []core.Sample {
	filtered := make([]core.Sample, 0, len(samples))
	for _, sample := range samples {
		if strings.Contains(strings.ToLower(sample.Path), query) || strings.Contains(strings.ToLower(sample.FileName), query) {
			filtered = append(filtered, sample)
		}
	}
	return filtered
}

func (s *Server) findClusterByID(ctx context.Context, id int64) (core.Cluster, []db.ClusterMember, bool, error) {
	for _, method := range []string{"kmeans", "dbscan"} {
		clusters, err := s.repo.ListClustersByMethod(ctx, method)
		if err != nil {
			return core.Cluster{}, nil, false, err
		}
		for _, cluster := range clusters {
			if cluster.ID != id {
				continue
			}
			members, err := s.repo.ListClusterMembers(ctx, id)
			if err != nil {
				return core.Cluster{}, nil, false, err
			}
			return cluster, members, true, nil
		}
	}
	return core.Cluster{}, nil, false, nil
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
