package metrics

import "time"

// Sample is one complete reading from a laptop agent. Network counters are cumulative.
type Sample struct {
	Hostname       string    `json:"hostname"`
	SampledAt      time.Time `json:"sampled_at"`
	CPUPercent     float64   `json:"cpu_percent"`
	MemoryPercent  float64   `json:"memory_percent"`
	DiskPercent    float64   `json:"disk_percent"`
	NetworkRXBytes uint64    `json:"network_rx_bytes"`
	NetworkTXBytes uint64    `json:"network_tx_bytes"`
	UptimeSeconds  uint64    `json:"uptime_seconds"`
	TemperatureC   *float64  `json:"temperature_c,omitempty"`
}
