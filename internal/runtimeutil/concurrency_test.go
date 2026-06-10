package runtimeutil

import "testing"

func TestEffectiveConcurrencyDefaultsToOne(t *testing.T) {
	tests := []struct {
		name string
		in   int
		want int
	}{
		{name: "zero", in: 0, want: 1},
		{name: "negative", in: -3, want: 1},
		{name: "positive", in: 4, want: 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EffectiveConcurrency(tt.in); got != tt.want {
				t.Fatalf("EffectiveConcurrency(%d) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestEffectivePrefetchFactorDefaultsToOne(t *testing.T) {
	tests := []struct {
		name string
		in   int
		want int
	}{
		{name: "zero", in: 0, want: 1},
		{name: "negative", in: -2, want: 1},
		{name: "positive", in: 3, want: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EffectivePrefetchFactor(tt.in); got != tt.want {
				t.Fatalf("EffectivePrefetchFactor(%d) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestClaimLimitUsesBatchAvailableAndPrefetch(t *testing.T) {
	tests := []struct {
		name           string
		batchSize      int
		availableSlots int
		prefetchFactor int
		want           int
	}{
		{name: "bounded by batch", batchSize: 5, availableSlots: 3, prefetchFactor: 2, want: 5},
		{name: "bounded by slots times prefetch", batchSize: 50, availableSlots: 3, prefetchFactor: 2, want: 6},
		{name: "zero batch", batchSize: 0, availableSlots: 3, prefetchFactor: 2, want: 0},
		{name: "zero slots", batchSize: 50, availableSlots: 0, prefetchFactor: 2, want: 0},
		{name: "default prefetch", batchSize: 50, availableSlots: 3, prefetchFactor: 0, want: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClaimLimit(tt.batchSize, tt.availableSlots, tt.prefetchFactor)
			if got != tt.want {
				t.Fatalf("ClaimLimit(%d,%d,%d) = %d, want %d", tt.batchSize, tt.availableSlots, tt.prefetchFactor, got, tt.want)
			}
		})
	}
}
