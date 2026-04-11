package dedup

import (
	"fmt"
	"sort"

	"github.com/darkliquid/zounds/pkg/analysis"
	"github.com/darkliquid/zounds/pkg/core"
)

type PerceptualHash struct {
	Sample core.Sample
	Hash   string
}

type PerceptualMatch struct {
	Sample     core.Sample
	Hash       string
	Distance   int
	Similarity float64
}

type PerceptualGroup struct {
	Reference core.Sample
	Hash      string
	Matches   []PerceptualMatch
}

type PerceptualFinder struct {
	MaxDistance int
}

func NewPerceptualFinder(maxDistance int) PerceptualFinder {
	if maxDistance <= 0 {
		maxDistance = 8
	}
	return PerceptualFinder{MaxDistance: maxDistance}
}

func (f PerceptualFinder) Find(hashes []PerceptualHash) ([]PerceptualGroup, error) {
	if len(hashes) < 2 {
		return nil, nil
	}

	parent := make([]int, len(hashes))
	for i := range parent {
		parent[i] = i
	}

	find := func(x int) int {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}

	union := func(a, b int) {
		rootA := find(a)
		rootB := find(b)
		if rootA != rootB {
			parent[rootB] = rootA
		}
	}

	for i := 0; i < len(hashes); i++ {
		for j := i + 1; j < len(hashes); j++ {
			distance, err := analysis.HammingDistanceHex(hashes[i].Hash, hashes[j].Hash)
			if err != nil {
				return nil, fmt.Errorf("compare perceptual hashes for %q and %q: %w", hashes[i].Sample.Path, hashes[j].Sample.Path, err)
			}
			if distance <= f.MaxDistance {
				union(i, j)
			}
		}
	}

	groupMembers := make(map[int][]int)
	for i := range hashes {
		root := find(i)
		groupMembers[root] = append(groupMembers[root], i)
	}

	var groups []PerceptualGroup
	for _, members := range groupMembers {
		if len(members) < 2 {
			continue
		}
		sort.Slice(members, func(i, j int) bool {
			return hashes[members[i]].Sample.Path < hashes[members[j]].Sample.Path
		})

		referenceIndex := members[0]
		group := PerceptualGroup{
			Reference: hashes[referenceIndex].Sample,
			Hash:      hashes[referenceIndex].Hash,
			Matches:   make([]PerceptualMatch, 0, len(members)-1),
		}

		for _, memberIndex := range members[1:] {
			distance, err := analysis.HammingDistanceHex(hashes[referenceIndex].Hash, hashes[memberIndex].Hash)
			if err != nil {
				return nil, err
			}
			group.Matches = append(group.Matches, PerceptualMatch{
				Sample:     hashes[memberIndex].Sample,
				Hash:       hashes[memberIndex].Hash,
				Distance:   distance,
				Similarity: 1 - float64(distance)/64.0,
			})
		}

		sort.Slice(group.Matches, func(i, j int) bool {
			if group.Matches[i].Distance == group.Matches[j].Distance {
				return group.Matches[i].Sample.Path < group.Matches[j].Sample.Path
			}
			return group.Matches[i].Distance < group.Matches[j].Distance
		})

		groups = append(groups, group)
	}

	sort.Slice(groups, func(i, j int) bool {
		if len(groups[i].Matches) == len(groups[j].Matches) {
			return groups[i].Reference.Path < groups[j].Reference.Path
		}
		return len(groups[i].Matches) > len(groups[j].Matches)
	})

	return groups, nil
}

func PlanPerceptualCull(groups []PerceptualGroup, strategy KeepStrategy) []CullAction {
	actions := make([]CullAction, 0, len(groups))
	for _, group := range groups {
		samples := []core.Sample{group.Reference}
		for _, match := range group.Matches {
			samples = append(samples, match.Sample)
		}
		sortSamplesForStrategy(samples, strategy)

		actions = append(actions, CullAction{
			Hash:   group.Hash,
			Keep:   samples[0],
			Remove: append([]core.Sample(nil), samples[1:]...),
		})
	}
	return actions
}
