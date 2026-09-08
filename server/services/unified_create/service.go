package unified_create

import (
	"go.ripls.org/ripls/server/ai"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/api/apiconnect"
	"go.ripls.org/ripls/server/genai/experiencegen"
	"go.ripls.org/ripls/server/genai/geargen"
	"go.ripls.org/ripls/server/genai/requestgen"
	"go.ripls.org/ripls/server/storage"
	"go.ripls.org/ripls/server/webfetch"
)

// Service implements UnifiedCreateService. After classification it
// delegates to the per-type streaming generators on experience/gear/
// request, bridging their emits into the unified response envelope via
// per-type adapters defined in stream_adapters.go. All fan-out,
// fallback, and final-stamping logic lives in the per-type services —
// unified_create is intentionally thin.
//
// Per-type service references are held as library-package interfaces
// (experiencegen.Generator etc.) rather than concrete *experience.Service
// pointers. This keeps unified_create from importing other service
// packages — see server/services/service_isolation_test.go — and lets
// tests inject fake generators.
type Service struct {
	apiconnect.UnimplementedUnifiedCreateServiceHandler
	classifier        Classifier
	aiProvider        ai.Provider
	storage           *storage.ProtoSQLStorage
	bucket            storage.BucketStorage
	fetcher           webfetch.Fetcher
	experienceService experiencegen.Generator
	gearService       geargen.Generator
	requestService    requestgen.Generator
}

// New constructs a Service. fetcher is required for URL-mode generation;
// nil disables URL mode. bucket is required for image-mode classification
// (resolving media_id to a presigned URL for the classifier prompt); nil
// makes image-mode return an internal error. The per-type generators must
// be non-nil — each is responsible for the streaming fan-out for its
// DetectedContentType.
func New(
	classifier Classifier,
	aiProvider ai.Provider,
	sqlStorage *storage.ProtoSQLStorage,
	bucket storage.BucketStorage,
	fetcher webfetch.Fetcher,
	experienceService experiencegen.Generator,
	gearService geargen.Generator,
	requestService requestgen.Generator,
) *Service {
	if classifier == nil {
		classifier = NewStubClassifier()
	}
	return &Service{
		classifier:        classifier,
		aiProvider:        aiProvider,
		storage:           sqlStorage,
		bucket:            bucket,
		fetcher:           fetcher,
		experienceService: experienceService,
		gearService:       gearService,
		requestService:    requestService,
	}
}

// detectedTypeFromString maps the classifier's string output to the
// proto enum value.
func detectedTypeFromString(s string) api.DetectedContentType {
	switch s {
	case "gear":
		return api.DetectedContentType_DETECTED_CONTENT_TYPE_GEAR
	case "event":
		return api.DetectedContentType_DETECTED_CONTENT_TYPE_EVENT
	case "request":
		return api.DetectedContentType_DETECTED_CONTENT_TYPE_REQUEST
	default:
		return api.DetectedContentType_DETECTED_CONTENT_TYPE_UNSPECIFIED
	}
}
