package dto

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestPSUSnapshotEmptyAndNullable(t *testing.T) {
	s := NewPSUStatus(PSUStatePending, time.Unix(0, 0))
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"devices":[]`) {
		t.Fatal("empty devices must be an array", string(data))
	}
	s.Devices = append(s.Devices, PSUDevice{Sensors: []PSUSensor{{Name: "input_power"}}})
	data, err = json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"value":null`) {
		t.Fatal("unknown sensor must stay null", string(data))
	}
}
