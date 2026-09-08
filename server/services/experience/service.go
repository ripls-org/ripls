package experience

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"time"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/ai"
	"go.ripls.org/ripls/server/branding"
	"go.ripls.org/ripls/server/chat"
	cebus "go.ripls.org/ripls/server/community_event_bus"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/api/apiconnect"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/impact_metrics"
	"go.ripls.org/ripls/server/impact_metrics/estimator"
	"go.ripls.org/ripls/server/location"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/media"
	"go.ripls.org/ripls/server/notifications"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/storage"
	"go.ripls.org/ripls/server/timezone"
	"go.ripls.org/ripls/server/weather"
	"go.ripls.org/ripls/server/webfetch"
)

// StockImageryProvider defines the interface for stock imagery operations.
// This is a copy of media.StockImageryProvider to avoid import cycles.
type StockImageryProvider interface {
	GetStockImage(ctx context.Context, query string, opts *media.StockImageOptions) (*models.StockImage, error)
	SearchStockImageCandidates(ctx context.Context, query string, limit int) ([]media.StockImageCandidate, error)
}

// StockVideoProvider defines the interface for stock video operations.
// This is a copy of media.StockVideoProvider to avoid import cycles.
type StockVideoProvider interface {
	GetStockVideo(ctx context.Context, query string) (*models.StockImage, error)
	SearchStockVideoCandidates(ctx context.Context, query string, limit int) ([]media.StockImageCandidate, error)
}

// Service implements the ExperienceService RPC interface.
type Service struct {
	storage              *storage.ProtoSQLStorage
	bucket               storage.BucketStorage
	notificationService  notifications.Service
	bus                  cebus.Publisher                           // CommunityEvent publisher; required (#510 PR 3).
	systemMessageWriter  *chat.SystemMessageWriter                 // Optional: can be nil
	aiProvider           ai.Provider                               // Optional: can be nil
	stockImageryProvider StockImageryProvider                      // Optional: can be nil
	stockVideoProvider   StockVideoProvider                        // Optional: can be nil
	locationProvider     location.Provider                         // Optional: can be nil
	webFetcher           webfetch.Fetcher                          // Optional: can be nil
	estimatorCfg         *estimator.Config                         // Optional: can be nil (enables full impact estimation)
	socialResolver       *impact_metrics.ConnectionContextResolver // Optional: can be nil (enables SF estimation)
	weatherProvider      weather.Provider                          // Optional: can be nil (enables the event-day forecast)

	// imageDownloadClient builds the *http.Client used for og:image downloads
	// in downloadAndStoreWebpageImage. Defaults to safehttp.NewClient
	// (SSRF-guarded). Tests inject a plain client to allow loopback servers.
	imageDownloadClient func(timeout time.Duration) *http.Client

	// branding names this instance in the User-Agent of og:image downloads, so
	// the operator a site owner sees is the one actually making the request.
	// Zero value is fine — BotUserAgent falls back to a generic agent.
	branding branding.Config
}

// SetEstimatorConfig sets the impact estimator config for this service (optional).
// When set, enables full three-dimensional impact estimation on experience completion.
func (s *Service) SetEstimatorConfig(cfg *estimator.Config) {
	s.estimatorCfg = cfg
}

// SetSocialResolver sets the social connection context resolver (optional).
// When set, enables social footprint estimation on experience completion.
func (s *Service) SetSocialResolver(r *impact_metrics.ConnectionContextResolver) {
	s.socialResolver = r
}

// resolveConnectionContext resolves the social connection context between two users.
// Returns nil if no resolver is set or if resolution fails (logs warning on failure).
func (s *Service) resolveConnectionContext(ctx context.Context, userA, userB, communityID string, logger *logging.Logger) *api.ConnectionContext {
	if s.socialResolver == nil || userA == "" || userB == "" {
		return nil
	}
	connCtx, err := s.socialResolver.Resolve(ctx, userA, userB, communityID)
	if err != nil {
		logger.WarnContext(ctx, "failed to resolve connection context for SF estimation",
			"user_a", userA, "user_b", userB, "error", err)
		return nil
	}
	return connCtx
}

// New creates a new experience service instance. bus is required — emit
// sites publish via it (#510 PR 3).
func New(storage *storage.ProtoSQLStorage, bucketStorage storage.BucketStorage, notificationService notifications.Service, bus cebus.Publisher) *Service {
	return &Service{
		storage:             storage,
		bucket:              bucketStorage,
		notificationService: notificationService,
		bus:                 bus,
	}
}

// SetSystemMessageWriter sets the system message writer for this service (optional).
func (s *Service) SetSystemMessageWriter(writer *chat.SystemMessageWriter) {
	s.systemMessageWriter = writer
}

// SetAIProvider sets the AI provider for this service (optional).
func (s *Service) SetAIProvider(provider ai.Provider) {
	s.aiProvider = provider
}

// SetStockImageryProvider sets the stock imagery provider for this service (optional).
func (s *Service) SetStockImageryProvider(provider StockImageryProvider) {
	s.stockImageryProvider = provider
}

// SetStockVideoProvider sets the stock video provider for this service (optional).
// When set, text-generated experiences will prefer video over static images.
func (s *Service) SetStockVideoProvider(provider StockVideoProvider) {
	s.stockVideoProvider = provider
}

// SetLocationProvider sets the geocoding/place-search provider (optional).
func (s *Service) SetLocationProvider(provider location.Provider) {
	s.locationProvider = provider
}

// SetWebFetcher sets the web fetcher for this service (optional).
// SetBranding supplies the instance identity used in the outbound User-Agent.
func (s *Service) SetBranding(b branding.Config) {
	s.branding = b
}

func (s *Service) SetWebFetcher(fetcher webfetch.Fetcher) {
	s.webFetcher = fetcher
}

// SetWeatherProvider sets the weather provider used to surface the event-day
// forecast on GetExperience (optional). When unset, the response carries no
// forecast and the client renders no weather chip. See docs/weather.md.
func (s *Service) SetWeatherProvider(provider weather.Provider) {
	s.weatherProvider = provider
}

// convertExperienceState converts from storage model ExperienceState to API ExperienceState.
func convertExperienceState(state models.ExperienceState) api.ExperienceState {
	switch state {
	case models.ExperienceState_EXPERIENCE_STATE_ACTIVE:
		return api.ExperienceState_EXPERIENCE_STATE_ACTIVE
	case models.ExperienceState_EXPERIENCE_STATE_JOINED:
		return api.ExperienceState_EXPERIENCE_STATE_JOINED
	case models.ExperienceState_EXPERIENCE_STATE_IN_PROCESS:
		return api.ExperienceState_EXPERIENCE_STATE_IN_PROCESS
	case models.ExperienceState_EXPERIENCE_STATE_COMPLETED:
		return api.ExperienceState_EXPERIENCE_STATE_COMPLETED
	case models.ExperienceState_EXPERIENCE_STATE_CANCELLED:
		return api.ExperienceState_EXPERIENCE_STATE_CANCELLED
	default:
		return api.ExperienceState_EXPERIENCE_STATE_UNSPECIFIED
	}
}

// buildAPIExperience builds an API Experience from a stored Experience model.
// If communityID is provided, RSVP counts are scoped to that community and
// created_at reflects when the experience was shared with that community.
// If communityID is empty, cross-community RSVP dedup is used and created_at
// is taken from the first CommunityExperience row.
//
// It delegates to buildAPIExperiences for all batch fetching, then applies
// community-scoped post-filtering when communityID is non-empty.
func (s *Service) buildAPIExperience(ctx context.Context, exp *models.Experience, communityID string) (*api.Experience, error) {
	results, err := s.buildAPIExperiences(ctx, []*models.Experience{exp})
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, connecterr.Internal(ctx, "buildAPIExperience", fmt.Errorf("buildAPIExperiences returned no result for %s", exp.Id))
	}
	result := results[0]

	if communityID == "" {
		return result, nil
	}

	// Apply community-scoped overrides: community-filtered RSVP counts and
	// the shared_at timestamp for this specific community.
	result.CommunityId = communityID

	// Re-query RSVPs scoped to this community for accurate per-community counts.
	logger := logging.LoggerWithContext(ctx).With("experience_id", exp.Id, "community_id", communityID)
	communityRSVPs, rsvpErr := storage.QueryByFields[*models.ExperienceRSVP](s.storage, ctx, map[string]any{
		"experience_id": exp.Id,
		"community_id":  communityID,
	})
	if rsvpErr != nil {
		logger.Warn("failed to query community-scoped RSVPs", "error", rsvpErr)
	} else {
		byUser := make(map[string]*models.ExperienceRSVP, len(communityRSVPs))
		for _, rsvp := range communityRSVPs {
			if existing, ok := byUser[rsvp.UserId]; !ok || rsvp.LastUpdatedUnixSec > existing.LastUpdatedUnixSec {
				byUser[rsvp.UserId] = rsvp
			}
		}
		var yes, maybe int32
		for _, rsvp := range byUser {
			switch rsvp.GetIntention() {
			case models.RSVPIntention_RSVP_INTENTION_YES:
				yes++
			case models.RSVPIntention_RSVP_INTENTION_MAYBE:
				maybe++
			}
		}
		result.RsvpYesCount = yes
		result.RsvpMaybeCount = maybe
	}

	// Override created_at to use the timestamp from the specific community.
	allCEs, ceErr := s.storage.QueryByField(ctx, "experience_id", exp.Id, &models.CommunityExperience{})
	if ceErr != nil {
		logger.Warn("failed to query community experiences for created_at", "error", ceErr)
	} else {
		for _, m := range allCEs {
			ce := m.(*models.CommunityExperience)
			if ce.CommunityId == communityID {
				result.CreatedAtUnixSec = ce.SharedAtUnixSec
				break
			}
		}
	}

	return result, nil
}

// buildRSVPs builds the RSVP list for an experience.
//
// Returns the deduplicated set of RSVPs across every community the
// experience has been shared with — one entry per user, using the
// most-recently-updated row when a user has entries in multiple
// communities. The list is intentionally NOT community-scoped: access to
// the experience is gated at the read layer (the caller must be a member
// of some shared community), but once a viewer can see the event they see
// every attendee. This matches user-facing intent — "who's going" is a
// property of the event, not of the community lens.
func (s *Service) buildRSVPs(ctx context.Context, experienceID string) ([]*api.RSVP, error) {
	logger := logging.LoggerWithContext(ctx).With("experience_id", experienceID)

	all, err := storage.QueryByField[*models.ExperienceRSVP](s.storage, ctx, "experience_id", experienceID)
	if err != nil {
		logger.Error("failed to query RSVPs", "error", err)
		return nil, connecterr.Internal(ctx, "buildRSVPs", err)
	}

	// Deduplicate by userId: keep the most recently updated RSVP per user.
	byUser := make(map[string]*models.ExperienceRSVP, len(all))
	for _, rsvp := range all {
		if existing, ok := byUser[rsvp.UserId]; !ok || rsvp.LastUpdatedUnixSec > existing.LastUpdatedUnixSec {
			byUser[rsvp.UserId] = rsvp
		}
	}

	// Collect and sort by original RSVP time for stable ordering.
	deduped := make([]*models.ExperienceRSVP, 0, len(byUser))
	for _, rsvp := range byUser {
		deduped = append(deduped, rsvp)
	}
	sort.Slice(deduped, func(i, j int) bool {
		return deduped[i].RsvpedAtUnixSec < deduped[j].RsvpedAtUnixSec
	})

	// Batch fetch user details.
	userIDs := make([]string, 0, len(deduped))
	for _, rsvp := range deduped {
		userIDs = append(userIDs, rsvp.UserId)
	}
	userMap, err := services.FetchAPIUsersBatch(ctx, s.storage, userIDs)
	if err != nil {
		logger.Error("failed to batch fetch RSVP users", "error", err)
		return nil, connecterr.Internal(ctx, "buildRSVPs", err)
	}

	// Build RSVP list with user details.
	rsvps := make([]*api.RSVP, 0, len(deduped))
	for _, rsvp := range deduped {
		user := userMap[rsvp.UserId]
		if user == nil {
			logger.Warn("user not found for RSVP", "user_id", rsvp.UserId)
			continue
		}

		// Convert the storage intention enum to its API twin
		intention := api.RSVPIntention_RSVP_INTENTION_UNSPECIFIED
		switch rsvp.GetIntention() {
		case models.RSVPIntention_RSVP_INTENTION_YES:
			intention = api.RSVPIntention_RSVP_INTENTION_YES
		case models.RSVPIntention_RSVP_INTENTION_MAYBE:
			intention = api.RSVPIntention_RSVP_INTENTION_MAYBE
		case models.RSVPIntention_RSVP_INTENTION_NO:
			intention = api.RSVPIntention_RSVP_INTENTION_NO
		}

		// Convert the storage attended enum to its API twin
		attended := api.AttendedStatus_ATTENDED_STATUS_UNSPECIFIED
		switch rsvp.GetAttended() {
		case models.AttendedStatus_ATTENDED_STATUS_UNKNOWN:
			attended = api.AttendedStatus_ATTENDED_STATUS_UNKNOWN
		case models.AttendedStatus_ATTENDED_STATUS_YES:
			attended = api.AttendedStatus_ATTENDED_STATUS_YES
		case models.AttendedStatus_ATTENDED_STATUS_NO:
			attended = api.AttendedStatus_ATTENDED_STATUS_NO
		}

		rsvps = append(rsvps, &api.RSVP{
			User:               user,
			Intention:          intention,
			Attended:           attended,
			RsvpedAtUnixSec:    rsvp.RsvpedAtUnixSec,
			LastUpdatedUnixSec: rsvp.LastUpdatedUnixSec,
		})
	}

	return rsvps, nil
}

// getUserDisplayName fetches a user's display name for system chat messages.
// Returns "" when the user is unknown or unnamed: the name travels as a
// template param, so the stand-in has to be the client's localized one rather
// than an English word spliced into a translated sentence (#2844). The English
// `description` fallback substitutes its own via chat.actorOrSomeone.
func (s *Service) getUserDisplayName(ctx context.Context, userID string) string {
	logger := logging.LoggerWithContext(ctx).With("user_id", userID)

	user := &models.User{}
	if err := s.storage.GetByID(ctx, userID, user); err != nil {
		logger.Warn("failed to get user for display name", "error", err)
		return ""
	}
	return user.Name
}

// validateExperienceTimeTimezone rejects a wire ExperienceTime whose timezone
// is not a valid IANA name. Empty is allowed (timezone unknown); an invalid
// string would poison every server-rendered display of the event's time, so
// fail fast at the RPC boundary instead of storing it.
func validateExperienceTimeTimezone(apiTime *api.ExperienceTime) error {
	var tz string
	switch t := apiTime.GetTimeType().(type) {
	case *api.ExperienceTime_Specific:
		tz = t.Specific.GetTimezone()
	case *api.ExperienceTime_Range:
		tz = t.Range.GetTimezone()
	default:
		return nil
	}
	if !timezone.IsValidOrEmpty(tz) {
		return connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("invalid IANA timezone %q", tz))
	}
	return nil
}

// convertTimeAPIToModels converts API ExperienceTime to Models ExperienceTime.
func convertTimeAPIToModels(apiTime *api.ExperienceTime) *models.ExperienceTime {
	if apiTime == nil {
		return nil
	}

	switch t := apiTime.TimeType.(type) {
	case *api.ExperienceTime_Specific:
		return &models.ExperienceTime{
			TimeType: &models.ExperienceTime_Specific{
				Specific: &models.SpecificTime{
					UnixTimestampSec: t.Specific.UnixTimestampSec,
					Timezone:         t.Specific.Timezone,
					DurationMinutes:  t.Specific.DurationMinutes,
					IsAllDay:         t.Specific.IsAllDay,
				},
			},
		}
	case *api.ExperienceTime_Range:
		var startUnixSec, endUnixSec int64
		if t.Range.StartUnixSec != nil {
			startUnixSec = *t.Range.StartUnixSec
		}
		if t.Range.EndUnixSec != nil {
			endUnixSec = *t.Range.EndUnixSec
		}
		return &models.ExperienceTime{
			TimeType: &models.ExperienceTime_Range{
				Range: &models.TimeRange{
					Description:     t.Range.Description,
					StartUnixSec:    startUnixSec,
					EndUnixSec:      endUnixSec,
					Timezone:        t.Range.Timezone,
					DurationMinutes: t.Range.DurationMinutes,
					IsAllDay:        t.Range.IsAllDay,
				},
			},
		}
	case *api.ExperienceTime_Tbd:
		return &models.ExperienceTime{
			TimeType: &models.ExperienceTime_Tbd{
				Tbd: &models.TimeTBD{},
			},
		}
	default:
		return &models.ExperienceTime{
			TimeType: &models.ExperienceTime_Tbd{
				Tbd: &models.TimeTBD{},
			},
		}
	}
}

// convertAPIValueEstimateToStorage converts an API ValueEstimate to a storage model ValueEstimate.
func convertAPIValueEstimateToStorage(apiEstimate *api.ValueEstimate) *models.ValueEstimate {
	if apiEstimate == nil {
		return nil
	}
	result := &models.ValueEstimate{
		EstimatedValueUsd: apiEstimate.EstimatedValueUsd,
	}
	if apiEstimate.Provenance != nil {
		result.Provenance = &models.Provenance{
			Source:     models.ProvenanceSource(apiEstimate.Provenance.Source),
			Name:       apiEstimate.Provenance.Name,
			Version:    apiEstimate.Provenance.Version,
			Confidence: apiEstimate.Provenance.Confidence,
			Reasoning:  apiEstimate.Provenance.Reasoning,
			Sources:    apiEstimate.Provenance.Sources,
		}
	}
	return result
}

// convertAPIExperienceMetadataToStorage unpacks API ExperienceMetadata into individual storage fields.
// Returns value_estimate and category.
func convertAPIExperienceMetadataToStorage(metadata *api.ExperienceMetadata) (*models.ValueEstimate, string) {
	if metadata == nil {
		return nil, ""
	}
	return convertAPIValueEstimateToStorage(metadata.ValueEstimate),
		metadata.Category
}

// experienceTimeParam converts an ExperienceTime into the chat system-message
// time param: TBD when no time is set, the user's own description (verbatim,
// never translated) for informal ranges, or the instant plus the event
// timezone's UTC offset for concrete times — the client renders the
// event-local wall time in the viewer's locale.
func experienceTimeParam(t *models.ExperienceTime) chat.TimeParam {
	if t == nil {
		return chat.TimeParam{TBD: true}
	}
	switch v := t.TimeType.(type) {
	case *models.ExperienceTime_Specific:
		loc, err := time.LoadLocation(v.Specific.Timezone)
		if err != nil {
			loc = time.UTC
		}
		ts := time.Unix(v.Specific.UnixTimestampSec, 0).In(loc)
		_, offsetSec := ts.Zone()
		return chat.TimeParam{
			UnixSec:      v.Specific.UnixTimestampSec,
			UTCOffsetMin: offsetSec / 60,
		}
	case *models.ExperienceTime_Range:
		if v.Range.Description != "" {
			return chat.TimeParam{Description: v.Range.Description}
		}
		return chat.TimeParam{TBD: true}
	case *models.ExperienceTime_Tbd:
		return chat.TimeParam{TBD: true}
	default:
		return chat.TimeParam{TBD: true}
	}
}

// Verify that Service implements the ExperienceServiceHandler interface.
var _ apiconnect.ExperienceServiceHandler = (*Service)(nil)
