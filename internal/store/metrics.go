package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"smartserver/internal/metrics"
)

type Metrics struct{ db *sql.DB }

func Open(path string) (*Metrics, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA busy_timeout = 5000; CREATE TABLE IF NOT EXISTS metric_samples (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		hostname TEXT NOT NULL,
		sampled_at TEXT NOT NULL,
		cpu_percent REAL NOT NULL,
		memory_percent REAL NOT NULL,
		disk_percent REAL NOT NULL,
		network_rx_bytes INTEGER NOT NULL,
		network_tx_bytes INTEGER NOT NULL,
		uptime_seconds INTEGER NOT NULL,
		temperature_c REAL
	); CREATE INDEX IF NOT EXISTS metric_samples_time ON metric_samples(sampled_at DESC);
	CREATE TABLE IF NOT EXISTS traffic_samples (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		hostname TEXT NOT NULL,
		started_at TEXT NOT NULL,
		sampled_at TEXT NOT NULL,
		request_count INTEGER NOT NULL,
		error_count INTEGER NOT NULL,
		duration_sum_ms REAL NOT NULL,
		request_rate_per_second REAL,
		error_rate_percent REAL,
		avg_latency_ms REAL
	); CREATE INDEX IF NOT EXISTS traffic_samples_time ON traffic_samples(sampled_at DESC);
	CREATE TABLE IF NOT EXISTS controller_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		occurred_at TEXT NOT NULL,
		kind TEXT NOT NULL,
		status TEXT NOT NULL,
		message TEXT NOT NULL
	); CREATE INDEX IF NOT EXISTS controller_events_time ON controller_events(occurred_at DESC);`); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize metrics table: %w", err)
	}
	if err := ensureTrafficP95Column(db); err != nil {
		db.Close()
		return nil, err
	}
	if err := migrateServers(db); err != nil {
		db.Close()
		return nil, err
	}
	return &Metrics{db: db}, nil
}

func (m *Metrics) Close() error { return m.db.Close() }

func (m *Metrics) Save(ctx context.Context, s metrics.Sample) error {
	_, err := m.db.ExecContext(ctx, `INSERT INTO metric_samples
		(server_id, hostname, sampled_at, cpu_percent, memory_percent, disk_percent, network_rx_bytes, network_tx_bytes, uptime_seconds, temperature_c)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, serverID(s.ServerID), s.Hostname, s.SampledAt.UTC().Format(time.RFC3339Nano), s.CPUPercent, s.MemoryPercent, s.DiskPercent, s.NetworkRXBytes, s.NetworkTXBytes, s.UptimeSeconds, s.TemperatureC)
	if err != nil {
		return fmt.Errorf("save metric sample: %w", err)
	}
	return nil
}

func (m *Metrics) Latest(ctx context.Context) (metrics.Sample, error) {
	return m.LatestFor(ctx, "primary")
}

func (m *Metrics) LatestFor(ctx context.Context, id string) (metrics.Sample, error) {
	return scanSample(m.db.QueryRowContext(ctx, `SELECT server_id, hostname, sampled_at, cpu_percent, memory_percent, disk_percent, network_rx_bytes, network_tx_bytes, uptime_seconds, temperature_c FROM metric_samples WHERE server_id = ? ORDER BY id DESC LIMIT 1`, serverID(id)))
}

func (m *Metrics) Recent(ctx context.Context, limit int) ([]metrics.Sample, error) {
	return m.RecentFor(ctx, "primary", limit)
}

func (m *Metrics) RecentFor(ctx context.Context, id string, limit int) ([]metrics.Sample, error) {
	rows, err := m.db.QueryContext(ctx, `SELECT server_id, hostname, sampled_at, cpu_percent, memory_percent, disk_percent, network_rx_bytes, network_tx_bytes, uptime_seconds, temperature_c FROM metric_samples WHERE server_id = ? ORDER BY id DESC LIMIT ?`, serverID(id), limit)
	if err != nil {
		return nil, fmt.Errorf("query metric samples: %w", err)
	}
	defer rows.Close()
	samples := make([]metrics.Sample, 0, limit)
	for rows.Next() {
		s, err := scanSample(rows)
		if err != nil {
			return nil, err
		}
		samples = append(samples, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate metric samples: %w", err)
	}
	return samples, nil
}

type scanner interface{ Scan(...any) error }

func scanSample(row scanner) (metrics.Sample, error) {
	var s metrics.Sample
	var timestamp string
	var temp sql.NullFloat64
	if err := row.Scan(&s.ServerID, &s.Hostname, &timestamp, &s.CPUPercent, &s.MemoryPercent, &s.DiskPercent, &s.NetworkRXBytes, &s.NetworkTXBytes, &s.UptimeSeconds, &temp); err != nil {
		return s, fmt.Errorf("scan metric sample: %w", err)
	}
	parsed, err := time.Parse(time.RFC3339Nano, timestamp)
	if err != nil {
		return s, fmt.Errorf("parse sample time: %w", err)
	}
	s.SampledAt = parsed
	if temp.Valid {
		s.TemperatureC = &temp.Float64
	}
	return s, nil
}
