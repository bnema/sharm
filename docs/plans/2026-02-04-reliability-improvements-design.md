# Sharm Reliability Improvements Design

Date: 2026-02-04

## Overview

Four improvements to enhance reliability and user experience:
1. Fix days remaining display
2. Chunked upload for mobile resilience
3. Client-side probe with HTMLVideoElement
4. Basic PWA for Android install

---

## 1. Fix Days Remaining Display

**Problem**: `dashboard.templ:153` displays `m.RetentionDays` (static config value) instead of actual days remaining until expiration.

**Solution**: Add `DaysRemaining()` method to `Media` domain struct.

```go
// domain/media.go
func (m *Media) DaysRemaining() int {
    remaining := time.Until(m.ExpiresAt).Hours() / 24
    if remaining < 0 {
        return 0
    }
    return int(math.Ceil(remaining))
}
```

**Changes**:
- `internal/domain/media.go`: Add `DaysRemaining()` method
- `internal/adapter/http/templates/dashboard.templ`: Replace `m.RetentionDays` with `m.DaysRemaining()`

---

## 2. Chunked Upload for Mobile Resilience

**Problem**: Current upload streams entire file in one request. Unstable LTE connections cause full re-uploads on failure.

**Solution**: Client-side chunking with server-side assembly.

**Parameters**:
- Chunk size: 5 MB
- Retry attempts per chunk: 3
- Retry delay: exponential backoff (1s, 2s, 4s)

**New Endpoints**:

```text
POST /upload/chunk
  Form data:
    - uploadId: string (client-generated UUID)
    - chunkIndex: int
    - totalChunks: int
    - chunk: binary data

POST /upload/complete
  Form data:
    - uploadId: string
    - filename: string
    - retentionDays: int
    - codecs: []string
```

**Server Storage**:

```text
{dataDir}/chunks/{uploadId}/
  ├── 0
  ├── 1
  └── ...
```

**Flow**:
1. Client generates uploadId (UUID)
2. Client splits file into 5MB chunks
3. Client uploads each chunk sequentially with retry
4. Client calls /upload/complete
5. Server concatenates chunks, processes as normal upload
6. Server cleans up chunk directory

**Changes**:
- `internal/adapter/http/handler.go`: Add `HandleChunkUpload`, `HandleCompleteUpload`
- `internal/adapter/http/routes.go`: Register new routes
- `static/js/upload.js`: New chunked upload logic with retry

---

## 3. Client-Side Probe with HTMLVideoElement

**Problem**: `/probe` endpoint requires full file upload before returning metadata, causing double upload.

**Solution**: Use browser's native HTMLVideoElement for pre-upload preview. Keep server-side ffprobe for detailed info after upload.

**Client-Side Probe** (basic info):
```javascript
function probeFile(file) {
    return new Promise((resolve, reject) => {
        const video = document.createElement('video');
        video.preload = 'metadata';

        video.onloadedmetadata = () => {
            resolve({
                duration: video.duration,
                width: video.videoWidth,
                height: video.videoHeight,
                type: file.type
            });
            URL.revokeObjectURL(video.src);
        };

        video.onerror = () => reject(new Error('Cannot read video metadata'));
        video.src = URL.createObjectURL(file);
    });
}
```

**Changes**:
- `internal/adapter/http/handler.go`: Remove `HandleProbeUpload`
- `internal/adapter/http/routes.go`: Remove `/probe` route
- `internal/adapter/http/templates/upload.templ`: Update UI to show client probe results
- `static/js/upload.js`: Add client-side probe function

**Note**: Server-side ffprobe remains for admin MediaInfo dialog (runs on already-uploaded files).

---

## 4. Basic PWA for Android Install

**Problem**: No app icon when accessing from Android home screen.

**Solution**: Add web app manifest and minimal service worker.

**manifest.json**:
```json
{
  "name": "Sharm",
  "short_name": "Sharm",
  "description": "Share media with expiring links",
  "start_url": "/",
  "display": "standalone",
  "background_color": "#1a1a1a",
  "theme_color": "#1a1a1a",
  "icons": [
    {
      "src": "/static/icon-192.png",
      "sizes": "192x192",
      "type": "image/png"
    },
    {
      "src": "/static/icon-512.png",
      "sizes": "512x512",
      "type": "image/png"
    }
  ]
}
```

**Service Worker** (minimal, for installability):
```javascript
// sw.js
self.addEventListener('fetch', (event) => {
    event.respondWith(fetch(event.request));
});
```

**Changes**:
- `static/manifest.json`: New file
- `static/sw.js`: New file (minimal)
- `static/icon-192.png`: New icon
- `static/icon-512.png`: New icon
- `internal/adapter/http/templates/layout.templ`: Add manifest link and SW registration

---

## Implementation Order

1. **Days remaining fix** - Quick win, isolated change
2. **PWA** - Quick win, no backend changes
3. **Client-side probe** - Remove code, simplify
4. **Chunked upload** - Most complex, do last

---

## Testing

- Days remaining: Create media, wait, verify display updates
- Chunked upload: Test with network throttling, simulate failures
- Client probe: Test various video formats (MP4, WebM, MOV)
- PWA: Test install on Android Chrome
