package web

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/darkliquid/zounds/pkg/audio"
	"github.com/darkliquid/zounds/pkg/audio/codecs"
	"github.com/darkliquid/zounds/pkg/cluster"
	"github.com/darkliquid/zounds/pkg/core"
	"github.com/darkliquid/zounds/pkg/db"
	"github.com/darkliquid/zounds/pkg/pipeline"
)

type Server struct {
	repo       *db.Repository
	registry   *audio.Registry
	mux        *http.ServeMux
	audioCache map[int64][]byte
	cacheMu    sync.RWMutex
}

func NewServer(repo *db.Repository) (*Server, error) {
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		return nil, fmt.Errorf("load static assets: %w", err)
	}

	registry, err := codecs.NewRegistry()
	if err != nil {
		return nil, fmt.Errorf("init audio codecs: %w", err)
	}

	s := &Server{
		repo:       repo,
		registry:   registry,
		mux:        http.NewServeMux(),
		audioCache: make(map[int64][]byte),
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
	Tags     []core.Tag           `json:"tags,omitempty"`
	Analysis *core.SampleAnalysis `json:"analysis,omitempty"`
}

type similarMatchResponse struct {
	sampleResponse
	Similarity float64 `json:"similarity"`
	Distance   float64 `json:"distance"`
}

type clusterMemberResponse struct {
	sampleResponse
	Score float64 `json:"score,omitempty"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
}

type clusterResponse struct {
	core.Cluster
	Samples []int64                 `json:"samples,omitempty"`
	Members []clusterMemberResponse `json:"members,omitempty"`
	X       float64                 `json:"x"`
	Y       float64                 `json:"y"`
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

	sampleIDs := make([]int64, len(samples))
	for i, sample := range samples {
		sampleIDs[i] = sample.ID
	}
	tagsBySample, err := s.repo.ListTagsForSampleIDs(ctx, sampleIDs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	analysesBySample, err := s.repo.ListSampleAnalysesBySampleIDs(ctx, sampleIDs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	facetFilter := parseFacetedFilter(r)
	out := make([]sampleResponse, 0, len(samples))
	for _, sample := range samples {
		tags := tagsBySample[sample.ID]
		var analysisPtr *core.SampleAnalysis
		if a, ok := analysesBySample[sample.ID]; ok {
			analysisPtr = &a
		}
		if !matchesFacetedFilter(sample, tags, analysisPtr, facetFilter) {
			continue
		}
		out = append(out, sampleResponse{Sample: sample, Tags: tags, Analysis: analysisPtr})
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
	case len(parts) == 2 && parts[1] == "similar":
		s.handleSampleSimilar(w, r, id)
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

	format := sample.Format
	if format == "" {
		format = core.DetectFormatFromExtension(sample.Path)
	}

	// Browser HTML5 <audio> elements typically cannot play AIFF natively.
	// Transcode AIFF on-the-fly to WAV for browser preview.
	if format == core.FormatAIFF {
		s.cacheMu.RLock()
		cached, ok := s.audioCache[id]
		s.cacheMu.RUnlock()

		if ok {
			w.Header().Set("Content-Type", "audio/wav")
			http.ServeContent(w, r, sample.FileName+".wav", sample.ModifiedAt, bytes.NewReader(cached))
			return
		}

		decoded, err := audio.DecodeFile(r.Context(), s.registry, sample.Path)
		if err != nil {
			http.ServeFile(w, r, sample.Path)
			return
		}

		enc, ok := s.registry.Encoder(core.FormatWAV)
		if !ok {
			http.ServeFile(w, r, sample.Path)
			return
		}

		seeker := &memoryWriteSeeker{}
		if err := enc.Encode(r.Context(), seeker, decoded.Buffer); err != nil {
			http.ServeFile(w, r, sample.Path)
			return
		}

		wavBytes := seeker.buf
		s.cacheMu.Lock()
		if len(s.audioCache) > 100 {
			s.audioCache = make(map[int64][]byte)
		}
		s.audioCache[id] = wavBytes
		s.cacheMu.Unlock()

		w.Header().Set("Content-Type", "audio/wav")
		http.ServeContent(w, r, sample.FileName+".wav", sample.ModifiedAt, bytes.NewReader(wavBytes))
		return
	}

	http.ServeFile(w, r, sample.Path)
}

func (s *Server) handleSampleSimilar(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()
	if _, err := s.repo.FindSampleByID(ctx, id); err != nil {
		if err == sql.ErrNoRows {
			http.NotFound(w, r)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	metric := strings.TrimSpace(r.URL.Query().Get("metric"))
	if metric == "" {
		metric = "acoustic"
	}

	limit := 25
	if l := strings.TrimSpace(r.URL.Query().Get("limit")); l != "" {
		if val, err := strconv.Atoi(l); err == nil && val > 0 {
			limit = val
		}
	}

	threshold := 0.0
	if th := strings.TrimSpace(r.URL.Query().Get("threshold")); th != "" {
		if val, err := strconv.ParseFloat(th, 64); err == nil {
			threshold = val
		}
	}

	order := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("order")))
	if order != "asc" && order != "desc" {
		order = "desc"
	}

	matches, err := pipeline.FindSimilar(ctx, s.repo, []string{strconv.FormatInt(id, 10)}, pipeline.SimilarOptions{
		Metric:    metric,
		Limit:     limit,
		Threshold: threshold,
		Order:     order,
	})
	if err != nil {
		if strings.Contains(err.Error(), "no ") && strings.Contains(err.Error(), "feature vectors") {
			writeJSON(w, http.StatusOK, []similarMatchResponse{})
			return
		}
		if strings.Contains(err.Error(), "has no") && strings.Contains(err.Error(), "feature vector") {
			writeJSON(w, http.StatusOK, []similarMatchResponse{})
			return
		}
		writeError(w, http.StatusBadRequest, err)
		return
	}

	matchIDs := make([]int64, len(matches))
	for i, m := range matches {
		matchIDs[i] = m.Sample.ID
	}

	tagsBySample, err := s.repo.ListTagsForSampleIDs(ctx, matchIDs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	analysesBySample, err := s.repo.ListSampleAnalysesBySampleIDs(ctx, matchIDs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	out := make([]similarMatchResponse, len(matches))
	for i, m := range matches {
		var analysisPtr *core.SampleAnalysis
		if a, ok := analysesBySample[m.Sample.ID]; ok {
			analysisPtr = &a
		}
		out[i] = similarMatchResponse{
			sampleResponse: sampleResponse{
				Sample:   m.Sample,
				Tags:     tagsBySample[m.Sample.ID],
				Analysis: analysisPtr,
			},
			Similarity: m.Similarity,
			Distance:   m.Distance,
		}
	}

	writeJSON(w, http.StatusOK, out)
}

type memoryWriteSeeker struct {
	buf []byte
	pos int64
}

func (m *memoryWriteSeeker) Write(p []byte) (n int, err error) {
	minCap := m.pos + int64(len(p))
	if minCap > int64(len(m.buf)) {
		newBuf := make([]byte, minCap)
		copy(newBuf, m.buf)
		m.buf = newBuf
	}
	copy(m.buf[m.pos:], p)
	m.pos += int64(len(p))
	return len(p), nil
}

func (m *memoryWriteSeeker) Seek(offset int64, whence int) (int64, error) {
	var newPos int64
	switch whence {
	case io.SeekStart:
		newPos = offset
	case io.SeekCurrent:
		newPos = m.pos + offset
	case io.SeekEnd:
		newPos = int64(len(m.buf)) + offset
	default:
		return 0, fmt.Errorf("invalid whence: %d", whence)
	}
	if newPos < 0 {
		return 0, fmt.Errorf("negative position: %d", newPos)
	}
	m.pos = newPos
	return newPos, nil
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

	pointBySample, err := s.projectClusterMembers(r.Context(), projection)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	type clusterMembersEntry struct {
		cluster core.Cluster
		members []db.ClusterMember
	}
	allClusterMembers := make([]clusterMembersEntry, len(clusters))
	allSampleIDsMap := make(map[int64]bool)

	for i, cluster := range clusters {
		members, err := s.repo.ListClusterMembers(r.Context(), cluster.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		allClusterMembers[i] = clusterMembersEntry{cluster: cluster, members: members}
		for _, m := range members {
			allSampleIDsMap[m.SampleID] = true
		}
	}

	allSampleIDs := make([]int64, 0, len(allSampleIDsMap))
	for id := range allSampleIDsMap {
		allSampleIDs = append(allSampleIDs, id)
	}

	samplesMap, err := s.repo.FindSamplesByIDs(r.Context(), allSampleIDs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	tagsMap, err := s.repo.ListTagsForSampleIDs(r.Context(), allSampleIDs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	analysesMap, err := s.repo.ListSampleAnalysesBySampleIDs(r.Context(), allSampleIDs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	sampleCache := make(map[int64]sampleResponse, len(allSampleIDs))
	for _, id := range allSampleIDs {
		if s, ok := samplesMap[id]; ok {
			var aPtr *core.SampleAnalysis
			if a, ok := analysesMap[id]; ok {
				aPtr = &a
			}
			sampleCache[id] = sampleResponse{Sample: s, Tags: tagsMap[id], Analysis: aPtr}
		}
	}

	out := make([]clusterResponse, len(clusters))
	for i, entry := range allClusterMembers {
		response, err := s.buildClusterResponse(r.Context(), entry.cluster, entry.members, pointBySample, sampleCache)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		out[i] = response
	}

	writeJSON(w, http.StatusOK, out)
}

func (s *Server) projectClusterMembers(ctx context.Context, method string) (map[int64]cluster.ProjectionPoint, error) {
	projections, err := s.repo.ListSampleProjections(ctx, method, "")
	if err == nil && len(projections) > 0 {
		pointBySample := make(map[int64]cluster.ProjectionPoint, len(projections))
		for _, p := range projections {
			pointBySample[p.SampleID] = cluster.ProjectionPoint{
				SampleID: p.SampleID,
				X:        p.X,
				Y:        p.Y,
			}
		}
		return pointBySample, nil
	}

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
		_ = s.repo.UpsertSampleProjection(ctx, core.SampleProjection{
			SampleID:  point.SampleID,
			Method:    method,
			Namespace: "analysis",
			X:         point.X,
			Y:         point.Y,
		})
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

	pointBySample, err := s.projectClusterMembers(r.Context(), "tsne")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	response, err := s.buildClusterResponse(r.Context(), cluster, members, pointBySample, make(map[int64]sampleResponse))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
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

func (s *Server) buildClusterResponse(ctx context.Context, cluster core.Cluster, members []db.ClusterMember, pointBySample map[int64]cluster.ProjectionPoint, sampleCache map[int64]sampleResponse) (clusterResponse, error) {
	response := clusterResponse{
		Cluster: cluster,
		Samples: make([]int64, 0, len(members)),
		Members: make([]clusterMemberResponse, 0, len(members)),
	}
	var (
		sumX, sumY float64
		projected  int
	)
	for _, member := range members {
		response.Samples = append(response.Samples, member.SampleID)

		sample, err := s.sampleResponseByID(ctx, member.SampleID, sampleCache)
		if err != nil {
			return clusterResponse{}, err
		}

		memberResponse := clusterMemberResponse{
			sampleResponse: sample,
			Score:          member.Score,
		}
		if point, ok := pointBySample[member.SampleID]; ok {
			memberResponse.X = point.X
			memberResponse.Y = point.Y
			sumX += point.X
			sumY += point.Y
			projected++
		}
		response.Members = append(response.Members, memberResponse)
	}
	if projected > 0 {
		response.X = sumX / float64(projected)
		response.Y = sumY / float64(projected)
	}
	return response, nil
}

func (s *Server) sampleResponseByID(ctx context.Context, sampleID int64, sampleCache map[int64]sampleResponse) (sampleResponse, error) {
	if cached, ok := sampleCache[sampleID]; ok {
		return cached, nil
	}

	sample, err := s.repo.FindSampleByID(ctx, sampleID)
	if err != nil {
		return sampleResponse{}, err
	}
	tags, err := s.repo.ListTagsForSample(ctx, sampleID)
	if err != nil {
		return sampleResponse{}, err
	}

	var aPtr *core.SampleAnalysis
	if a, err := s.repo.GetSampleAnalysis(ctx, sampleID); err == nil {
		aPtr = &a
	}

	response := sampleResponse{Sample: sample, Tags: tags, Analysis: aPtr}
	sampleCache[sampleID] = response
	return response, nil
}

type facetedFilter struct {
	BPMMin      *float64
	BPMMax      *float64
	Key         string
	Chord       string
	LoopClass   string
	DurationMin *float64
	DurationMax *float64
}

func parseFacetedFilter(r *http.Request) facetedFilter {
	q := r.URL.Query()
	f := facetedFilter{
		Key:   strings.ToLower(strings.TrimSpace(q.Get("key"))),
		Chord: strings.ToLower(strings.TrimSpace(q.Get("chord"))),
	}

	loopClass := strings.ToLower(strings.TrimSpace(q.Get("loop_class")))
	if loopClass == "" {
		loopClass = strings.ToLower(strings.TrimSpace(q.Get("type")))
	}
	f.LoopClass = loopClass

	if v := strings.TrimSpace(q.Get("bpm_min")); v != "" {
		if val, err := strconv.ParseFloat(v, 64); err == nil {
			f.BPMMin = &val
		}
	}
	if v := strings.TrimSpace(q.Get("bpm_max")); v != "" {
		if val, err := strconv.ParseFloat(v, 64); err == nil {
			f.BPMMax = &val
		}
	}
	if v := strings.TrimSpace(q.Get("duration_min")); v != "" {
		if val, err := strconv.ParseFloat(v, 64); err == nil {
			f.DurationMin = &val
		}
	}
	if v := strings.TrimSpace(q.Get("duration_max")); v != "" {
		if val, err := strconv.ParseFloat(v, 64); err == nil {
			f.DurationMax = &val
		}
	}

	return f
}

func matchesFacetedFilter(sample core.Sample, tags []core.Tag, analysis *core.SampleAnalysis, f facetedFilter) bool {
	if f.BPMMin != nil {
		if analysis == nil || analysis.BPM == nil || *analysis.BPM < *f.BPMMin {
			return false
		}
	}
	if f.BPMMax != nil {
		if analysis == nil || analysis.BPM == nil || *analysis.BPM > *f.BPMMax {
			return false
		}
	}
	if f.DurationMin != nil {
		if analysis == nil || analysis.DurationSeconds == nil || *analysis.DurationSeconds < *f.DurationMin {
			return false
		}
	}
	if f.DurationMax != nil {
		if analysis == nil || analysis.DurationSeconds == nil || *analysis.DurationSeconds > *f.DurationMax {
			return false
		}
	}
	if f.LoopClass != "" {
		matched := false
		if analysis != nil && analysis.LoopClass != nil && strings.EqualFold(*analysis.LoopClass, f.LoopClass) {
			matched = true
		}
		if !matched {
			targetTag := "articulation:" + f.LoopClass
			for _, t := range tags {
				if strings.EqualFold(t.NormalizedName, targetTag) || strings.EqualFold(t.NormalizedName, f.LoopClass) {
					matched = true
					break
				}
			}
		}
		if !matched {
			return false
		}
	}
	if f.Key != "" {
		matched := false
		if analysis != nil {
			keyStr := ""
			if analysis.KeyTonic != nil {
				keyStr = *analysis.KeyTonic
				if analysis.KeyMode != nil {
					keyStr += " " + *analysis.KeyMode
				}
			}
			keyStr = strings.ToLower(keyStr)
			if strings.Contains(keyStr, f.Key) {
				matched = true
			}
		}
		if !matched {
			for _, t := range tags {
				if strings.Contains(strings.ToLower(t.NormalizedName), f.Key) {
					matched = true
					break
				}
			}
		}
		if !matched {
			return false
		}
	}
	if f.Chord != "" {
		matched := false
		if analysis != nil && analysis.Chord != nil {
			if strings.Contains(strings.ToLower(*analysis.Chord), f.Chord) {
				matched = true
			}
		}
		if !matched {
			for _, t := range tags {
				if strings.Contains(strings.ToLower(t.NormalizedName), f.Chord) {
					matched = true
					break
				}
			}
		}
		if !matched {
			return false
		}
	}

	return true
}

func (s *Server) findClusterByID(ctx context.Context, id int64) (core.Cluster, []db.ClusterMember, bool, error) {
	for _, method := range []string{"kmeans", "dbscan", "hdbscan"} {
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

func ListenAndServe(ctx context.Context, addr string, repo *db.Repository, logger *log.Logger) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer func() { _ = listener.Close() }()

	return Serve(ctx, listener, repo, logger)
}

// Serve serves HTTP requests on the given listener until ctx is cancelled.
func Serve(ctx context.Context, listener net.Listener, repo *db.Repository, logger *log.Logger) error {
	server, err := NewServer(repo)
	if err != nil {
		return err
	}

	handler := server.Handler()
	if logger != nil {
		handler = withRequestLogging(handler, logger)
		logger.Printf("serving web ui on http://%s", listener.Addr().String())
	}

	httpServer := &http.Server{
		Handler: handler,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- httpServer.Serve(listener)
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

func withRequestLogging(next http.Handler, logger *log.Logger) http.Handler {
	if logger == nil {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		recorder := &loggingResponseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		logger.Printf("%s %s %d %s", r.Method, r.URL.RequestURI(), recorder.status, time.Since(started).Round(time.Millisecond))
	})
}

type loggingResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *loggingResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
