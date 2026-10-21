package controller

import (
	"context"
	"time"

	"smartserver/internal/prediction"
	"smartserver/internal/store"
)

type PredictionStatus struct {
	Available      bool                 `json:"available"`
	Reason         string               `json:"reason,omitempty"`
	RiskScore      *float64             `json:"risk_score,omitempty"`
	ModelVersion   string               `json:"model_version,omitempty"`
	Source         string               `json:"source,omitempty"`
	HorizonSeconds int                  `json:"horizon_seconds,omitempty"`
	ScoredAt       *time.Time           `json:"scored_at,omitempty"`
	Features       *prediction.Features `json:"features,omitempty"`
}

type FeatureStatus struct {
	Available  bool                 `json:"available"`
	Reason     string               `json:"reason,omitempty"`
	SampledAt  *time.Time           `json:"sampled_at,omitempty"`
	CPUPercent *float64             `json:"cpu_percent,omitempty"`
	Features   *prediction.Features `json:"features,omitempty"`
}

// ReadFeatures extracts current model inputs without requiring a trained model.
func ReadFeatures(ctx context.Context, db *store.Metrics, metricsAvailable, trafficAvailable bool) FeatureStatus {
	if !metricsAvailable || !trafficAvailable {
		return FeatureStatus{Reason: "recent metrics or traffic stats are unavailable"}
	}
	metrics, err := db.Recent(ctx, 60)
	if err != nil {
		return FeatureStatus{Reason: "could not read metrics history"}
	}
	traffic, err := db.RecentTraffic(ctx, 60)
	if err != nil {
		return FeatureStatus{Reason: "could not read traffic history"}
	}
	features, err := prediction.FromSamples(time.Now().UTC(), metrics, traffic)
	if err != nil {
		return FeatureStatus{Reason: err.Error()}
	}
	return FeatureStatus{Available: true, SampledAt: &metrics[0].SampledAt, CPUPercent: &metrics[0].CPUPercent, Features: &features}
}

// EvaluatePrediction reports a score only when the model and both inputs are fresh.
func EvaluatePrediction(ctx context.Context, db *store.Metrics, model *prediction.Model, modelError string, metricsAvailable, trafficAvailable bool) PredictionStatus {
	if model == nil {
		if modelError == "" {
			modelError = "model not configured"
		}
		return PredictionStatus{Reason: modelError}
	}
	base := PredictionStatus{ModelVersion: model.Version, Source: model.Source, HorizonSeconds: model.HorizonSeconds}
	input := ReadFeatures(ctx, db, metricsAvailable, trafficAvailable)
	if !input.Available {
		base.Reason = input.Reason
		return base
	}
	risk, err := model.Predict(*input.Features)
	if err != nil {
		base.Reason = "model evaluation failed"
		return base
	}
	now := time.Now().UTC()
	base.Available = true
	base.RiskScore = &risk
	base.ScoredAt = &now
	base.Features = input.Features
	return base
}
