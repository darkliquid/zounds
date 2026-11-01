package core_test

import (
	"testing"

	"github.com/darkliquid/zounds/pkg/core"
)

func TestNormalizeTagNamePreservesFacetNamespace(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input string
		want  string
	}{
		{input: "instrument:kick", want: "instrument:kick"},
		{input: "  Instrument : Sub_Bass  ", want: "instrument:sub bass"},
		{input: "character :  dark-metallic ", want: "character:dark metallic"},
		{input: "Dark_Synth-Pad", want: "dark synth pad"},
		{input: " : ", want: ""},
		{input: ":kick", want: "kick"},
		{input: "instrument:", want: "instrument"},
	}

	for _, tc := range cases {
		if got := core.NormalizeTagName(tc.input); got != tc.want {
			t.Errorf("NormalizeTagName(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
