package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

func serverID(id string) string {
	if id == "" {
		return "primary"
	}
	return id
}

func migrateServers(db *sql.DB) error {
	for _, table := range []string{"metric_samples", "traffic_samples"} {
		rows, err := db.Query("PRAGMA table_info(" + table + ")")
		if err != nil {
			return fmt.Errorf("inspect %s: %w", table, err)
		}
		found := false
		for rows.Next() {
			var cid, nn, pk int
			var name, typ string
			var def sql.NullString
			if err := rows.Scan(&cid, &name, &typ, &nn, &def, &pk); err != nil {
				rows.Close()
				return err
			}
			found = found || name == "server_id"
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if !found {
			if _, err := db.Exec("ALTER TABLE " + table + " ADD COLUMN server_id TEXT NOT NULL DEFAULT 'primary'"); err != nil {
				return fmt.Errorf("migrate %s: %w", table, err)
			}
		}
		if _, err := db.Exec("CREATE INDEX IF NOT EXISTS " + table + "_server ON " + table + "(server_id, id DESC)"); err != nil {
			return err
		}
	}
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS control_state (id INTEGER PRIMARY KEY CHECK(id=1), payload TEXT NOT NULL)`)
	return err
}

// SaveControl records the state and its transition event atomically.
func (m *Metrics) SaveControl(ctx context.Context, value any, reason string) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT INTO control_state(id, payload) VALUES(1, ?) ON CONFLICT(id) DO UPDATE SET payload=excluded.payload`, string(body)); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO controller_events(occurred_at, kind, status, message) VALUES(?, 'scaling', 'transition', ?)`, time.Now().UTC().Format(time.RFC3339Nano), reason); err != nil {
		return err
	}
	return tx.Commit()
}

func (m *Metrics) LoadControl(ctx context.Context, value any) error {
	var body string
	if err := m.db.QueryRowContext(ctx, `SELECT payload FROM control_state WHERE id=1`).Scan(&body); err != nil {
		return err
	}
	return json.Unmarshal([]byte(body), value)
}
