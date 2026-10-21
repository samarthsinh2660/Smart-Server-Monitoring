package prediction

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"time"
)

const ModelFormat = 2

// Model is a small, versioned logistic regression model with fixed input order.
type Model struct {
	Format         int        `json:"format"`
	Version        string     `json:"version"`
	Source         string     `json:"source"`
	TrainedAt      time.Time  `json:"trained_at"`
	HorizonSeconds int        `json:"horizon_seconds"`
	FeatureNames   [8]string  `json:"feature_names"`
	Means          [8]float64 `json:"means"`
	Scales         [8]float64 `json:"scales"`
	Weights        [8]float64 `json:"weights"`
	Bias           float64    `json:"bias"`
}

func (m Model) Validate() error {
	if m.Format != ModelFormat || m.Version == "" || m.HorizonSeconds < 1 || m.FeatureNames != FeatureNames || m.TrainedAt.IsZero() {
		return fmt.Errorf("invalid model metadata")
	}
	if m.Source != "recorded" && m.Source != "synthetic" {
		return fmt.Errorf("invalid model source")
	}
	if !finite(m.Bias) {
		return fmt.Errorf("invalid model bias")
	}
	for i := range m.Weights {
		if !finite(m.Means[i]) || !finite(m.Scales[i]) || m.Scales[i] <= 0 || !finite(m.Weights[i]) {
			return fmt.Errorf("invalid model parameter %d", i)
		}
	}
	return nil
}

func (m Model) Predict(features Features) (float64, error) {
	if err := m.Validate(); err != nil {
		return 0, err
	}
	values := features.Values()
	z := m.Bias
	for i, value := range values {
		if !finite(value) {
			return 0, fmt.Errorf("non-finite feature %s", FeatureNames[i])
		}
		z += m.Weights[i] * (value - m.Means[i]) / m.Scales[i]
	}
	if z >= 0 {
		return 1 / (1 + math.Exp(-z)), nil
	}
	e := math.Exp(z)
	return e / (1 + e), nil
}

func Load(path string) (Model, error) {
	file, err := os.Open(path)
	if err != nil {
		return Model{}, fmt.Errorf("open model: %w", err)
	}
	defer file.Close()
	var model Model
	if err := json.NewDecoder(file).Decode(&model); err != nil {
		return Model{}, fmt.Errorf("decode model: %w", err)
	}
	if err := model.Validate(); err != nil {
		return Model{}, err
	}
	return model, nil
}

func (m Model) Save(path string) error {
	if err := m.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("encode model: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0644); err != nil {
		return fmt.Errorf("write model: %w", err)
	}
	return nil
}
