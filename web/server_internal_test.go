package web

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/darkliquid/zounds/pkg/core"
)

func TestFilterSamplesByQueryMatchesPathAndFilename(t *testing.T) {
	t.Parallel()

	samples := []core.Sample{
		{Path: "/library/drums/impact.wav", FileName: "impact.wav"},
		{Path: "/library/pads/soft.wav", FileName: "soft.wav"},
	}

	got := filterSamplesByQuery(samples, "impact")
	if len(got) != 1 || got[0].FileName != "impact.wav" {
		t.Fatalf("unexpected match set %#v", got)
	}

	got = filterSamplesByQuery(samples, "pads")
	if len(got) != 1 || got[0].FileName != "soft.wav" {
		t.Fatalf("unexpected path match set %#v", got)
	}
}

func TestWithRequestLoggingCapturesStatusAndPath(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	logger := log.New(&out, "", 0)
	handler := withRequestLogging(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}), logger)

	req := httptest.NewRequest(http.MethodPost, "/api/samples", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	logLine := out.String()
	for _, want := range []string{"POST", "/api/samples", "201"} {
		if !strings.Contains(logLine, want) {
			t.Fatalf("expected %q in log output %q", want, logLine)
		}
	}
}
