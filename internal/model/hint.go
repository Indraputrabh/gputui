package model

// Hint describes a rule-based performance hypothesis with evidence.
type Hint struct {
	Name       string     `json:"name"`
	Category   string     `json:"category"`
	Severity   string     `json:"severity"`
	Confidence float64    `json:"confidence"`
	Summary    string     `json:"summary"`
	Evidence   []Evidence `json:"evidence"`
}

// Evidence is one metric/event supporting a hint.
type Evidence struct {
	Metric    string  `json:"metric"`
	Value     float64 `json:"value,omitempty"`
	Threshold float64 `json:"threshold,omitempty"`
	Unit      string  `json:"unit,omitempty"`
	Msg       string  `json:"msg,omitempty"`
}
