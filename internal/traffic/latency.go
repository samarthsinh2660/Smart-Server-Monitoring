package traffic

import "math"

var latencyUpperMS = [10]float64{10, 25, 50, 100, 200, 400, 800, 1600, 3200, 6400}

func LatencyBucket(milliseconds float64) int {
	for i, upper := range latencyUpperMS {
		if milliseconds <= upper {
			return i
		}
	}
	return len(latencyUpperMS)
}

// P95Latency returns a histogram upper bound in milliseconds for the interval.
func P95Latency(current, previous [11]uint64) (float64, bool) {
	var total uint64
	var deltas [11]uint64
	for i := range current {
		if current[i] < previous[i] {
			return 0, false
		}
		deltas[i] = current[i] - previous[i]
		total += deltas[i]
	}
	if total == 0 {
		return 0, false
	}
	rank := uint64(math.Ceil(.95 * float64(total)))
	var count uint64
	for i, delta := range deltas {
		count += delta
		if count >= rank {
			if i == len(latencyUpperMS) {
				return 0, false
			}
			return latencyUpperMS[i], true
		}
	}
	return 0, false
}
