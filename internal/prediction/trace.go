package prediction

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"time"
)

// TraceSample is a measured feature vector and the simultaneous raw CPU reading.
type TraceSample struct {
	SampledAt  time.Time
	CPUPercent float64
	Features   Features
}

// LabelTrace uses future sustained CPU overload events as the training target.
// The final horizon and any window with missing samples are excluded.
func LabelTrace(samples []TraceSample, runID string, horizon time.Duration) ([]Example, error) {
	if runID == "" || horizon <= 0 || len(samples) < 2 {
		return nil, fmt.Errorf("run ID, positive horizon, and multiple samples are required")
	}
	for i, sample := range samples {
		if sample.SampledAt.IsZero() || !finite(sample.CPUPercent) || sample.CPUPercent < 0 || sample.CPUPercent > 100 {
			return nil, fmt.Errorf("invalid trace sample %d", i)
		}
		for _, value := range sample.Features.Values() {
			if !finite(value) {
				return nil, fmt.Errorf("invalid features in trace sample %d", i)
			}
		}
		if i > 0 && !sample.SampledAt.After(samples[i-1].SampledAt) {
			return nil, fmt.Errorf("trace samples must be in increasing time order")
		}
	}
	var events []time.Time
	var highSince time.Time
	active := false
	for i, sample := range samples {
		if i > 0 && sample.SampledAt.Sub(samples[i-1].SampledAt) > 15*time.Second {
			highSince = time.Time{}
			active = false
		}
		if sample.CPUPercent < 80 {
			highSince = time.Time{}
			active = false
			continue
		}
		if highSince.IsZero() {
			highSince = sample.SampledAt
		}
		if !active && sample.SampledAt.Sub(highSince) >= time.Minute {
			events = append(events, sample.SampledAt)
			active = true
		}
	}
	var examples []Example
	for i, sample := range samples {
		end := sample.SampledAt.Add(horizon)
		if end.After(samples[len(samples)-1].SampledAt) {
			break
		}
		complete := true
		for j := i + 1; j < len(samples); j++ {
			if samples[j].SampledAt.Sub(samples[j-1].SampledAt) > 15*time.Second {
				complete = false
				break
			}
			if !samples[j].SampledAt.Before(end) {
				break
			}
		}
		if !complete {
			continue
		}
		label := 0.0
		for _, event := range events {
			if event.After(sample.SampledAt) && !event.After(end) {
				label = 1
				break
			}
		}
		examples = append(examples, Example{Source: "recorded", RunID: runID, Features: sample.Features, Label: label})
	}
	return examples, nil
}

// WriteCSV writes examples in the same schema accepted by ReadCSV.
func WriteCSV(output io.Writer, examples []Example, header bool) error {
	writer := csv.NewWriter(output)
	if header {
		fields := append([]string{"source", "run_id"}, FeatureNames[:]...)
		if err := writer.Write(append(fields, "overload_soon")); err != nil {
			return err
		}
	}
	for _, example := range examples {
		fields := []string{example.Source, example.RunID}
		for _, value := range example.Features.Values() {
			fields = append(fields, strconv.FormatFloat(value, 'g', -1, 64))
		}
		fields = append(fields, strconv.FormatFloat(example.Label, 'f', 0, 64))
		if err := writer.Write(fields); err != nil {
			return err
		}
	}
	writer.Flush()
	return writer.Error()
}
