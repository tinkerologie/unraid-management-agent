package alerting

import (
	"testing"
	"time"
)

func TestQueryHistory(t *testing.T) {
	e := NewEngine(NewStore(t.TempDir()), &mockDataProvider{})
	base := time.Unix(1_700_000_000, 0)
	for i := 0; i < 5; i++ {
		e.history.Record("cpu_temp", "", float64(40+i), base.Add(time.Duration(i)*time.Second))
	}
	r := e.QueryHistory("cpu_temp", "")
	if r.Count != 5 {
		t.Errorf("count=%d want 5", r.Count)
	}
	if r.Min != 40 || r.Max != 44 || r.Last != 44 {
		t.Errorf("min/max/last = %v/%v/%v", r.Min, r.Max, r.Last)
	}
	if r.Avg != 42 {
		t.Errorf("avg=%v want 42", r.Avg)
	}
	if r.Slope <= 0 {
		t.Errorf("slope=%v want >0", r.Slope)
	}
	// empty series
	if e.QueryHistory("nope", "").Count != 0 {
		t.Error("empty series should have count 0")
	}
}
