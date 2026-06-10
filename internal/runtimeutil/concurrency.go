package runtimeutil

func EffectiveConcurrency(n int) int {
	if n <= 0 {
		return 1
	}
	return n
}

func EffectivePrefetchFactor(n int) int {
	if n <= 0 {
		return 1
	}
	return n
}

func ClaimLimit(batchSize, availableSlots, prefetchFactor int) int {
	if batchSize <= 0 || availableSlots <= 0 {
		return 0
	}
	limit := availableSlots * EffectivePrefetchFactor(prefetchFactor)
	if limit > batchSize {
		return batchSize
	}
	return limit
}
