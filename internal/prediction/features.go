package prediction

import (
	"fmt"
	"math"
	"time"

	"smartserver/internal/metrics"
	"smartserver/internal/traffic"
)

var FeatureNames = [8]string{
	"cpu_mean_percent", "cpu_trend_percent_per_minute", "memory_percent",
	"request_rate_per_second", "request_trend_per_minute",
	"p95_latency_ms", "p95_latency_trend_ms_per_minute", "error_rate_percent",
}

type Features struct {
	CPUMeanPercent        float64 `json:"cpu_mean_percent"`
	CPUTrendPerMinute     float64 `json:"cpu_trend_percent_per_minute"`
	MemoryPercent         float64 `json:"memory_percent"`
	RequestRatePerSecond  float64 `json:"request_rate_per_second"`
	RequestTrendPerMinute float64 `json:"request_trend_per_minute"`
	P95LatencyMS          float64 `json:"p95_latency_ms"`
	P95LatencyTrend       float64 `json:"p95_latency_trend_ms_per_minute"`
	ErrorRatePercent      float64 `json:"error_rate_percent"`
}

func (f Features) Values() [8]float64 {
	return [8]float64{f.CPUMeanPercent, f.CPUTrendPerMinute, f.MemoryPercent,
		f.RequestRatePerSecond, f.RequestTrendPerMinute, f.P95LatencyMS, f.P95LatencyTrend, f.ErrorRatePercent}
}

// FromSamples builds features from up to five minutes of newest-first history.
func FromSamples(now time.Time, metricSamples []metrics.Sample, trafficSamples []traffic.Sample) (Features, error) {
	if len(metricSamples) < 6 || len(trafficSamples) < 6 {
		return Features{}, fmt.Errorf("need at least six metric and traffic samples")
	}
	if stale(now, metricSamples[0].SampledAt) || stale(now, trafficSamples[0].SampledAt) {
		return Features{}, fmt.Errorf("latest metrics or traffic stats are stale")
	}
	cutoff := now.Add(-5 * time.Minute)
	var cpu, cpuTimes, rates, rateTimes []float64
	var memory float64
	for i, sample := range metricSamples {
		if sample.SampledAt.Before(cutoff) {
			break
		}
		if sample.CPUPercent < 0 || sample.CPUPercent > 100 || sample.MemoryPercent < 0 || sample.MemoryPercent > 100 {
			return Features{}, fmt.Errorf("invalid metric sample")
		}
		cpu = append(cpu, sample.CPUPercent)
		cpuTimes = append(cpuTimes, sample.SampledAt.Sub(metricSamples[0].SampledAt).Minutes())
		if i == 0 {
			memory = sample.MemoryPercent
		}
	}
	var latency, errors float64
	var latencies, latencyTimes []float64
	latencyFound := false
	trafficCount := 0
	for _, sample := range trafficSamples {
		if sample.SampledAt.Before(cutoff) {
			break
		}
		if !sample.StartedAt.Equal(trafficSamples[0].StartedAt) {
			break
		}
		trafficCount++
		if sample.RequestRate != nil {
			if !finite(*sample.RequestRate) || *sample.RequestRate < 0 {
				return Features{}, fmt.Errorf("invalid request rate")
			}
			rates = append(rates, *sample.RequestRate)
			rateTimes = append(rateTimes, sample.SampledAt.Sub(trafficSamples[0].SampledAt).Minutes())
		}
		if sample.P95LatencyMS != nil {
			if !finite(*sample.P95LatencyMS) || *sample.P95LatencyMS < 0 {
				return Features{}, fmt.Errorf("invalid p95 latency")
			}
			latencies = append(latencies, *sample.P95LatencyMS)
			latencyTimes = append(latencyTimes, sample.SampledAt.Sub(trafficSamples[0].SampledAt).Minutes())
		}
		if !latencyFound && now.Sub(sample.SampledAt) <= 30*time.Second && sample.P95LatencyMS != nil && sample.ErrorRate != nil {
			if !finite(*sample.ErrorRate) || *sample.ErrorRate < 0 || *sample.ErrorRate > 100 {
				return Features{}, fmt.Errorf("invalid latency or error rate")
			}
			latency, errors = *sample.P95LatencyMS, *sample.ErrorRate
			latencyFound = true
		}
	}
	if len(cpu) < 6 || trafficCount < 6 || len(rates) < 3 || len(latencies) < 3 || !latencyFound {
		return Features{}, fmt.Errorf("insufficient recent traffic or latency data")
	}
	return Features{
		CPUMeanPercent: mean(cpu), CPUTrendPerMinute: slope(cpuTimes, cpu), MemoryPercent: memory,
		RequestRatePerSecond: mean(rates), RequestTrendPerMinute: slope(rateTimes, rates),
		P95LatencyMS: latency, P95LatencyTrend: slope(latencyTimes, latencies), ErrorRatePercent: errors,
	}, nil
}

func stale(now, sampledAt time.Time) bool {
	return sampledAt.IsZero() || sampledAt.After(now.Add(5*time.Second)) || now.Sub(sampledAt) > 15*time.Second
}

func mean(values []float64) float64 {
	var sum float64
	for _, value := range values {
		sum += value
	}
	return sum / float64(len(values))
}

func slope(times, values []float64) float64 {
	xMean, yMean := mean(times), mean(values)
	var numerator, denominator float64
	for i, value := range values {
		delta := times[i] - xMean
		numerator += delta * (value - yMean)
		denominator += delta * delta
	}
	if denominator == 0 {
		return 0
	}
	return numerator / denominator
}

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }
