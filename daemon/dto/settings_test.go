package dto_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
)

func TestParitySchedule_JSONSerialization(t *testing.T) {
	tests := []struct {
		name           string
		schedule       dto.ParitySchedule
		mustContain    []string
		mustNotContain []string
	}{
		{
			name: "Custom W2 emits week_of_month and unconditional day_of_month",
			schedule: dto.ParitySchedule{
				Mode:        "custom",
				DayOfMonth:  0,
				WeekOfMonth: "W2",
			},
			mustContain: []string{
				`"mode":"custom"`,
				`"day_of_month":0`,
				`"week_of_month":"W2"`,
			},
			mustNotContain: []string{
				`"last_day_of_month"`,
			},
		},
		{
			name: "Custom wildcard emits week_of_month",
			schedule: dto.ParitySchedule{
				Mode:        "custom",
				DayOfMonth:  0,
				WeekOfMonth: "*",
			},
			mustContain: []string{
				`"mode":"custom"`,
				`"day_of_month":0`,
				`"week_of_month":"*"`,
			},
			mustNotContain: []string{
				`"last_day_of_month"`,
			},
		},
		{
			name: "Empty week_of_month is omitted",
			schedule: dto.ParitySchedule{
				Mode:       "monthly",
				DayOfMonth: 15,
			},
			mustContain: []string{
				`"mode":"monthly"`,
				`"day_of_month":15`,
			},
			mustNotContain: []string{
				`"week_of_month"`,
				`"last_day_of_month"`,
			},
		},
		{
			name: "LastDayOfMonth true is emitted",
			schedule: dto.ParitySchedule{
				Mode:           "monthly",
				DayOfMonth:     0,
				LastDayOfMonth: true,
			},
			mustContain: []string{
				`"mode":"monthly"`,
				`"day_of_month":0`,
				`"last_day_of_month":true`,
			},
			mustNotContain: []string{
				`"week_of_month"`,
			},
		},
		{
			name: "LastDayOfMonth false is omitted",
			schedule: dto.ParitySchedule{
				Mode:           "monthly",
				DayOfMonth:     1,
				LastDayOfMonth: false,
			},
			mustContain: []string{
				`"mode":"monthly"`,
				`"day_of_month":1`,
			},
			mustNotContain: []string{
				`"last_day_of_month"`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.schedule)
			if err != nil {
				t.Fatalf("json.Marshal failed: %v", err)
			}
			jsonStr := string(data)
			for _, s := range tt.mustContain {
				if !strings.Contains(jsonStr, s) {
					t.Errorf("JSON %s does not contain %q", jsonStr, s)
				}
			}
			for _, s := range tt.mustNotContain {
				if strings.Contains(jsonStr, s) {
					t.Errorf("JSON %s unexpectedly contains %q", jsonStr, s)
				}
			}
		})
	}
}
