package controller

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"smartserver/internal/metrics"
	"smartserver/internal/prediction"
	"smartserver/internal/store"
	"smartserver/internal/traffic"
)

func TestEvaluatePrediction(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "monitor.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if got := EvaluatePrediction(ctx, db, nil, "", true, true); got.Available || got.Reason == "" {
		t.Fatalf("missing model result: %+v", got)
	}
	model := prediction.Model{
		Format: prediction.ModelFormat, Version: "test-v1", Source: "synthetic",
		TrainedAt: time.Now().UTC(), HorizonSeconds: 120, FeatureNames: prediction.FeatureNames,
		Scales: [8]float64{1, 1, 1, 1, 1, 1, 1, 1},
	}
	if got := EvaluatePrediction(ctx, db, &model, "", false, true); got.Available || got.Reason == "" {
		t.Fatalf("missing metrics result: %+v", got)
	}
	now := time.Now().UTC()
	for i := 5; i >= 0; i-- {
		at := now.Add(-time.Duration(i*5) * time.Second)
		if err := db.Save(ctx, metrics.Sample{Hostname: "laptop", SampledAt: at, CPUPercent: 35, MemoryPercent: 40}); err != nil {
			t.Fatal(err)
		}
		rate, errorRate, latency := 2.0, 0.0, 100.0
		sample := traffic.Sample{
			Snapshot:    traffic.Snapshot{Hostname: "laptop", StartedAt: now.Add(-time.Hour), SampledAt: at},
			RequestRate: &rate, ErrorRate: &errorRate, P95LatencyMS: &latency,
		}
		if err := db.SaveTraffic(ctx, sample); err != nil {
			t.Fatal(err)
		}
	}
	got := EvaluatePrediction(ctx, db, &model, "", true, true)
	if !got.Available || got.RiskScore == nil || *got.RiskScore != .5 || got.ModelVersion != "test-v1" {
		t.Fatalf("available prediction: %+v", got)
	}
	features := ReadFeatures(ctx, db, true, true)
	if !features.Available || features.Features == nil || features.CPUPercent == nil || *features.CPUPercent != 35 {
		t.Fatalf("features without a model: %+v", features)
	}
}
