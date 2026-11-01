package dedup

import (
	"fmt"
	"math/bits"
	"sort"
	"strconv"

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

	parsed := make([]uint64, len(hashes))
	for i, h := range hashes {
		val, err := strconv.ParseUint(h.Hash, 16, 64)
		if err != nil {
			return nil, fmt.Errorf("compare perceptual hashes for %q: parse hash %q: %w", h.Sample.Path, h.Hash, err)
		}
		parsed[i] = val
	}

	dist := func(i, j int) int {
		return bits.OnesCount64(parsed[i] ^ parsed[j])
	}

	// 4x16-bit LSH buckets:
	// A 64-bit fingerprint is partitioned into 4 16-bit chunks.
	var tables [4]map[uint16][]int
	for t := 0; t < 4; t++ {
		tables[t] = make(map[uint16][]int)
	}
	for i, v := range parsed {
		c0 := uint16(v & 0xffff)
		c1 := uint16((v >> 16) & 0xffff)
		c2 := uint16((v >> 32) & 0xffff)
		c3 := uint16((v >> 48) & 0xffff)
		tables[0][c0] = append(tables[0][c0], i)
		tables[1][c1] = append(tables[1][c1], i)
		tables[2][c2] = append(tables[2][c2], i)
		tables[3][c3] = append(tables[3][c3], i)
	}

	neighbors := make([][]int, len(hashes))
	if len(hashes) <= 500 {
		// For small collections, exhaustively check all pairs to guarantee 100% recall
		for i := 0; i < len(hashes); i++ {
			for j := i + 1; j < len(hashes); j++ {
				if d := dist(i, j); d <= f.MaxDistance {
					neighbors[i] = append(neighbors[i], j)
					neighbors[j] = append(neighbors[j], i)
				}
			}
		}
	} else {
		// For large collections, generate candidate pairs from 4x16-bit LSH collisions
		checked := make(map[uint64]struct{})
		for t := 0; t < 4; t++ {
			for _, bucket := range tables[t] {
				if len(bucket) < 2 {
					continue
				}
				for bi := 0; bi < len(bucket); bi++ {
					for bj := bi + 1; bj < len(bucket); bj++ {
						u, v := bucket[bi], bucket[bj]
						if u > v {
							u, v = v, u
						}
						pairKey := (uint64(u) << 32) | uint64(v)
						if _, exists := checked[pairKey]; exists {
							continue
						}
						checked[pairKey] = struct{}{}
						if d := dist(u, v); d <= f.MaxDistance {
							neighbors[u] = append(neighbors[u], v)
							neighbors[v] = append(neighbors[v], u)
						}
					}
				}
			}
		}
	}

	// Bounded-Diameter Grouping (Clique Partitioning):
	// Form duplicate groups such that every pair of samples in a group has distance <= MaxDistance.
	// This strictly bounds the diameter to <= MaxDistance and eliminates transitive chaining.
	assigned := make([]bool, len(hashes))
	var groups []PerceptualGroup

	for {
		// Find unassigned seed with the most unassigned neighbors (densest cluster first)
		bestSeed := -1
		bestUnassignedCount := -1

		for i := 0; i < len(hashes); i++ {
			if assigned[i] {
				continue
			}
			unassignedCount := 0
			for _, n := range neighbors[i] {
				if !assigned[n] {
					unassignedCount++
				}
			}
			if unassignedCount > bestUnassignedCount ||
				(unassignedCount == bestUnassignedCount && bestSeed >= 0 && hashes[i].Sample.Path < hashes[bestSeed].Sample.Path) {
				bestSeed = i
				bestUnassignedCount = unassignedCount
			}
		}

		if bestSeed < 0 || bestUnassignedCount == 0 {
			break
		}

		// Candidate group starting with bestSeed
		groupMembers := []int{bestSeed}

		// Filter candidate neighbors that are unassigned
		var candNeighbors []int
		for _, n := range neighbors[bestSeed] {
			if !assigned[n] {
				candNeighbors = append(candNeighbors, n)
			}
		}

		// Sort candidate neighbors by distance to seed ascending, then by sample path
		sort.Slice(candNeighbors, func(i, j int) bool {
			di := dist(bestSeed, candNeighbors[i])
			dj := dist(bestSeed, candNeighbors[j])
			if di != dj {
				return di < dj
			}
			return hashes[candNeighbors[i]].Sample.Path < hashes[candNeighbors[j]].Sample.Path
		})

		// Add candidates that preserve the bounded diameter (distance <= MaxDistance to ALL members)
		for _, cand := range candNeighbors {
			bounded := true
			for _, m := range groupMembers {
				if dist(cand, m) > f.MaxDistance {
					bounded = false
					break
				}
			}
			if bounded {
				groupMembers = append(groupMembers, cand)
			}
		}

		if len(groupMembers) < 2 {
			assigned[bestSeed] = true
			continue
		}

		for _, m := range groupMembers {
			assigned[m] = true
		}

		// Deterministic reference selection by sample path
		sort.Slice(groupMembers, func(i, j int) bool {
			return hashes[groupMembers[i]].Sample.Path < hashes[groupMembers[j]].Sample.Path
		})

		refIndex := groupMembers[0]
		group := PerceptualGroup{
			Reference: hashes[refIndex].Sample,
			Hash:      hashes[refIndex].Hash,
			Matches:   make([]PerceptualMatch, 0, len(groupMembers)-1),
		}

		for _, m := range groupMembers[1:] {
			d := dist(refIndex, m)
			group.Matches = append(group.Matches, PerceptualMatch{
				Sample:     hashes[m].Sample,
				Hash:       hashes[m].Hash,
				Distance:   d,
				Similarity: 1.0 - float64(d)/64.0,
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
