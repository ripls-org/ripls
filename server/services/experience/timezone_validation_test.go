package experience

import (
	"errors"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
)

func TestValidateExperienceTimeTimezone(t *testing.T) {
	specific := func(tz string) *api.ExperienceTime {
		return &api.ExperienceTime{
			TimeType: &api.ExperienceTime_Specific{Specific: &api.SpecificTime{Timezone: tz}},
		}
	}
	timeRange := func(tz string) *api.ExperienceTime {
		return &api.ExperienceTime{
			TimeType: &api.ExperienceTime_Range{Range: &api.TimeRange{Timezone: tz}},
		}
	}

	tests := []struct {
		name    string
		et      *api.ExperienceTime
		wantErr bool
	}{
		{"tbd", &api.ExperienceTime{TimeType: &api.ExperienceTime_Tbd{Tbd: &api.TimeTBD{}}}, false},
		{"specific empty", specific(""), false},
		{"specific valid", specific("America/Denver"), false},
		{"specific abbreviation rejected", specific("PST"), true},
		{"specific garbage rejected", specific("Not/AZone"), true},
		{"range empty", timeRange(""), false},
		{"range valid", timeRange("Europe/London"), false},
		{"range garbage rejected", timeRange("later-ish"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateExperienceTimeTimezone(tt.et)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil {
				return
			}
			var cerr *connect.Error
			if !errors.As(err, &cerr) || cerr.Code() != connect.CodeInvalidArgument {
				t.Errorf("error = %v, want connect.CodeInvalidArgument", err)
			}
		})
	}
}
