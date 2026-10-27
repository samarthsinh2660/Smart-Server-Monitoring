package proxy

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

type Target struct {
	Enabled  bool   `json:"enabled"`
	Healthy  bool   `json:"healthy"`
	Sessions int    `json:"sessions"`
	Status   string `json:"status"`
}

type Runtime struct{ Socket string }

func (r Runtime) command(ctx context.Context, command string) (string, error) {
	conn, err := (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, "unix", r.Socket)
	if err != nil {
		return "", fmt.Errorf("HAProxy socket: %w", err)
	}
	defer conn.Close()
	deadline := time.Now().Add(2 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return "", err
	}
	if _, err := io.WriteString(conn, command+"\nquit\n"); err != nil {
		return "", err
	}
	body, err := io.ReadAll(io.LimitReader(conn, 1<<20))
	if err != nil {
		return "", fmt.Errorf("HAProxy response: %w", err)
	}
	if len(body) == 1<<20 {
		return "", fmt.Errorf("HAProxy response too large")
	}
	return string(body), nil
}

func (r Runtime) Set(ctx context.Context, id, state string) error {
	if id != "primary" && id != "standby" {
		return fmt.Errorf("unknown proxy target")
	}
	if state != "ready" && state != "maint" && state != "drain" {
		return fmt.Errorf("unknown proxy state")
	}
	body, err := r.command(ctx, "set server demo_servers/"+id+" state "+state)
	if err != nil {
		return err
	}
	if strings.TrimSpace(body) != "" {
		return fmt.Errorf("HAProxy rejected change: %s", strings.TrimSpace(body))
	}
	return nil
}

func ParseTargets(body string) (map[string]Target, error) {
	reader := csv.NewReader(strings.NewReader(body))
	reader.FieldsPerRecord = -1
	rows, err := reader.ReadAll()
	if err != nil || len(rows) < 2 {
		return nil, fmt.Errorf("invalid HAProxy stats response")
	}
	cols := make(map[string]int)
	for i, c := range rows[0] {
		cols[strings.TrimSpace(strings.TrimPrefix(c, "#"))] = i
	}
	for _, name := range []string{"pxname", "svname", "status", "scur"} {
		if _, ok := cols[name]; !ok {
			return nil, fmt.Errorf("missing HAProxy stats column %s", name)
		}
	}
	result := make(map[string]Target)
	for _, row := range rows[1:] {
		get := func(k string) string {
			if i := cols[k]; i < len(row) {
				return row[i]
			}
			return ""
		}
		id, status := get("svname"), get("status")
		if get("pxname") != "demo_servers" || (id != "primary" && id != "standby") {
			continue
		}
		sessions, err := strconv.Atoi(get("scur"))
		if err != nil || sessions < 0 || status == "" {
			return nil, fmt.Errorf("invalid HAProxy session count")
		}
		result[id] = Target{Enabled: !strings.HasPrefix(status, "MAINT"), Healthy: status == "UP", Sessions: sessions, Status: status}
	}
	if len(result) != 2 {
		return nil, fmt.Errorf("HAProxy must define demo_servers/primary and standby")
	}
	return result, nil
}

func (r Runtime) Targets(ctx context.Context) (map[string]Target, error) {
	body, err := r.command(ctx, "show stat")
	if err != nil {
		return nil, err
	}
	return ParseTargets(body)
}
