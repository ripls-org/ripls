// Package auth provides authentication and identity primitives — JWT issuance
// and verification, OIDC provider management, password hashing, phone-token
// verification, and helpers for extracting caller identity from request context.
//
// # Community-scoped access control
//
// Three helpers enforce authorization at community boundaries:
//
//   - RequireMemberOfActiveCommunity: verifies the caller is an active member
//     of a specific community. Use this for RPCs that accept a community_id
//     parameter directly (e.g. GetCommunityActions, GetUserCommunityImpactDetail).
//
//   - RequireAccessToCommunityScopedEntity: verifies the caller can read a
//     community-shared entity (gear, experience, or request). Applies an
//     owner-bypass (callerID == ownerID always succeeds), then checks whether
//     the entity is shared with at least one community the caller belongs to.
//     Returns (sharedCommunityIDs, callerCommunityIDs, error) so handlers can
//     apply viewer-lens filtering when rendering SharedCommunities fields.
//
//   - RequireSharedCommunityWithUser: verifies the caller shares at least one
//     active community with a target user. Applies a self-bypass (callerID ==
//     targetID always succeeds). Use this for per-user profile and impact RPCs.
//
// All three helpers log denials at WARN level and return CodePermissionDenied;
// storage failures return CodeInternal via connecterr.Internal.
package auth
