package portfolio

import (
	"context"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
)

const (
	// defaultSocialTimeGoalMinutes is the system default weekly quality-time target.
	defaultSocialTimeGoalMinutes = 360

	// defaultMoneySavedGoalCents is the system default weekly cost-saved target ($500).
	defaultMoneySavedGoalCents = 50_000

	// defaultCO2AvoidedGoalGrams is the system default weekly CO₂-avoided target (30 kg).
	defaultCO2AvoidedGoalGrams = 30_000
)

// SetWeeklyGoals saves the user's personal weekly impact goals and returns the
// effective values after saving.
func (s *Service) SetWeeklyGoals(
	ctx context.Context,
	req *connect.Request[api.SetWeeklyGoalsRequest],
) (*connect.Response[api.SetWeeklyGoalsResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "SetWeeklyGoals",
		"user_id", authInfo.UserID,
	)

	userID := authInfo.UserID

	// Apply defaults for zero values.
	socialGoal := req.Msg.SocialTimeMinutesGoal
	if socialGoal <= 0 {
		socialGoal = defaultSocialTimeGoalMinutes
	}
	moneyGoal := req.Msg.MoneySavedCentsGoal
	if moneyGoal <= 0 {
		moneyGoal = defaultMoneySavedGoalCents
	}
	co2Goal := req.Msg.Co2AvoidedGramsGoal
	if co2Goal <= 0 {
		co2Goal = defaultCO2AvoidedGoalGrams
	}

	existing, err := s.storage.QueryByField(ctx, "user_id", userID, &models.UserWeeklyGoals{})
	if err != nil {
		return nil, connecterr.Internal(ctx, "SetWeeklyGoals", err, "detail", "failed to query existing goals")
	}

	now := time.Now().Unix()

	if len(existing) > 0 {
		g := existing[0].(*models.UserWeeklyGoals)
		g.SocialTimeMinutesGoal = socialGoal
		g.MoneySavedCentsGoal = moneyGoal
		g.Co2AvoidedGramsGoal = co2Goal
		g.UpdatedAtUnixSec = now
		if err := s.storage.Update(ctx, g); err != nil {
			return nil, connecterr.Internal(ctx, "SetWeeklyGoals", err, "detail", "failed to update weekly goals")
		}
		logger.InfoContext(ctx, "weekly goals updated")
	} else {
		g := &models.UserWeeklyGoals{
			Id:                    uuid.New().String(),
			UserId:                userID,
			SocialTimeMinutesGoal: socialGoal,
			MoneySavedCentsGoal:   moneyGoal,
			Co2AvoidedGramsGoal:   co2Goal,
			UpdatedAtUnixSec:      now,
		}
		if _, err := s.storage.Insert(ctx, g); err != nil {
			return nil, connecterr.Internal(ctx, "SetWeeklyGoals", err, "detail", "failed to insert weekly goals")
		}
		logger.InfoContext(ctx, "weekly goals created")
	}

	return connect.NewResponse(&api.SetWeeklyGoalsResponse{
		SocialTimeMinutesGoal: socialGoal,
		MoneySavedCentsGoal:   moneyGoal,
		Co2AvoidedGramsGoal:   co2Goal,
	}), nil
}
