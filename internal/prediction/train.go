package prediction

import (
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"
)

type Example struct {
	Source   string
	RunID    string
	Features Features
	Label    float64
}

type Report struct {
	TrainingRows    int      `json:"training_rows"`
	HoldoutRows     int      `json:"holdout_rows"`
	HoldoutAccuracy float64  `json:"holdout_accuracy"`
	PositiveRecall  *float64 `json:"positive_recall,omitempty"`
	TruePositive    int      `json:"true_positive"`
	TrueNegative    int      `json:"true_negative"`
	FalsePositive   int      `json:"false_positive"`
	FalseNegative   int      `json:"false_negative"`
	HighRiskChecks  int      `json:"high_risk_checks_at_0_8"`
	HighRiskFalse   int      `json:"high_risk_false_at_0_8"`
}

// ReadCSV reads prelabelled feature rows. A run ID keeps adjacent samples together.
func ReadCSV(input io.Reader) ([]Example, error) {
	reader := csv.NewReader(input)
	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("read trace header: %w", err)
	}
	expected := append([]string{"source", "run_id"}, FeatureNames[:]...)
	expected = append(expected, "overload_soon")
	if strings.Join(header, ",") != strings.Join(expected, ",") {
		return nil, fmt.Errorf("trace header must be %s", strings.Join(expected, ","))
	}
	var examples []Example
	for line := 2; ; line++ {
		fields, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read trace line %d: %w", line, err)
		}
		if fields[0] != "recorded" && fields[0] != "synthetic" {
			return nil, fmt.Errorf("line %d: source must be recorded or synthetic", line)
		}
		if fields[1] == "" {
			return nil, fmt.Errorf("line %d: run_id is empty", line)
		}
		var values [8]float64
		for i := range values {
			values[i], err = strconv.ParseFloat(fields[i+2], 64)
			if err != nil || !finite(values[i]) {
				return nil, fmt.Errorf("line %d: invalid %s", line, FeatureNames[i])
			}
		}
		label, err := strconv.ParseFloat(fields[10], 64)
		if err != nil || (label != 0 && label != 1) {
			return nil, fmt.Errorf("line %d: overload_soon must be 0 or 1", line)
		}
		examples = append(examples, Example{Source: fields[0], RunID: fields[1], Features: Features{
			CPUMeanPercent: values[0], CPUTrendPerMinute: values[1], MemoryPercent: values[2],
			RequestRatePerSecond: values[3], RequestTrendPerMinute: values[4],
			P95LatencyMS: values[5], P95LatencyTrend: values[6], ErrorRatePercent: values[7],
		}, Label: label})
	}
	return examples, nil
}

// Train fits a deterministic logistic classifier and evaluates on a separate run.
func Train(examples []Example, holdoutRun, version string, horizonSeconds int) (Model, Report, error) {
	if version == "" || horizonSeconds < 1 || holdoutRun == "" {
		return Model{}, Report{}, fmt.Errorf("version, horizon, and holdout run are required")
	}
	var training, holdout []Example
	source := ""
	for _, example := range examples {
		if source == "" {
			source = example.Source
		}
		if example.Source != source {
			return Model{}, Report{}, fmt.Errorf("mixed trace sources")
		}
		if example.RunID == holdoutRun {
			holdout = append(holdout, example)
		} else {
			training = append(training, example)
		}
	}
	if len(training) < 12 || len(holdout) < 2 {
		return Model{}, Report{}, fmt.Errorf("need at least 12 training rows and 2 holdout rows from a separate run")
	}
	positives := 0
	for _, example := range training {
		if example.Label == 1 {
			positives++
		}
	}
	if positives < 2 || len(training)-positives < 2 {
		return Model{}, Report{}, fmt.Errorf("training requires positive and negative overload examples")
	}
	model := Model{Format: ModelFormat, Version: version, Source: source, TrainedAt: time.Now().UTC(), HorizonSeconds: horizonSeconds, FeatureNames: FeatureNames}
	for _, example := range training {
		for i, value := range example.Features.Values() {
			model.Means[i] += value / float64(len(training))
		}
	}
	for _, example := range training {
		for i, value := range example.Features.Values() {
			delta := value - model.Means[i]
			model.Scales[i] += delta * delta / float64(len(training))
		}
	}
	for i := range model.Scales {
		model.Scales[i] = math.Sqrt(model.Scales[i])
		if model.Scales[i] < 1e-6 {
			model.Scales[i] = 1
		}
	}
	const learningRate = 0.08
	const regularization = 0.001
	for epoch := 0; epoch < 2000; epoch++ {
		var weightGradient [8]float64
		biasGradient := 0.0
		for _, example := range training {
			probability, err := model.Predict(example.Features)
			if err != nil {
				return Model{}, Report{}, err
			}
			residual := probability - example.Label
			biasGradient += residual
			for i, value := range example.Features.Values() {
				weightGradient[i] += residual * (value - model.Means[i]) / model.Scales[i]
			}
		}
		model.Bias -= learningRate * biasGradient / float64(len(training))
		for i := range model.Weights {
			model.Weights[i] -= learningRate * (weightGradient[i]/float64(len(training)) + regularization*model.Weights[i])
		}
	}
	report := Report{TrainingRows: len(training), HoldoutRows: len(holdout)}
	correct, actualPositive, predictedPositive := 0, 0, 0
	for _, example := range holdout {
		probability, err := model.Predict(example.Features)
		if err != nil {
			return Model{}, Report{}, err
		}
		predicted := 0.0
		if probability >= 0.5 {
			predicted = 1
		}
		if predicted == example.Label {
			correct++
		}
		switch {
		case predicted == 1 && example.Label == 1:
			report.TruePositive++
		case predicted == 0 && example.Label == 0:
			report.TrueNegative++
		case predicted == 1:
			report.FalsePositive++
		default:
			report.FalseNegative++
		}
		if probability >= .8 {
			report.HighRiskChecks++
			if example.Label == 0 {
				report.HighRiskFalse++
			}
		}
		if example.Label == 1 {
			actualPositive++
			if predicted == 1 {
				predictedPositive++
			}
		}
	}
	report.HoldoutAccuracy = float64(correct) / float64(len(holdout))
	if actualPositive > 0 {
		recall := float64(predictedPositive) / float64(actualPositive)
		report.PositiveRecall = &recall
	}
	return model, report, nil
}
