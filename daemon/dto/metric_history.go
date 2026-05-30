package dto

// MetricSample is a single timestamped metric reading returned by the history API.
type MetricSample struct {
	TimeUnix int64   `json:"time_unix"`
	Value    float64 `json:"value"`
}

// MetricHistoryResult is the response envelope for a metric history query.
// It includes the raw sample list and summary statistics computed over the window.
type MetricHistoryResult struct {
	Metric  string         `json:"metric"`
	Entity  string         `json:"entity,omitempty"`
	Samples []MetricSample `json:"samples"`
	Count   int            `json:"count"`
	Slope   float64        `json:"slope_per_sec"`
	Min     float64        `json:"min"`
	Max     float64        `json:"max"`
	Avg     float64        `json:"avg"`
	Last    float64        `json:"last"`
}
