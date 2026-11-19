package communityapps

import (
	"encoding/json"
	"io"
)

// DecodePlanArguments reads one bounded JSON object without exposing decoder
// diagnostics, which can include user-provided keys or environment values.
func DecodePlanArguments[T any](reader io.Reader) (T, error) {
	var args *T
	limited := &io.LimitedReader{R: reader, N: (128 << 10) + 1}
	decoder := json.NewDecoder(limited)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&args); err != nil || args == nil ||
		decoder.Decode(new(json.RawMessage)) != io.EOF || limited.N == 0 {
		var zero T
		return zero, ErrInvalidPlanRequest
	}
	return *args, nil
}
