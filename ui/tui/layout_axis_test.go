package tui

import (
	"errors"
	"github.com/mmcdole/rune/ui"
	"math/rand"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestAllocateAxis(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		extent int
		tracks []axisTrack
		want   []int
	}{
		{
			name:   "default fractions split an odd remainder toward the first child",
			extent: 9,
			tracks: []axisTrack{{}, {}},
			want:   []int{5, 4},
		},
		{
			name:   "weighted fractions use largest remainder rounding",
			extent: 101,
			tracks: []axisTrack{{Size: ui.Fraction(7)}, {Size: ui.Fraction(3)}},
			want:   []int{71, 30},
		},
		{
			name:   "fixed is removed before fractions",
			extent: 99,
			tracks: []axisTrack{{Size: ui.Cells(40)}, {Size: ui.Fraction(1)}},
			want:   []int{40, 59},
		},
		{
			name:   "percentage uses available extent",
			extent: 100,
			tracks: []axisTrack{{Size: ui.Percent(50)}, {Size: ui.Percent(50)}},
			want:   []int{50, 50},
		},
		{
			name:   "fraction gets remainder after fixed and percentage",
			extent: 100,
			tracks: []axisTrack{{Size: ui.Percent(30)}, {Size: ui.Cells(20)}, {Size: ui.Fraction(1)}},
			want:   []int{30, 20, 50},
		},
		{
			name:   "auto uses measured preference",
			extent: 20,
			tracks: []axisTrack{{Size: ui.Fraction(1)}, {Size: ui.AutoSize(), Auto: 3, Max: 5}},
			want:   []int{17, 3},
		},
		{
			name:   "fraction maximum redistributes remainder",
			extent: 100,
			tracks: []axisTrack{{Size: ui.Fraction(1), Max: 30}, {Size: ui.Fraction(1)}},
			want:   []int{30, 70},
		},
		{
			name:   "all capped fractions leave a trailing tail",
			extent: 100,
			tracks: []axisTrack{{Size: ui.Fraction(1), Max: 30}, {Size: ui.Fraction(1), Max: 40}},
			want:   []int{30, 40},
		},
		{
			name:   "fixed-only underfill leaves a trailing tail",
			extent: 50,
			tracks: []axisTrack{{Size: ui.Cells(20)}, {Size: ui.Cells(10)}},
			want:   []int{20, 10},
		},
		{
			name:   "fraction minimum repins and redistributes",
			extent: 20,
			tracks: []axisTrack{{Size: ui.Fraction(1), Min: 15}, {Size: ui.Fraction(1)}},
			want:   []int{15, 5},
		},
		{
			name:   "overcommit shrinks fractions before percentage and fixed",
			extent: 100,
			tracks: []axisTrack{
				{Size: ui.Percent(50)}, {Size: ui.Cells(20)},
				{Size: ui.Fraction(1), Min: 30}, {Size: ui.Fraction(1)},
			},
			want: []int{50, 20, 30, 0},
		},
		{
			name:   "overcommit shrinks percentage before fixed",
			extent: 100,
			tracks: []axisTrack{{Size: ui.Percent(70), Min: 60}, {Size: ui.Cells(50)}},
			want:   []int{60, 40},
		},
		{
			name:   "percentage rounding cannot overrun one cell",
			extent: 1,
			tracks: []axisTrack{{Size: ui.Percent(50)}, {Size: ui.Percent(50)}, {Size: ui.Percent(50)}},
			want:   []int{0, 0, 1},
		},
		{
			name:   "capped fixed and auto shrink together after fraction and percentage minima",
			extent: 9,
			tracks: []axisTrack{
				{Size: ui.Cells(4), Max: 4}, {Size: ui.AutoSize(), Auto: 10, Max: 4},
				{Size: ui.Fraction(1), Min: 2}, {Size: ui.Percent(10), Min: 2},
			},
			want: []int{2, 3, 2, 2},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := allocateAxis(test.extent, test.tracks)
			if err != nil {
				t.Fatalf("allocateAxis(): %v", err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("allocateAxis() = %v, want %v", got, test.want)
			}
			if sumInts(got) > test.extent {
				t.Fatalf("allocation %v exceeds extent %d", got, test.extent)
			}
		})
	}
}

func BenchmarkAllocateAxis(b *testing.B) {
	for _, count := range []int{2, 8, 32} {
		for _, workload := range []struct {
			name string
			pair [2]axisTrack
		}{
			{"unconstrained", [2]axisTrack{{Size: ui.Cells(10)}, {Size: ui.Fraction(1)}}},
			// Raising every other fraction above its equal share forces fraction shrink.
			{"shrink_fraction", [2]axisTrack{{Size: ui.Fraction(1), Min: 25}, {Size: ui.Fraction(1)}}},
			{"shrink_percent", [2]axisTrack{{Size: ui.Percent(80)}, {Size: ui.Percent(80)}}},
			// No earlier kind can absorb the excess; both fixed and auto must shrink.
			{"shrink_fixed_auto", [2]axisTrack{{Size: ui.Cells(30)}, {Size: ui.AutoSize(), Auto: 30, Max: 25}}},
		} {
			b.Run(workload.name+"/tracks="+strconv.Itoa(count), func(b *testing.B) {
				tracks := make([]axisTrack, count)
				for i := range tracks {
					tracks[i] = workload.pair[i%2]
				}
				b.ReportAllocs()
				for b.Loop() {
					if _, err := allocateAxis(20*count, tracks); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

func TestAllocateAxisReportsImpossibleMinimums(t *testing.T) {
	t.Parallel()

	_, err := allocateAxis(9, []axisTrack{{Min: 5}, {Min: 5}})
	if !errors.Is(err, errLayoutTooSmall) {
		t.Fatalf("allocateAxis() error = %v, want errLayoutTooSmall", err)
	}
}

func TestAllocateAxisValidatesTracks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		extent int
		tracks []axisTrack
		want   string
	}{
		{name: "negative extent", extent: -1, want: "extent must not be negative"},
		{name: "invalid size", extent: 10, tracks: []axisTrack{{Size: ui.Fraction(0)}}, want: "layout size must be between"},
		{name: "negative minimum", extent: 10, tracks: []axisTrack{{Min: -1}}, want: "min must be between"},
		{name: "minimum above maximum", extent: 10, tracks: []axisTrack{{Min: 2, Max: 1}}, want: "min must not exceed max"},
		{name: "negative auto", extent: 10, tracks: []axisTrack{{Size: ui.AutoSize(), Auto: -1}}, want: "auto size must be between"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := allocateAxis(test.extent, test.tracks)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("allocateAxis() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestAllocateAxisIgnoresAutoMeasurementOnOtherSizeKinds(t *testing.T) {
	t.Parallel()

	got, err := allocateAxis(10, []axisTrack{
		{Size: ui.Cells(4), Auto: -1},
		{Size: ui.Fraction(1), Auto: ui.MaxLayoutCells + 1},
	})
	if err != nil {
		t.Fatalf("allocateAxis() rejected irrelevant auto measurements: %v", err)
	}
	if want := []int{4, 6}; !reflect.DeepEqual(got, want) {
		t.Fatalf("allocateAxis() = %v, want %v", got, want)
	}
}

func TestAllocateAxisDeterministicProperties(t *testing.T) {
	t.Parallel()

	random := rand.New(rand.NewSource(7))
	for iteration := 0; iteration < 2000; iteration++ {
		extent := random.Intn(201)
		tracks := make([]axisTrack, 1+random.Intn(6))
		hasUncappedFraction := false
		for i := range tracks {
			track := axisTrack{Min: random.Intn(11), Auto: random.Intn(31)}
			if random.Intn(3) == 0 {
				track.Max = track.Min + 1 + random.Intn(31)
			}
			switch random.Intn(5) {
			case 0:
				track.Size = ui.LayoutSize{}
				hasUncappedFraction = hasUncappedFraction || track.Max == 0
			case 1:
				track.Size = ui.Cells(1 + random.Intn(100))
			case 2:
				track.Size = ui.Fraction(1 + random.Intn(10))
				hasUncappedFraction = hasUncappedFraction || track.Max == 0
			case 3:
				track.Size = ui.Percent(1 + random.Intn(100))
			case 4:
				track.Size = ui.AutoSize()
			}
			tracks[i] = track
		}

		got, err := allocateAxis(extent, tracks)
		if err != nil {
			if !errors.Is(err, errLayoutTooSmall) {
				t.Fatalf("iteration %d: allocateAxis(%d, %#v): %v", iteration, extent, tracks, err)
			}
			continue
		}
		again, err := allocateAxis(extent, tracks)
		if err != nil || !reflect.DeepEqual(got, again) {
			t.Fatalf("iteration %d: allocation is not deterministic: %v/%v then %v/%v", iteration, got, err, again, err)
		}
		if len(got) != len(tracks) {
			t.Fatalf("iteration %d: got %d sizes for %d tracks", iteration, len(got), len(tracks))
		}
		for i, size := range got {
			if size < tracks[i].Min || (tracks[i].Max > 0 && size > tracks[i].Max) {
				t.Fatalf("iteration %d track %d: size %d violates [%d,%d]", iteration, i, size, tracks[i].Min, tracks[i].Max)
			}
		}
		used := sumInts(got)
		if used > extent {
			t.Fatalf("iteration %d: allocation uses %d cells in extent %d", iteration, used, extent)
		}
		if hasUncappedFraction && used != extent {
			t.Fatalf("iteration %d: uncapped fraction left %d of %d cells unused: %v", iteration, extent-used, extent, got)
		}
	}
}
