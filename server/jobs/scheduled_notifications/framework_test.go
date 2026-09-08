package scheduled_notifications

import (
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestUniquenessKey(t *testing.T) {
	t.Run("experience rows encode recipient + experience_id + purpose + offset", func(t *testing.T) {
		a := &models.ScheduledNotification{
			RecipientUserId: "u-1",
			Item: &models.ScheduledNotification_Experience{
				Experience: &models.ExperienceNotification{
					ExperienceId:            "e-1",
					Purpose:                 models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START,
					OffsetSecondsFromAnchor: -86400,
				},
			},
		}
		b := &models.ScheduledNotification{
			RecipientUserId: "u-1",
			Item: &models.ScheduledNotification_Experience{
				Experience: &models.ExperienceNotification{
					ExperienceId:            "e-1",
					Purpose:                 models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START,
					OffsetSecondsFromAnchor: -86400,
				},
			},
		}
		if UniquenessKey(a) != UniquenessKey(b) {
			t.Errorf("identical experience rows differ: %q vs %q", UniquenessKey(a), UniquenessKey(b))
		}
	})

	t.Run("different offsets are different keys", func(t *testing.T) {
		a := &models.ScheduledNotification{
			RecipientUserId: "u-1",
			Item: &models.ScheduledNotification_Experience{
				Experience: &models.ExperienceNotification{
					ExperienceId:            "e-1",
					Purpose:                 models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START,
					OffsetSecondsFromAnchor: -86400,
				},
			},
		}
		b := &models.ScheduledNotification{
			RecipientUserId: "u-1",
			Item: &models.ScheduledNotification_Experience{
				Experience: &models.ExperienceNotification{
					ExperienceId:            "e-1",
					Purpose:                 models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START,
					OffsetSecondsFromAnchor: -7200,
				},
			},
		}
		if UniquenessKey(a) == UniquenessKey(b) {
			t.Errorf("different offsets produced same key %q", UniquenessKey(a))
		}
	})

	t.Run("different purposes are different keys", func(t *testing.T) {
		a := &models.ScheduledNotification{
			RecipientUserId: "u-1",
			Item: &models.ScheduledNotification_Experience{
				Experience: &models.ExperienceNotification{
					ExperienceId:            "e-1",
					Purpose:                 models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START,
					OffsetSecondsFromAnchor: 0,
				},
			},
		}
		b := &models.ScheduledNotification{
			RecipientUserId: "u-1",
			Item: &models.ScheduledNotification_Experience{
				Experience: &models.ExperienceNotification{
					ExperienceId:            "e-1",
					Purpose:                 models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_CLOSE_PROMPT,
					OffsetSecondsFromAnchor: 0,
				},
			},
		}
		if UniquenessKey(a) == UniquenessKey(b) {
			t.Errorf("different purposes produced same key %q", UniquenessKey(a))
		}
	})

	t.Run("experience and loan rows are different keys even with matching ids and purpose ordinals", func(t *testing.T) {
		exp := &models.ScheduledNotification{
			RecipientUserId: "u-1",
			Item: &models.ScheduledNotification_Experience{
				Experience: &models.ExperienceNotification{
					ExperienceId:            "same-id",
					Purpose:                 models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START,
					OffsetSecondsFromAnchor: -86400,
				},
			},
		}
		loan := &models.ScheduledNotification{
			RecipientUserId: "u-1",
			Item: &models.ScheduledNotification_Loan{
				Loan: &models.LoanNotification{
					TransferId:              "same-id",
					Purpose:                 models.LoanNotificationPurpose_LOAN_NOTIFICATION_PURPOSE_RETURN_REMINDER,
					OffsetSecondsFromAnchor: -86400,
				},
			},
		}
		if UniquenessKey(exp) == UniquenessKey(loan) {
			t.Errorf("experience and loan collapsed to same key %q", UniquenessKey(exp))
		}
	})

	t.Run("request rows encode recipient + request_id + purpose + offset", func(t *testing.T) {
		a := &models.ScheduledNotification{
			RecipientUserId: "u-1",
			Item: &models.ScheduledNotification_Request{
				Request: &models.RequestNotification{
					RequestId:               "r-1",
					Purpose:                 models.RequestNotificationPurpose_REQUEST_NOTIFICATION_PURPOSE_FOLLOWUP_PROMPT,
					OffsetSecondsFromAnchor: 3 * 24 * 3600,
				},
			},
		}
		b := &models.ScheduledNotification{
			RecipientUserId: "u-1",
			Item: &models.ScheduledNotification_Request{
				Request: &models.RequestNotification{
					RequestId:               "r-1",
					Purpose:                 models.RequestNotificationPurpose_REQUEST_NOTIFICATION_PURPOSE_FOLLOWUP_PROMPT,
					OffsetSecondsFromAnchor: 3 * 24 * 3600,
				},
			},
		}
		if UniquenessKey(a) != UniquenessKey(b) {
			t.Errorf("identical request rows differ: %q vs %q", UniquenessKey(a), UniquenessKey(b))
		}
	})

	t.Run("request and loan rows are different keys even with matching ids and purpose ordinals", func(t *testing.T) {
		req := &models.ScheduledNotification{
			RecipientUserId: "u-1",
			Item: &models.ScheduledNotification_Request{
				Request: &models.RequestNotification{
					RequestId:               "same-id",
					Purpose:                 models.RequestNotificationPurpose_REQUEST_NOTIFICATION_PURPOSE_FOLLOWUP_PROMPT,
					OffsetSecondsFromAnchor: 259200,
				},
			},
		}
		loan := &models.ScheduledNotification{
			RecipientUserId: "u-1",
			Item: &models.ScheduledNotification_Loan{
				Loan: &models.LoanNotification{
					TransferId:              "same-id",
					Purpose:                 models.LoanNotificationPurpose_LOAN_NOTIFICATION_PURPOSE_RETURN_REMINDER,
					OffsetSecondsFromAnchor: 259200,
				},
			},
		}
		if UniquenessKey(req) == UniquenessKey(loan) {
			t.Errorf("request and loan collapsed to same key %q", UniquenessKey(req))
		}
	})

	t.Run("unset item oneof returns empty key", func(t *testing.T) {
		row := &models.ScheduledNotification{RecipientUserId: "u-1"}
		if got := UniquenessKey(row); got != "" {
			t.Errorf("unset item: got %q, want \"\"", got)
		}
	})

	t.Run("nil row returns empty key", func(t *testing.T) {
		if got := UniquenessKey(nil); got != "" {
			t.Errorf("nil row: got %q, want \"\"", got)
		}
	})
}
