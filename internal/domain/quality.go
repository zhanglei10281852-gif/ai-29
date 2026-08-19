package domain

import (
	"fmt"
	"math"
)

// MilliScore stores scores without floating-point persistence drift.
type MilliScore int64

func ScoreFromFloat(value float64) (MilliScore, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, FieldError{Field: "score", Message: "must be finite"}
	}
	if value < -196 || value > 100 {
		return 0, FieldError{Field: "score", Message: "outside supported range"}
	}
	return MilliScore(math.Round(value * 1000)), nil
}

func (t MilliScore) Float64() float64 { return float64(t) / 1000 }

func (t MilliScore) String() string { return fmt.Sprintf("%.3f", t.Float64()) }

type QualityRange struct {
	Minimum MilliScore `json:"minimum"`
	Maximum MilliScore `json:"maximum"`
}

func NewQualityRange(minimum, maximum MilliScore) (QualityRange, error) {
	if minimum >= maximum {
		return QualityRange{}, FieldError{Field: "score_range", Message: "minimum must be lower than maximum"}
	}
	return QualityRange{Minimum: minimum, Maximum: maximum}, nil
}

func (r QualityRange) Contains(value MilliScore) bool {
	return value >= r.Minimum && value <= r.Maximum
}

func (r QualityRange) Validate() error {
	if r.Minimum >= r.Maximum {
		return FieldError{Field: "score_range", Message: "minimum must be lower than maximum"}
	}
	return nil
}
