package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"smartserver/internal/traffic"
)

func (m *Metrics) SaveTraffic(ctx context.Context, sample traffic.Sample) error {
	_, err := m.db.ExecContext(ctx, `INSERT INTO traffic_samples
		(hostname, started_at, sampled_at, request_count, error_count, duration_sum_ms, request_rate_per_second, error_rate_percent, avg_latency_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, sample.Hostname, sample.StartedAt.UTC().Format(time.RFC3339Nano), sample.SampledAt.UTC().Format(time.RFC3339Nano), sample.RequestCount, sample.ErrorCount, sample.DurationSumMS, sample.RequestRate, sample.ErrorRate, sample.AvgLatencyMS)
	if err != nil {
		return fmt.Errorf("save traffic sample: %w", err)
	}
	return nil
}

func (m *Metrics) RecentTraffic(ctx context.Context, limit int) ([]traffic.Sample, error) {
	rows, err := m.db.QueryContext(ctx, `SELECT hostname, started_at, sampled_at, request_count, error_count, duration_sum_ms, request_rate_per_second, error_rate_percent, avg_latency_ms FROM traffic_samples ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("query traffic samples: %w", err)
	}
	defer rows.Close()
	samples := make([]traffic.Sample, 0, limit)
	for rows.Next() {
		var sample traffic.Sample
		var started, timestamp string
		var rate, errorRate, latency sql.NullFloat64
		if err := rows.Scan(&sample.Hostname, &started, &timestamp, &sample.RequestCount, &sample.ErrorCount, &sample.DurationSumMS, &rate, &errorRate, &latency); err != nil {
			return nil, fmt.Errorf("scan traffic sample: %w", err)
		}
		sample.SampledAt, err = time.Parse(time.RFC3339Nano, timestamp)
		if err != nil {
			return nil, fmt.Errorf("parse traffic time: %w", err)
		}
		sample.StartedAt, err = time.Parse(time.RFC3339Nano, started)
		if err != nil {
			return nil, fmt.Errorf("parse app start time: %w", err)
		}
		if rate.Valid {
			sample.RequestRate = &rate.Float64
		}
		if errorRate.Valid {
			sample.ErrorRate = &errorRate.Float64
		}
		if latency.Valid {
			sample.AvgLatencyMS = &latency.Float64
		}
		samples = append(samples, sample)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate traffic samples: %w", err)
	}
	return samples, nil
}
