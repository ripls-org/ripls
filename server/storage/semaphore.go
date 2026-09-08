package storage

// MediaProcessingSemaphore limits concurrent media processing operations to prevent memory exhaustion.
// Each media operation (image download + thumbnail generation) uses ~50-100MB of memory.
// Limiting to 2 concurrent operations keeps total memory usage bounded.
//
// This semaphore should be used by any code that:
// - Downloads images from external sources (Unsplash, etc.)
// - Calls StoreMedia which generates thumbnails.
var MediaProcessingSemaphore = make(chan struct{}, 2)

// AcquireMediaSemaphore acquires a slot in the media processing semaphore.
// This blocks until a slot is available.
func AcquireMediaSemaphore() {
	MediaProcessingSemaphore <- struct{}{}
}

// ReleaseMediaSemaphore releases a slot in the media processing semaphore.
// This should be called in a defer after AcquireMediaSemaphore.
func ReleaseMediaSemaphore() {
	<-MediaProcessingSemaphore
}
