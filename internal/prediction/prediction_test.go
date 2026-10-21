package prediction

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"smartserver/internal/metrics"
	"smartserver/internal/traffic"
)

func TestTrainAndLoadSyntheticModel(t *testing.T) {
	file, err := os.Open("../../examples/overload-training.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	examples, err := ReadCSV(file)
	if err != nil {
		t.Fatal(err)
	}
	model, report, err := Train(examples, "burst-b", "demo-v1", 120)
	if err != nil {
		t.Fatal(err)
	}
	if model.Source != "synthetic" || report.TrainingRows != 20 || report.HoldoutRows != 8 || report.HoldoutAccuracy < .75 {
		t.Fatalf("unexpected training report: model=%+v report=%+v", model, report)
	}
	if report.TruePositive+report.TrueNegative+report.FalsePositive+report.FalseNegative != report.HoldoutRows || report.HighRiskFalse > report.HighRiskChecks {
		t.Fatalf("inconsistent holdout counts: %+v", report)
	}
	path := filepath.Join(t.TempDir(), "model.json")
	if err := model.Save(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	oldFormat := loaded
	oldFormat.Format = 1
	if err := oldFormat.Validate(); err == nil {
		t.Fatal("obsolete model format accepted")
	}
	before, err := model.Predict(examples[0].Features)
	if err != nil {
		t.Fatal(err)
	}
	after, err := loaded.Predict(examples[0].Features)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(before-after) > 1e-12 {
		t.Fatalf("round trip changed score: %v != %v", before, after)
	}
}

func TestFromSamples(t *testing.T) {
	now := time.Now().UTC()
	var metricSamples []metrics.Sample
	var trafficSamples []traffic.Sample
	for i := 0; i < 6; i++ {
		at := now.Add(-time.Duration(i*5) * time.Second)
		metricSamples = append(metricSamples, metrics.Sample{SampledAt: at, CPUPercent: float64(60 - i*5), MemoryPercent: 45})
		rate, errors, latency := float64(5-i), 0.0, float64(200-i*20)
		trafficSamples = append(trafficSamples, traffic.Sample{Snapshot: traffic.Snapshot{SampledAt: at}, RequestRate: &rate, ErrorRate: &errors, P95LatencyMS: &latency})
	}
	features, err := FromSamples(now, metricSamples, trafficSamples)
	if err != nil {
		t.Fatal(err)
	}
	if features.CPUTrendPerMinute < 50 || features.RequestTrendPerMinute < 10 || features.P95LatencyTrend < 100 || features.MemoryPercent != 45 {
		t.Fatalf("unexpected features: %+v", features)
	}
	if _, err := FromSamples(now.Add(time.Minute), metricSamples, trafficSamples); err == nil {
		t.Fatal("stale samples should not produce a score")
	}
	trafficSamples[3].StartedAt = now.Add(-time.Minute)
	if _, err := FromSamples(now, metricSamples, trafficSamples); err == nil {
		t.Fatal("samples from a previous app start should not produce a score")
	}
	trafficSamples[3].StartedAt = time.Time{}
	trafficSamples[0].P95LatencyMS = nil
	for i := 1; i < len(trafficSamples); i++ {
		trafficSamples[i].P95LatencyMS = nil
	}
	if _, err := FromSamples(now, metricSamples, trafficSamples); err == nil {
		t.Fatal("missing latency should not produce a score")
	}
}
