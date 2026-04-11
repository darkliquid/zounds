package web

import (
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
