package traffic

// Sample adds rates for the interval since the previous app snapshot.
// Nil rates mean there was no usable previous snapshot yet.
type Sample struct {
	Snapshot
	RequestRate  *float64 `json:"request_rate_per_second"`
	ErrorRate    *float64 `json:"error_rate_percent"`
	AvgLatencyMS *float64 `json:"avg_latency_ms"`
}
