package store

import (
	"context"
	"fmt"
	"time"
)

type Event struct {
	ID         int64     `json:"id"`
	OccurredAt time.Time `json:"occurred_at"`
	Kind       string    `json:"kind"`
	Status     string    `json:"status"`
	Message    string    `json:"message"`
}

func (m *Metrics) SaveEvent(ctx context.Context, event Event) error {
	_, err := m.db.ExecContext(ctx, `INSERT INTO controller_events (occurred_at, kind, status, message) VALUES (?, ?, ?, ?)`, event.OccurredAt.UTC().Format(time.RFC3339Nano), event.Kind, event.Status, event.Message)
	if err != nil {
		return fmt.Errorf("save controller event: %w", err)
	}
	return nil
}

func (m *Metrics) RecentEvents(ctx context.Context, limit int) ([]Event, error) {
	rows, err := m.db.QueryContext(ctx, `SELECT id, occurred_at, kind, status, message FROM controller_events ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("query controller events: %w", err)
	}
	defer rows.Close()
	events := make([]Event, 0, limit)
	for rows.Next() {
		var event Event
		var timestamp string
		if err := rows.Scan(&event.ID, &timestamp, &event.Kind, &event.Status, &event.Message); err != nil {
			return nil, fmt.Errorf("scan controller event: %w", err)
		}
		event.OccurredAt, err = time.Parse(time.RFC3339Nano, timestamp)
		if err != nil {
			return nil, fmt.Errorf("parse event time: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate controller events: %w", err)
	}
	return events, nil
}
