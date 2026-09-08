package media

// SystemUserID is the sentinel media.user_id value used for stock images
// (Unsplash, Pexels, Pixabay, and the fake provider used in dev / tests).
// Stock images are not uploaded by any specific user — they are
// ingested into the media table with this sentinel owner so they
// participate in the standard media schema (id, content type, bucket
// key) without requiring a per-row owner.
//
// Exported because the per-resource read authorization in
// server/services/media/authorization.go uses it as a pass-rule:
// any media with UserId == SystemUserID is readable by any
// authenticated caller (stock images are public-by-design).
const SystemUserID = "system"
