// Anonymous decline counter for the public SSR event landing.
//
// The Yes / Maybe paths on /go/{code} hand off to Flutter Web for
// auth + RSVP; the No path stays on the SSR page and POSTs to this
// handler so the host can see "N visitors said they can't make it"
// without any identity capture.

package web

import (
	"errors"
	"net/http"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// HandleRecordWebDecline increments the anonymous decline counter on
// an event-flavored share link's underlying Experience. Wired to
// POST /go/{code}/decline. Returns 204 on success, 404 when the
// share link or experience is missing, 410 when the link is revoked,
// 400 when the link's target is not an event, and 500 on internal
// failure.
func (s *Service) HandleRecordWebDecline(w http.ResponseWriter, r *http.Request) {
	shortCode := r.PathValue("code")

	logger := logging.LoggerWithContext(r.Context()).With(
		"operation", "RecordWebDecline",
		"short_code", logging.MaskToken(shortCode),
	)

	if shortCode == "" {
		logger.InfoContext(r.Context(), "missing short code in decline request")
		http.Error(w, "missing short code", http.StatusBadRequest)
		return
	}

	links, err := s.storage.QueryByField(r.Context(), "short_code", shortCode, &models.ShareLink{})
	if err != nil {
		logger.ErrorContext(r.Context(), "failed to look up share link", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	if len(links) == 0 {
		logger.InfoContext(r.Context(), "share link not found")
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	link := links[0].(*models.ShareLink)
	experienceID := link.GetExperienceId()
	logger = logger.With(
		"share_link_id", link.GetId(),
		"community_id", link.GetCommunityId(),
		"experience_id", experienceID,
	)

	if link.IsRevoked {
		logger.InfoContext(r.Context(), "decline rejected: share link is revoked")
		http.Error(w, "gone", http.StatusGone)
		return
	}

	if experienceID == "" {
		logger.InfoContext(r.Context(), "decline rejected: share link target is not an event")
		http.Error(w, "decline is only supported for event share links", http.StatusBadRequest)
		return
	}

	exp := &models.Experience{}
	if err := s.storage.GetByID(r.Context(), experienceID, exp); err != nil {
		if errors.Is(err, storage.ErrRecordNotFound) {
			logger.InfoContext(r.Context(), "decline rejected: experience not found")
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		logger.ErrorContext(r.Context(), "failed to fetch experience", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	current := exp.GetWebDeclineCount()
	next := current + 1
	exp.WebDeclineCount = &next

	if err := s.storage.Update(r.Context(), exp); err != nil {
		logger.ErrorContext(r.Context(), "failed to update web_decline_count", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	logger.InfoContext(r.Context(), "recorded web decline",
		"previous_count", current,
		"new_count", next,
	)
	w.WriteHeader(http.StatusNoContent)
}
