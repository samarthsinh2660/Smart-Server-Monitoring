package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"smartserver/internal/prediction"
)

type featureResponse struct {
	Available  bool                 `json:"available"`
	Reason     string               `json:"reason"`
	SampledAt  time.Time            `json:"sampled_at"`
	CPUPercent *float64             `json:"cpu_percent"`
	Features   *prediction.Features `json:"features"`
}

func main() {
	controllerURL := flag.String("controller-url", "http://127.0.0.1:8080", "Go controller base URL")
	runID := flag.String("run-id", "", "unique workload run name")
	output := flag.String("output", "data/recorded-traces.csv", "combined training CSV")
	duration := flag.Duration("duration", 5*time.Minute, "capture duration")
	horizon := flag.Duration("horizon", 2*time.Minute, "future overload prediction window")
	flag.Parse()
	if *runID == "" || *duration <= *horizon || *horizon <= 0 {
		log.Fatal("run-id is required and duration must exceed a positive horizon")
	}
	client := &http.Client{Timeout: 3 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	deadline := time.NewTimer(*duration)
	defer deadline.Stop()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	var samples []prediction.TraceSample
	fetch := func() {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, *controllerURL+"/api/features", nil)
		if err != nil {
			log.Printf("request features: %v", err)
			return
		}
		response, err := client.Do(request)
		if err != nil {
			log.Printf("read features: %v", err)
			return
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			log.Printf("features returned HTTP %d", response.StatusCode)
			return
		}
		var result featureResponse
		if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
			log.Printf("decode features: %v", err)
			return
		}
		if !result.Available || result.Features == nil || result.CPUPercent == nil || result.SampledAt.IsZero() {
			log.Printf("features unavailable: %s", result.Reason)
			return
		}
		if len(samples) > 0 && !result.SampledAt.After(samples[len(samples)-1].SampledAt) {
			return
		}
		samples = append(samples, prediction.TraceSample{SampledAt: result.SampledAt, CPUPercent: *result.CPUPercent, Features: *result.Features})
	}
	fetch()
capture:
	for {
		select {
		case <-ticker.C:
			fetch()
		case <-deadline.C:
			break capture
		case <-ctx.Done():
			break capture
		}
	}
	examples, err := prediction.LabelTrace(samples, *runID, *horizon)
	if err != nil {
		log.Fatal(err)
	}
	if len(examples) == 0 {
		log.Fatal("no complete labelled windows; capture longer with continuous traffic")
	}
	if err := appendRun(*output, *runID, examples); err != nil {
		log.Fatal(err)
	}
	positives := 0
	for _, example := range examples {
		if example.Label == 1 {
			positives++
		}
	}
	fmt.Printf("saved %d recorded rows (%d positive, %d negative) from %d samples to %s\n", len(examples), positives, len(examples)-positives, len(samples), *output)
}

func appendRun(path, runID string, examples []prediction.Example) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create trace directory: %w", err)
	}
	_, err := os.Stat(path)
	newFile := os.IsNotExist(err)
	if err != nil && !newFile {
		return fmt.Errorf("stat trace CSV: %w", err)
	}
	if !newFile {
		input, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("open trace CSV: %w", err)
		}
		previous, readErr := prediction.ReadCSV(input)
		closeErr := input.Close()
		if readErr != nil {
			return readErr
		}
		if closeErr != nil {
			return closeErr
		}
		for _, example := range previous {
			if example.Source != "recorded" {
				return fmt.Errorf("cannot mix recorded and synthetic runs")
			}
			if example.RunID == runID {
				return fmt.Errorf("run ID %q already exists", runID)
			}
		}
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open trace output: %w", err)
	}
	if err := prediction.WriteCSV(file, examples, newFile); err != nil {
		file.Close()
		return fmt.Errorf("write trace CSV: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close trace CSV: %w", err)
	}
	return nil
}
