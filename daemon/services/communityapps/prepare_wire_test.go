package communityapps

import (
	"errors"
	"strings"
	"testing"
)

func TestDecodePlanArgumentsAcceptsOneBoundedObject(t *testing.T) {
	const sentinel = "private-argument-sentinel"
	valid := `{"name":"disposable","environment":{"TEST_KEY":"` + sentinel + `"}}`
	args, err := DecodePlanArguments[PrepareRequest](strings.NewReader(valid))
	if err != nil || args.Name != "disposable" || args.Environment["TEST_KEY"] != sentinel {
		t.Fatal("a valid plan request did not retain its declared value")
	}
	for _, test := range []struct {
		name  string
		input string
	}{
		{name: "null", input: "null"},
		{name: "array", input: "[]"},
		{name: "second object", input: valid + valid},
		{name: "over limit", input: strings.Repeat(" ", 128<<10) + valid},
		{name: "unknown field", input: `{"unexpected":"` + sentinel + `"}`},
		{name: "wrong field type", input: `{"environment":"` + sentinel + `"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := DecodePlanArguments[PrepareRequest](strings.NewReader(test.input))
			if !errors.Is(err, ErrInvalidPlanRequest) || strings.Contains(err.Error(), sentinel) {
				t.Fatal("invalid input did not receive a safe parse error")
			}
		})
	}
}
