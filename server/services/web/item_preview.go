package web

import (
	"context"
	"strings"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// itemType classifies how an item landing page renders the item.
type itemType string

const (
	itemTypeGearLoan     itemType = "gear_loan"
	itemTypeGearGiveaway itemType = "gear_giveaway"
	itemTypeRequest      itemType = "request"
)

// itemPreviewData holds the item fields the gear and request landing
// pages render (name, hero image, loan/giveaway flavor, owner).
type itemPreviewData struct {
	ItemName     string
	ItemImageURL string
	ItemType     itemType
	OwnerName    string

	// ItemDescription is the item's own pitch — what a guest needs to read
	// before committing to the page's single action. Populated by
	// fetchRequestPreview; the gear landing doesn't render one yet.
	ItemDescription string

	// OwnerAvatarURL is a presigned URL for the owner's profile photo, or
	// empty when they have no photo (the landing falls back to their
	// initial). Populated by fetchRequestPreview; the gear landing doesn't
	// render one yet.
	OwnerAvatarURL string
}

// fetchGearPreview fetches gear, its owner, and the community-specific
// availability to build an item preview. Returns nil if the gear is missing,
// soft-deleted, or in a terminal state.
func (s *Service) fetchGearPreview(ctx context.Context, gearID, communityID string) *itemPreviewData {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "fetchGearPreview",
		"gear_id", gearID,
	)

	gear := &models.Gear{}
	if err := s.storage.GetByID(ctx, gearID, gear); err != nil {
		logger.Warn("failed to fetch gear for preview", "error", err)
		return nil
	}

	// Skip terminal states.
	if gear.State == models.GearState_GEAR_STATE_GIVEN_AWAY {
		logger.Debug("gear in terminal state, skipping preview", "state", gear.State.String())
		return nil
	}

	// Skip soft-deleted gear.
	if gear.Deleted != nil {
		logger.Debug("gear is soft-deleted, skipping preview")
		return nil
	}

	owner := &models.User{}
	if err := s.storage.GetByID(ctx, gear.OwnerId, owner); err != nil {
		logger.Warn("failed to fetch gear owner for preview", "error", err)
		return nil
	}

	// Determine loan vs giveaway from CommunityGear.
	it := itemTypeGearLoan
	communityGears, err := s.storage.QueryByFields(ctx, map[string]any{
		"community_id": communityID,
		"gear_id":      gearID,
	}, &models.CommunityGear{})
	if err == nil && len(communityGears) > 0 {
		cg := communityGears[0].(*models.CommunityGear)
		if cg.Availability == models.Availability_AVAILABILITY_FOR_GIVEAWAY {
			it = itemTypeGearGiveaway
		}
	}

	imageURL := s.getMediaImageURL(ctx, gear.MediaIds)

	return &itemPreviewData{
		ItemName:     gear.Name,
		ItemImageURL: imageURL,
		ItemType:     it,
		OwnerName:    firstName(owner.Name),
	}
}

// fetchRequestPreview fetches a request and its requester to build an item
// preview. Returns nil if the request is missing, soft-deleted, or cancelled.
func (s *Service) fetchRequestPreview(ctx context.Context, requestID string) *itemPreviewData {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "fetchRequestPreview",
		"target_request_id", requestID,
	)

	request := &models.Request{}
	if err := s.storage.GetByID(ctx, requestID, request); err != nil {
		logger.Warn("failed to fetch request for preview", "error", err)
		return nil
	}

	if request.State == models.RequestState_REQUEST_STATE_CANCELLED ||
		request.State == models.RequestState_REQUEST_STATE_FULFILLED {
		logger.Debug("request in terminal state, skipping preview", "state", request.State.String())
		return nil
	}

	if request.Deleted != nil {
		logger.Debug("request is soft-deleted, skipping preview")
		return nil
	}

	requester := &models.User{}
	if err := s.storage.GetByID(ctx, request.RequesterId, requester); err != nil {
		logger.Warn("failed to fetch requester for preview", "error", err)
		return nil
	}

	imageURL := s.getMediaImageURL(ctx, request.MediaIds)

	return &itemPreviewData{
		ItemName:        request.Title,
		ItemDescription: request.Description,
		ItemImageURL:    imageURL,
		ItemType:        itemTypeRequest,
		OwnerName:       firstName(requester.Name),
		OwnerAvatarURL:  s.getMediaImageURL(ctx, requester.MediaIds),
	}
}

// getMediaImageURL returns a presigned URL for the first media item in the list,
// or an empty string if no media is available.
func (s *Service) getMediaImageURL(ctx context.Context, mediaIDs []string) string {
	if len(mediaIDs) == 0 {
		return ""
	}

	mediaID := mediaIDs[0]
	media := &models.Media{}
	if err := s.storage.GetByID(ctx, mediaID, media); err != nil {
		logger := logging.LoggerWithContext(ctx)
		logger.Warn("failed to get item media for OG image", "error", err, "media_id", mediaID)
		return ""
	}

	// Prefer thumbnail: it has EXIF orientation baked in and is smaller,
	// which avoids rotated images in link previews on some platforms (e.g. Signal).
	bucketKey := storage.MediaBucketKey(media.UserId, media.Id)
	if media.GetThumbnailStorageUrl() != "" {
		bucketKey = storage.ThumbnailBucketKey(media.UserId, media.Id)
	}
	url, err := s.bucket.GetSignedURL(ctx, bucketKey, ogImageDuration)
	if err != nil {
		logger := logging.LoggerWithContext(ctx)
		logger.Warn("failed to generate presigned URL for item OG image", "error", err, "media_id", mediaID)
		return ""
	}

	return url
}

// firstName returns the first word of a name, for a less formal preview.
func firstName(name string) string {
	if i := strings.IndexByte(name, ' '); i > 0 {
		return name[:i]
	}
	return name
}
