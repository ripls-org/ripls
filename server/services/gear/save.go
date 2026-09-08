package gear

import (
	"context"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/impact_metrics/estimator"
)

// SaveGear inserts new gear or updates existing gear in the database.
// If req.Msg.Id is empty, creates new gear. Otherwise, updates existing gear.
// Only fields present in the request are updated; omitted fields are ignored.
// Repeated fields are replaced entirely, not appended.
func (s *Service) SaveGear(
	ctx context.Context,
	req *connect.Request[api.SaveGearRequest],
) (*connect.Response[api.SaveGearResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	// Determine if this is an insert or update
	isInsert := req.Msg.Id == ""

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
	)

	if isInsert {
		logger.Info("creating gear", "gear_name", req.Msg.GetName())

		// Create storage model from request; optional scalars default to "" when omitted.
		gearStored := &models.Gear{
			Name:             req.Msg.GetName(),
			Description:      req.Msg.GetDescription(),
			MediaIds:         req.Msg.MediaIds,
			LocationId:       req.Msg.GetLocationId(),
			OwnerId:          authInfo.UserID,
			State:            models.GearState_GEAR_STATE_AVAILABLE,
			CreatedAtUnixSec: time.Now().Unix(),
		}
		if req.Msg.SourceUrl != nil {
			gearStored.SourceUrl = *req.Msg.SourceUrl
		}

		// Extract metadata fields if metadata is provided
		if req.Msg.Metadata != nil {
			gearStored.Category = convertAPITrackedStringToStorage(req.Msg.Metadata.Category)
			gearStored.Brand = convertAPITrackedStringToStorage(req.Msg.Metadata.Brand)
			gearStored.Model = convertAPITrackedStringToStorage(req.Msg.Metadata.Model)
			gearStored.MaterialCategory = convertAPITrackedMaterialCategoryToStorage(req.Msg.Metadata.MaterialCategory)
			gearStored.WeightGrams = convertAPITrackedEstimateToStorage(req.Msg.Metadata.WeightGrams)

			// Convert and set value estimate if provided
			if req.Msg.Metadata.ValueEstimate != nil {
				gearStored.ValueEstimate = convertAPIValueEstimateToStorage(req.Msg.Metadata.ValueEstimate)
				logger.Info("saving value estimate",
					"value_usd", req.Msg.Metadata.ValueEstimate.EstimatedValueUsd)
			}
		}

		// Apply LLM provenance to metadata fields that lack provenance
		if metaProv := s.buildMetadataProvenance(req.Msg.GetGenerationMode()); metaProv != nil {
			applyDefaultProvenanceTrackedString(gearStored.Category, metaProv)
			applyDefaultProvenanceTrackedString(gearStored.Brand, metaProv)
			applyDefaultProvenanceTrackedString(gearStored.Model, metaProv)
			applyDefaultProvenanceTrackedMaterialCategory(gearStored.MaterialCategory, metaProv)
			applyDefaultProvenanceTrackedEstimate(gearStored.WeightGrams, metaProv)
		}

		// Compute embodied carbon from material/weight metadata if estimator is configured
		if s.estimatorCfg != nil {
			weightGrams := trackedEstimateMean(gearStored.WeightGrams)
			var valueUSD float32
			if gearStored.ValueEstimate != nil {
				valueUSD = gearStored.ValueEstimate.EstimatedValueUsd
			}
			carbonEst := estimator.EstimateGearCarbon(
				ctx, trackedMaterialCategoryValue(gearStored.MaterialCategory), weightGrams,
				valueUSD, trackedStringValue(gearStored.Category), s.estimatorCfg, nil,
			)
			if carbonEst != nil {
				gearStored.EmbodiedCarbon = &models.CarbonEstimate{
					Co2EGrams: &models.Estimate{
						Mean:   carbonEst.Co2EGrams.Mean,
						Stddev: carbonEst.Co2EGrams.Stddev,
					},
				}
				if carbonEst.Provenance != nil {
					gearStored.EmbodiedCarbon.Provenance = &models.Provenance{
						Source:  models.ProvenanceSource(carbonEst.Provenance.Source),
						Name:    carbonEst.Provenance.Name,
						Version: carbonEst.Provenance.Version,
					}
				}
				logger.Info("computed embodied carbon",
					"co2e_grams", carbonEst.Co2EGrams.Mean,
					"provenance", carbonEst.Provenance.GetName())
			}
		}

		// Insert into database
		id, err := s.storage.Insert(ctx, gearStored)
		if err != nil {
			logger.Error("failed to insert gear", "error", err)
			return nil, connecterr.Internal(ctx, "SaveGear", err)
		}

		// Provision the gear's per-item ad-hoc community (#2492) and share the
		// gear into it, so every gear is born with its own audience community —
		// mirroring SaveExperience / SubmitRequest. The provisioner (wired in
		// main.go) composes community.ProvisionPerItemCommunity + the community
		// service's gear share; gear sharing lives there, so this indirection
		// avoids a gear→community-service dependency. The share carries the
		// creation-time Lend/Give choice (#2687) — ShareItem community invites
		// inherit it from this first share — defaulting FOR_LOAN when unset.
		res := connect.NewResponse(&api.SaveGearResponse{
			Id: id,
		})
		if s.provisionItemCommunity != nil {
			itemCommunityID, err := s.provisionItemCommunity(ctx, authInfo.UserID, id, initialAvailability(req.Msg))
			if err != nil {
				logger.Error("failed to provision per-item community for gear", "gear_id", id, "error", err)
				return nil, connecterr.Internal(ctx, "SaveGear", err)
			}
			logger.Info("gear created", "gear_id", id, "item_community_id", itemCommunityID)
			if itemCommunityID != "" {
				res.Msg.ItemCommunityId = &itemCommunityID
			}
		}
		res.Header().Set("Gearserver-Version", "v1")
		return res, nil
	}

	// Update existing gear
	logger.Info("updating gear", "gear_id", req.Msg.Id)

	// Fetch existing gear
	gearStored := &models.Gear{}
	err = s.storage.GetByID(ctx, req.Msg.Id, gearStored)
	if err != nil {
		logger.Error("failed to get gear for update", "gear_id", req.Msg.Id, "error", err)
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	// Verify ownership — with a carve-out for non-owners appending media.
	// Any member of a community the gear is shared with may add media
	// (e.g. borrower photos uploaded from the media carousel), as long as
	// they don't try to change any other field and the new media list is a
	// superset of the existing one. All other update paths remain
	// owner-only.
	if gearStored.OwnerId != authInfo.UserID {
		if !isAppendOnlyGearMediaUpdate(req.Msg, gearStored) {
			return nil, connecterr.UserVisible(ctx, connect.CodePermissionDenied, "gear_owner_required_for_update", "only the owner can update this gear", nil)
		}
		if mayAddErr := s.callerMayAddGearMedia(ctx, req.Msg.Id, authInfo.UserID); mayAddErr != nil {
			return nil, mayAddErr
		}
	}

	// Update only fields present in the request (nil = omitted = leave unchanged; non-nil = set, including "").
	// For repeated fields, they are replaced entirely.
	if req.Msg.Name != nil {
		gearStored.Name = *req.Msg.Name
	}
	if req.Msg.Description != nil {
		gearStored.Description = *req.Msg.Description
	}
	if req.Msg.LocationId != nil {
		gearStored.LocationId = *req.Msg.LocationId
	}
	// Repeated fields are always replaced (even if empty slice)
	gearStored.MediaIds = req.Msg.MediaIds
	// Source URL can be set on update (though typically only set on create)
	if req.Msg.SourceUrl != nil {
		gearStored.SourceUrl = *req.Msg.SourceUrl
	}

	// Update metadata fields if provided, tracking user edits with USER provenance
	if req.Msg.Metadata != nil {
		needsCarbonRecompute := false
		userProv := &models.Provenance{Source: models.ProvenanceSource_PROVENANCE_SOURCE_USER}

		// Category
		if req.Msg.Metadata.Category != nil {
			incoming := convertAPITrackedStringToStorage(req.Msg.Metadata.Category)
			if trackedStringChanged(gearStored.Category, incoming) {
				incoming.Provenance = userProv
			} else if incoming != nil && gearStored.Category != nil {
				incoming.Provenance = gearStored.Category.Provenance
			}
			gearStored.Category = incoming
		}

		// Brand
		if req.Msg.Metadata.Brand != nil {
			incoming := convertAPITrackedStringToStorage(req.Msg.Metadata.Brand)
			if trackedStringChanged(gearStored.Brand, incoming) {
				incoming.Provenance = userProv
			} else if incoming != nil && gearStored.Brand != nil {
				incoming.Provenance = gearStored.Brand.Provenance
			}
			gearStored.Brand = incoming
		}

		// Model
		if req.Msg.Metadata.Model != nil {
			incoming := convertAPITrackedStringToStorage(req.Msg.Metadata.Model)
			if trackedStringChanged(gearStored.Model, incoming) {
				incoming.Provenance = userProv
			} else if incoming != nil && gearStored.Model != nil {
				incoming.Provenance = gearStored.Model.Provenance
			}
			gearStored.Model = incoming
		}

		// Material category
		if req.Msg.Metadata.MaterialCategory != nil {
			incoming := convertAPITrackedMaterialCategoryToStorage(req.Msg.Metadata.MaterialCategory)
			if trackedMaterialCategoryChanged(gearStored.MaterialCategory, incoming) {
				incoming.Provenance = userProv
				needsCarbonRecompute = true
			} else if incoming != nil && gearStored.MaterialCategory != nil {
				incoming.Provenance = gearStored.MaterialCategory.Provenance
			}
			gearStored.MaterialCategory = incoming
		}

		// Weight grams
		if req.Msg.Metadata.WeightGrams != nil {
			incoming := convertAPITrackedEstimateToStorage(req.Msg.Metadata.WeightGrams)
			if trackedEstimateChanged(gearStored.WeightGrams, incoming) {
				incoming.Provenance = userProv
				needsCarbonRecompute = true
			} else if incoming != nil && gearStored.WeightGrams != nil {
				incoming.Provenance = gearStored.WeightGrams.Provenance
			}
			gearStored.WeightGrams = incoming
		}

		// Value estimate
		if req.Msg.Metadata.ValueEstimate != nil {
			gearStored.ValueEstimate = convertAPIValueEstimateToStorage(req.Msg.Metadata.ValueEstimate)
			logger.Info("updating value estimate",
				"value_usd", req.Msg.Metadata.ValueEstimate.EstimatedValueUsd)
		}

		// Recompute embodied carbon if material or weight changed
		if needsCarbonRecompute && s.estimatorCfg != nil {
			weightGrams := trackedEstimateMean(gearStored.WeightGrams)
			var valueUSD float32
			if gearStored.ValueEstimate != nil {
				valueUSD = gearStored.ValueEstimate.EstimatedValueUsd
			}
			carbonEst := estimator.EstimateGearCarbon(
				ctx, trackedMaterialCategoryValue(gearStored.MaterialCategory), weightGrams,
				valueUSD, trackedStringValue(gearStored.Category), s.estimatorCfg, nil,
			)
			if carbonEst != nil {
				gearStored.EmbodiedCarbon = &models.CarbonEstimate{
					Co2EGrams: &models.Estimate{
						Mean:   carbonEst.Co2EGrams.Mean,
						Stddev: carbonEst.Co2EGrams.Stddev,
					},
				}
				if carbonEst.Provenance != nil {
					gearStored.EmbodiedCarbon.Provenance = &models.Provenance{
						Source:  models.ProvenanceSource(carbonEst.Provenance.Source),
						Name:    carbonEst.Provenance.Name,
						Version: carbonEst.Provenance.Version,
					}
				}
				logger.Info("recomputed embodied carbon after user edit",
					"co2e_grams", carbonEst.Co2EGrams.Mean)
			}
		}
	}

	// Update in database
	err = s.storage.Update(ctx, gearStored)
	if err != nil {
		logger.Error("failed to update gear", "gear_id", req.Msg.Id, "error", err)
		return nil, connecterr.Internal(ctx, "SaveGear", err)
	}

	res := connect.NewResponse(&api.SaveGearResponse{
		Id: gearStored.Id,
	})
	res.Header().Set("Gearserver-Version", "v1")
	return res, nil
}

// initialAvailability maps the request's creation-time Lend/Give choice to
// the storage availability for the per-item community's first share (#2687).
// Defaults FOR_LOAN when unset, matching the create preview's default toggle
// position.
func initialAvailability(msg *api.SaveGearRequest) models.Availability {
	if msg.GetAvailability() == api.Availability_AVAILABILITY_FOR_GIVEAWAY {
		return models.Availability_AVAILABILITY_FOR_GIVEAWAY
	}
	return models.Availability_AVAILABILITY_FOR_LOAN
}
