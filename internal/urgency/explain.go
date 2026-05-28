package urgency

type ExplainResult struct {
	UUID  string        `json:"uuid"`
	Total float64       `json:"total"`
	Items []ExplainItem `json:"items"`
}

type ExplainItem struct {
	Name         string  `json:"name"`
	Coefficient  float64 `json:"coefficient"`
	Raw          any     `json:"raw,omitempty"`
	Contribution float64 `json:"contribution"`
	Reason       string  `json:"reason"`
}

type Options struct {
	NowUnix  int64
	Blocked  bool
	Blocking bool
}
