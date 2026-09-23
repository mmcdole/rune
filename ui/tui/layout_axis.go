package tui

import (
	"errors"
	"fmt"
	"github.com/mmcdole/rune/ui"
	"slices"
	"sort"
)

// axisTrack is the measured, main-axis input to allocateAxis. Min is zero
// when omitted, Max is zero when unbounded, and Auto is the measured preferred
// size used only by ui.LayoutSizeAuto.
type axisTrack struct {
	Size ui.LayoutSize
	Min  int
	Max  int
	Auto int
}

// errLayoutTooSmall reports that an axis cannot honor its tracks' minima.
var errLayoutTooSmall = errors.New("layout extent is smaller than its minimum sizes")

// allocateAxis resolves measured tracks into child sizes for one container
// axis. Extent is the space available after container boundaries are accounted for.
// Uncapped fractional tracks consume all remaining cells. If there are no
// such tracks, unused cells are intentionally left at the end of the axis.
//
// Fixed, percentage, and auto sizes are preferred sizes. Fractional tracks
// divide the remainder. If preferred sizes overcommit the axis, tracks shrink
// toward Min in this order: fractional, percentage, then fixed and auto.
// Max caps every track. An impossible minimum returns errLayoutTooSmall.
func allocateAxis(extent int, tracks []axisTrack) ([]int, error) {
	if extent < 0 {
		return nil, fmt.Errorf("layout extent must not be negative")
	}

	if len(tracks) > ui.MaxLayoutNodes {
		return nil, fmt.Errorf("layout axis exceeds the limit of %d tracks", ui.MaxLayoutNodes)
	}
	if len(tracks) == 0 {
		return []int{}, nil
	}

	available := extent

	normalized := make([]axisTrack, len(tracks))
	minimum := 0
	for i, track := range tracks {
		if track.Size.Kind == ui.LayoutSizeDefault {
			track.Size = ui.Fraction(1)
		}
		if err := ui.ValidateLayoutSize(track.Size); err != nil {
			return nil, fmt.Errorf("track %d: %w", i+1, err)
		}
		if track.Min < 0 || track.Min > ui.MaxLayoutCells {
			return nil, fmt.Errorf("track %d: min must be between 0 and %d", i+1, ui.MaxLayoutCells)
		}
		if track.Max < 0 || track.Max > ui.MaxLayoutCells {
			return nil, fmt.Errorf("track %d: max must be between 0 and %d", i+1, ui.MaxLayoutCells)
		}
		if track.Max > 0 && track.Min > track.Max {
			return nil, fmt.Errorf("track %d: min must not exceed max", i+1)
		}
		if track.Size.Kind == ui.LayoutSizeAuto && (track.Auto < 0 || track.Auto > ui.MaxLayoutCells) {
			return nil, fmt.Errorf("track %d: auto size must be between 0 and %d", i+1, ui.MaxLayoutCells)
		}
		normalized[i] = track
		minimum += track.Min
	}
	if minimum > available {
		return nil, fmt.Errorf("%w: tracks need at least %d cells, have %d",
			errLayoutTooSmall, minimum, extent)
	}

	sizes := make([]int, len(normalized))
	flexIndices := make([]int, 0, len(normalized))
	flexWeights := make([]int, 0, len(normalized))
	nonFlexTotal := 0
	for i, track := range normalized {
		var target int
		switch track.Size.Kind {
		case ui.LayoutSizeCells:
			target = track.Size.Value
		case ui.LayoutSizePercent:
			target = roundedPercent(available, track.Size.Value)
		case ui.LayoutSizeAuto:
			target = track.Auto
		case ui.LayoutSizeFraction:
			flexIndices = append(flexIndices, i)
			flexWeights = append(flexWeights, track.Size.Value)
			continue
		}
		sizes[i] = clampTrack(target, track)
		nonFlexTotal += sizes[i]
	}

	if remainder := available - nonFlexTotal; remainder > 0 && len(flexIndices) > 0 {
		shares := proportional(remainder, flexWeights)
		for j, index := range flexIndices {
			sizes[index] = clampTrack(shares[j], normalized[index])
		}
	} else {
		for _, index := range flexIndices {
			sizes[index] = normalized[index].Min
		}
	}

	total := sumInts(sizes)
	if total > available {
		over := total - available
		over = shrinkTracks(sizes, normalized, over, ui.LayoutSizeFraction)
		over = shrinkTracks(sizes, normalized, over, ui.LayoutSizePercent)
		over = shrinkTracks(sizes, normalized, over, ui.LayoutSizeCells, ui.LayoutSizeAuto)
		if over > 0 {
			return nil, fmt.Errorf("%w: tracks need %d cells, have %d",
				errLayoutTooSmall, available+over, extent)
		}
	}

	if remaining := available - sumInts(sizes); remaining > 0 {
		growFractions(sizes, normalized, remaining)
	}
	return sizes, nil
}

func roundedPercent(total, percent int) int {
	whole := (total / 100) * percent
	remainder := (total % 100) * percent
	return whole + (remainder+50)/100
}

func clampTrack(size int, track axisTrack) int {
	size = max(size, track.Min)
	if track.Max > 0 {
		size = min(size, track.Max)
	}
	return size
}

// proportional divides total by positive weights using largest-remainder
// rounding. Equal remainders favor the earlier track.
func proportional(total int, weights []int) []int {
	shares := make([]int, len(weights))
	if total <= 0 || len(weights) == 0 {
		return shares
	}
	weightTotal := sumInts(weights)
	type residue struct {
		index int
		value int64
	}
	residues := make([]residue, len(weights))
	allocated := 0
	whole := total / weightTotal
	remainder := total % weightTotal
	for i, weight := range weights {
		product := int64(remainder) * int64(weight)
		shares[i] = whole*weight + int(product/int64(weightTotal))
		allocated += shares[i]
		residues[i] = residue{index: i, value: product % int64(weightTotal)}
	}
	sort.SliceStable(residues, func(i, j int) bool {
		return residues[i].value > residues[j].value
	})
	for i := 0; i < total-allocated; i++ {
		shares[residues[i].index]++
	}
	return shares
}

func shrinkTracks(sizes []int, tracks []axisTrack, amount int, kinds ...ui.LayoutSizeKind) int {
	if amount <= 0 {
		return 0
	}
	indices := make([]int, 0, len(tracks))
	capacities := make([]int, 0, len(tracks))
	totalCapacity := 0
	for i, track := range tracks {
		if !slices.Contains(kinds, track.Size.Kind) || sizes[i] <= track.Min {
			continue
		}
		capacity := sizes[i] - track.Min
		indices = append(indices, i)
		capacities = append(capacities, capacity)
		totalCapacity += capacity
	}
	if totalCapacity == 0 {
		return amount
	}
	take := min(amount, totalCapacity)
	reductions := proportional(take, capacities)
	for i, index := range indices {
		sizes[index] -= reductions[i]
	}
	return amount - take
}

func growFractions(sizes []int, tracks []axisTrack, amount int) int {
	for amount > 0 {
		indices := make([]int, 0, len(tracks))
		weights := make([]int, 0, len(tracks))
		for i, track := range tracks {
			if track.Size.Kind != ui.LayoutSizeFraction || (track.Max > 0 && sizes[i] >= track.Max) {
				continue
			}
			indices = append(indices, i)
			weights = append(weights, track.Size.Value)
		}
		if len(indices) == 0 {
			return amount
		}

		grants := proportional(amount, weights)
		consumed := 0
		for i, index := range indices {
			grant := grants[i]
			if maxSize := tracks[index].Max; maxSize > 0 {
				grant = min(grant, maxSize-sizes[index])
			}
			sizes[index] += grant
			consumed += grant
		}
		if consumed == 0 {
			return amount
		}
		amount -= consumed
	}
	return 0
}

func sumInts(values []int) int {
	total := 0
	for _, value := range values {
		total += value
	}
	return total
}
