package services

import (
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// ConvertTimeModelsToAPI converts a models.ExperienceTime to its api.ExperienceTime
// representation, copying all fields. Returns nil for nil input. Returns a TBD
// value for any unrecognized oneof variant.
func ConvertTimeModelsToAPI(modelsTime *models.ExperienceTime) *api.ExperienceTime {
	if modelsTime == nil {
		return nil
	}

	switch t := modelsTime.TimeType.(type) {
	case *models.ExperienceTime_Specific:
		return &api.ExperienceTime{
			TimeType: &api.ExperienceTime_Specific{
				Specific: &api.SpecificTime{
					UnixTimestampSec: t.Specific.UnixTimestampSec,
					Timezone:         t.Specific.Timezone,
					DurationMinutes:  t.Specific.DurationMinutes,
					IsAllDay:         t.Specific.IsAllDay,
				},
			},
		}
	case *models.ExperienceTime_Range:
		startUnixSec := t.Range.StartUnixSec
		endUnixSec := t.Range.EndUnixSec
		return &api.ExperienceTime{
			TimeType: &api.ExperienceTime_Range{
				Range: &api.TimeRange{
					Description:     t.Range.Description,
					StartUnixSec:    &startUnixSec,
					EndUnixSec:      &endUnixSec,
					Timezone:        t.Range.Timezone,
					DurationMinutes: t.Range.DurationMinutes,
					IsAllDay:        t.Range.IsAllDay,
				},
			},
		}
	case *models.ExperienceTime_Tbd:
		return &api.ExperienceTime{
			TimeType: &api.ExperienceTime_Tbd{
				Tbd: &api.TimeTBD{},
			},
		}
	default:
		return &api.ExperienceTime{
			TimeType: &api.ExperienceTime_Tbd{
				Tbd: &api.TimeTBD{},
			},
		}
	}
}
