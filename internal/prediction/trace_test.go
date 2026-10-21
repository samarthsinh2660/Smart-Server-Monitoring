package prediction

import (
	"bytes"
	"testing"
	"time"
)

func TestLabelTraceAndCSV(t *testing.T) {
	start := time.Now().UTC().Truncate(time.Second)
	var samples []TraceSample
	for i := 0; i <= 48; i++ {
		cpu := 30.0
		if i >= 12 {
			cpu = 85
		}
		samples = append(samples, TraceSample{
			SampledAt:  start.Add(time.Duration(i*5) * time.Second),
			CPUPercent: cpu,
			Features:   Features{CPUMeanPercent: cpu, MemoryPercent: 40},
		})
	}
	examples, err := LabelTrace(samples, "ramp-1", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(examples) != 37 {
		t.Fatalf("rows: got %d, want 37", len(examples))
	}
	if examples[12].Label != 1 || examples[0].Label != 0 || examples[24].Label != 0 {
		t.Fatalf("unexpected labels at 0, 12, 24: %v, %v, %v", examples[0].Label, examples[12].Label, examples[24].Label)
	}
	var output bytes.Buffer
	if err := WriteCSV(&output, examples, true); err != nil {
		t.Fatal(err)
	}
	parsed, err := ReadCSV(&output)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed) != len(examples) || parsed[12].Source != "recorded" || parsed[12].Label != 1 {
		t.Fatalf("CSV round trip: %+v", parsed)
	}
}

func TestLabelTraceDropsGaps(t *testing.T) {
	start := time.Now().UTC()
	var samples []TraceSample
	for i := 0; i < 50; i++ {
		if i >= 15 && i <= 19 {
			continue
		}
		samples = append(samples, TraceSample{SampledAt: start.Add(time.Duration(i*5) * time.Second), CPUPercent: 20})
	}
	examples, err := LabelTrace(samples, "steady-1", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	for _, example := range examples {
		if example.Label != 0 {
			t.Fatalf("unexpected overload label: %+v", example)
		}
	}
	if len(examples) >= len(samples)-12 {
		t.Fatalf("gap did not remove incomplete windows: %d rows", len(examples))
	}
}
