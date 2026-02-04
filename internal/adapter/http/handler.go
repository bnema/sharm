package http

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/bnema/sharm/internal/adapter/http/templates"
	"github.com/bnema/sharm/internal/adapter/http/validation"
	"github.com/bnema/sharm/internal/domain"
	"github.com/bnema/sharm/internal/infrastructure/logger"
)

type MediaService interface {
	Upload(filename string, file *os.File, retentionDays int, mediaType domain.MediaType, codecs []domain.Codec, fps int) (*domain.Media, error)
	Get(id string) (*domain.Media, error)
	ListAll() ([]*domain.Media, error)
	Delete(id string) error
	ProbeFile(filePath string) (*domain.ProbeResult, error)
}

type Handlers struct {
	mediaSvc  MediaService
	domain    string
	maxSizeMB int
	version   string
}

func NewHandlers(mediaSvc MediaService, domain string, maxSizeMB int, version string) *Handlers {
	return &Handlers{
		mediaSvc:  mediaSvc,
		domain:    domain,
		maxSizeMB: maxSizeMB,
		version:   version,
	}
}

func (h *Handlers) Dashboard() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		media, err := h.mediaSvc.ListAll()
		if err != nil {
			logger.Error.Printf("dashboard list error: %v", err)
			media = []*domain.Media{}
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = templates.Dashboard(media, h.domain, h.version).Render(r.Context(), w)
	}
}

func (h *Handlers) UploadPage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = templates.Upload(h.version).Render(r.Context(), w)
	}
}

func (h *Handlers) Upload() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, int64(h.maxSizeMB)*1024*1024)

		if err := r.ParseMultipartForm(int64(h.maxSizeMB) * 1024 * 1024); err != nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			_ = templates.ErrorInline("File too large").Render(r.Context(), w)
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			_ = templates.ErrorInline("Invalid file upload").Render(r.Context(), w)
			return
		}
		defer file.Close() //nolint:errcheck

		// Validate file type using magic bytes
		_, allowed, err := validation.ValidateMagicBytes(file)
		if err != nil {
			logger.Error.Printf("magic bytes validation error for %s: %v", header.Filename, err)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusInternalServerError)
			_ = templates.ErrorInline("Failed to validate file type").Render(r.Context(), w)
			return
		}
		if !allowed {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			_ = templates.ErrorInline("File type not allowed").Render(r.Context(), w)
			return
		}

		tmpFile, err := os.CreateTemp("", "upload-*.tmp")
		if err != nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusInternalServerError)
			_ = templates.ErrorInline("Failed to process upload").Render(r.Context(), w)
			return
		}
		defer func() {
			_ = tmpFile.Close()
			_ = os.Remove(tmpFile.Name()) // may already be moved by service
		}()

		if _, err := io.Copy(tmpFile, file); err != nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusInternalServerError)
			_ = templates.ErrorInline("Failed to save file").Render(r.Context(), w)
			return
		}

		retentionStr := r.FormValue("retention")
		retentionDays, err := strconv.Atoi(retentionStr)
		if err != nil {
			retentionDays = 7
		}

		// Parse selected codecs from form
		var codecs []domain.Codec
		for _, c := range r.Form["codecs"] {
			switch domain.Codec(c) {
			case domain.CodecAV1, domain.CodecH264, domain.CodecOpus:
				codecs = append(codecs, domain.Codec(c))
			}
		}

		fps, _ := strconv.Atoi(r.FormValue("fps"))

		mediaType := domain.DetectMediaType(header.Filename)
		_, err = h.mediaSvc.Upload(header.Filename, tmpFile, retentionDays, mediaType, codecs, fps)
		if err != nil {
			logger.Error.Printf("upload error for %s: %v", header.Filename, err)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusInternalServerError)
			msg := "Upload failed"
			if strings.Contains(err.Error(), "no space left") {
				msg = "Upload failed: disk full"
			} else if strings.Contains(err.Error(), "permission denied") {
				msg = "Upload failed: permission error"
			}
			_ = templates.ErrorInline(msg).Render(r.Context(), w)
			return
		}

		// Redirect to dashboard where SSE updates the row live
		w.Header().Set("HX-Redirect", "/")
		w.WriteHeader(http.StatusOK)
	}
}

const chunkSize = 5 * 1024 * 1024 // 5MB

// validateUploadID checks that uploadID is a valid UUID-like string (alphanumeric with dashes).
func validateUploadID(uploadID string) bool {
	if uploadID == "" || len(uploadID) > 64 {
		return false
	}
	for _, c := range uploadID {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-') {
			return false
		}
	}
	return true
}

func (h *Handlers) ChunkUpload() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, chunkSize+1024*1024) // chunk + overhead

		if err := r.ParseMultipartForm(chunkSize + 1024*1024); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		uploadID := r.FormValue("uploadId")
		chunkIndexStr := r.FormValue("chunkIndex")
		if uploadID == "" || chunkIndexStr == "" {
			http.Error(w, "Missing uploadId or chunkIndex", http.StatusBadRequest)
			return
		}

		if !validateUploadID(uploadID) {
			http.Error(w, "Invalid uploadId format", http.StatusBadRequest)
			return
		}

		// Parse and validate chunkIndex to prevent path traversal
		chunkIdx, err := strconv.Atoi(chunkIndexStr)
		if err != nil || chunkIdx < 0 {
			http.Error(w, "Invalid chunkIndex", http.StatusBadRequest)
			return
		}

		file, _, err := r.FormFile("chunk")
		if err != nil {
			http.Error(w, "Invalid chunk data", http.StatusBadRequest)
			return
		}
		defer func() {
			if err := file.Close(); err != nil {
				logger.Error.Printf("failed to close chunk file for upload %s chunk %d: %v", uploadID, chunkIdx, err)
			}
		}()

		chunkDir := filepath.Join(os.TempDir(), "sharm-chunks", uploadID)
		if err := os.MkdirAll(chunkDir, 0750); err != nil {
			logger.Error.Printf("failed to create chunk dir: %v", err)
			http.Error(w, "Server error", http.StatusInternalServerError)
			return
		}

		chunkPath := filepath.Join(chunkDir, strconv.Itoa(chunkIdx))
		out, err := os.OpenFile(chunkPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			logger.Error.Printf("failed to create chunk file %s: %v", chunkPath, err)
			http.Error(w, "Server error", http.StatusInternalServerError)
			return
		}
		defer func() {
			if err := out.Close(); err != nil {
				logger.Error.Printf("failed to close output file %s: %v", chunkPath, err)
			}
		}()

		if _, err := io.Copy(out, file); err != nil {
			logger.Error.Printf("failed to write chunk: %v", err)
			http.Error(w, "Server error", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("OK")); err != nil {
			logger.Error.Printf("failed to write response for chunk %d: %v", chunkIdx, err)
		}
	}
}

func (h *Handlers) CompleteUpload() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1024 * 1024); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		uploadID := r.FormValue("uploadId")
		filename := r.FormValue("filename")
		totalChunksStr := r.FormValue("totalChunks")
		retentionStr := r.FormValue("retention")

		if uploadID == "" || filename == "" || totalChunksStr == "" {
			http.Error(w, "Missing required fields", http.StatusBadRequest)
			return
		}

		if !validateUploadID(uploadID) {
			http.Error(w, "Invalid uploadId format", http.StatusBadRequest)
			return
		}

		totalChunks, err := strconv.Atoi(totalChunksStr)
		if err != nil || totalChunks < 1 {
			http.Error(w, "Invalid totalChunks", http.StatusBadRequest)
			return
		}
		// Prevent DoS via huge totalChunks value (max ~100GB at 5MB/chunk)
		if totalChunks > 20000 {
			http.Error(w, "Too many chunks", http.StatusBadRequest)
			return
		}

		retentionDays, err := strconv.Atoi(retentionStr)
		if err != nil {
			retentionDays = 7
		}

		// Parse codecs
		var codecs []domain.Codec
		for _, c := range r.Form["codecs"] {
			switch domain.Codec(c) {
			case domain.CodecAV1, domain.CodecH264, domain.CodecOpus:
				codecs = append(codecs, domain.Codec(c))
			}
		}

		fps, _ := strconv.Atoi(r.FormValue("fps"))

		chunkDir := filepath.Join(os.TempDir(), "sharm-chunks", uploadID)
		defer func() {
			if err := os.RemoveAll(chunkDir); err != nil {
				logger.Error.Printf("failed to cleanup chunk dir %s: %v", chunkDir, err)
			}
		}()

		// Assemble chunks into temp file
		assembled, err := os.CreateTemp("", "upload-assembled-*.tmp")
		if err != nil {
			logger.Error.Printf("failed to create assembled file: %v", err)
			http.Error(w, "Server error", http.StatusInternalServerError)
			return
		}
		defer func() {
			if err := assembled.Close(); err != nil {
				logger.Error.Printf("failed to close assembled file: %v", err)
			}
			if err := os.Remove(assembled.Name()); err != nil && !os.IsNotExist(err) {
				logger.Error.Printf("failed to remove assembled file: %v", err)
			}
		}()

		for i := range totalChunks {
			chunkPath := filepath.Join(chunkDir, strconv.Itoa(i))
			chunk, err := os.Open(chunkPath)
			if err != nil {
				logger.Error.Printf("missing chunk %d for upload %s: %v", i, uploadID, err)
				http.Error(w, fmt.Sprintf("Missing chunk %d", i), http.StatusBadRequest)
				return
			}
			_, copyErr := io.Copy(assembled, chunk)
			if closeErr := chunk.Close(); closeErr != nil {
				logger.Error.Printf("failed to close chunk %d for upload %s: %v", i, uploadID, closeErr)
			}
			if copyErr != nil {
				logger.Error.Printf("failed to copy chunk %d: %v", i, copyErr)
				http.Error(w, "Server error", http.StatusInternalServerError)
				return
			}
		}

		// Reset file position for reading
		if _, err := assembled.Seek(0, 0); err != nil {
			logger.Error.Printf("failed to seek assembled file: %v", err)
			http.Error(w, "Server error", http.StatusInternalServerError)
			return
		}

		// Validate assembled file type using magic bytes
		_, allowed, err := validation.ValidateMagicBytes(assembled)
		if err != nil {
			logger.Error.Printf("magic bytes validation error for %s: %v", filename, err)
			http.Error(w, "Failed to validate file type", http.StatusInternalServerError)
			return
		}
		if !allowed {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			_ = templates.ErrorInline("File type not allowed").Render(r.Context(), w)
			return
		}

		mediaType := domain.DetectMediaType(filename)
		_, err = h.mediaSvc.Upload(filename, assembled, retentionDays, mediaType, codecs, fps)
		if err != nil {
			logger.Error.Printf("upload error for %s: %v", filename, err)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusInternalServerError)
			msg := "Upload failed"
			if strings.Contains(err.Error(), "no space left") {
				msg = "Upload failed: disk full"
			} else if strings.Contains(err.Error(), "permission denied") {
				msg = "Upload failed: permission error"
			}
			_ = templates.ErrorInline(msg).Render(r.Context(), w)
			return
		}

		w.Header().Set("HX-Redirect", "/")
		w.WriteHeader(http.StatusOK)
	}
}

func (h *Handlers) StatusPage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/status/")
		id = strings.TrimSuffix(id, "/")

		media, err := h.mediaSvc.Get(id)
		if err != nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusNotFound)
			if r.Header.Get("HX-Request") == "true" {
				_ = templates.ErrorInline("Media not found").Render(r.Context(), w)
			} else {
				_ = templates.ErrorPage("404", "Media not found", h.version).Render(r.Context(), w)
			}
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")

		// HTMX polling request — return fragment
		if r.Header.Get("HX-Request") == "true" {
			switch media.Status {
			case domain.MediaStatusPending, domain.MediaStatusProcessing:
				_ = templates.StatusPolling(media.ID).Render(r.Context(), w)
			case domain.MediaStatusDone:
				shareURL := fmt.Sprintf("https://%s/v/%s", h.domain, media.ID)
				_ = templates.StatusDone(media, shareURL).Render(r.Context(), w)
			case domain.MediaStatusFailed:
				_ = templates.StatusFailed(media.ErrorMessage).Render(r.Context(), w)
			}
			return
		}

		// Full page request — if already done, redirect to share page
		if media.Status == domain.MediaStatusDone {
			http.Redirect(w, r, "/v/"+media.ID, http.StatusSeeOther)
			return
		}

		_ = templates.StatusPage(media.ID, h.version).Render(r.Context(), w)
	}
}

func (h *Handlers) DeleteMedia() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/media/")
		id = strings.TrimSuffix(id, "/")

		if err := h.mediaSvc.Delete(id); err != nil {
			logger.Error.Printf("delete error for %s: %v", id, err)
			http.Error(w, "Delete failed", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	}
}

func (h *Handlers) ProbeUpload() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, int64(h.maxSizeMB)*1024*1024)

		if err := r.ParseMultipartForm(32 << 20); err != nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			_ = templates.ErrorInline("Invalid file upload").Render(r.Context(), w)
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			_ = templates.ErrorInline("Invalid file upload").Render(r.Context(), w)
			return
		}
		defer func() {
			if err := file.Close(); err != nil {
				logger.Error.Printf("failed to close uploaded file: %v", err)
			}
		}()

		tmpFile, err := os.CreateTemp("", "probe-*.tmp")
		if err != nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusInternalServerError)
			_ = templates.ErrorInline("Failed to process file").Render(r.Context(), w)
			return
		}
		defer func() {
			_ = tmpFile.Close()
			_ = os.Remove(tmpFile.Name()) // may already be moved by service
		}()

		if _, err := io.Copy(tmpFile, file); err != nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusInternalServerError)
			_ = templates.ErrorInline("Failed to read file").Render(r.Context(), w)
			return
		}

		probeResult, err := h.mediaSvc.ProbeFile(tmpFile.Name())
		if err != nil {
			logger.Error.Printf("probe error for %s: %v", header.Filename, err)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusInternalServerError)
			_ = templates.ErrorInline("Failed to probe file").Render(r.Context(), w)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = templates.ProbeResult(probeResult, header.Filename).Render(r.Context(), w)
	}
}

func (h *Handlers) MediaInfo() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/media/")
		id = strings.TrimSuffix(id, "/info")
		id = strings.TrimSuffix(id, "/")

		media, err := h.mediaSvc.Get(id)
		if err != nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusNotFound)
			_ = templates.ErrorInline("Media not found").Render(r.Context(), w)
			return
		}

		var probe *domain.ProbeResult
		if media.ProbeJSON != "" {
			probe, _ = media.ParseProbe()
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = templates.MediaInfoDialog(media, probe).Render(r.Context(), w)
	}
}

func (h *Handlers) Media() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Extract the media ID and suffix
		trimmed := strings.TrimPrefix(path, "/v/")
		parts := strings.SplitN(trimmed, "/", 2)
		id := parts[0]
		suffix := ""
		if len(parts) > 1 {
			suffix = parts[1]
		}

		switch suffix {
		case "raw", "raw.mp4":
			h.ServeRaw()(w, r)
		case "thumb":
			h.ServeThumb()(w, r)
		case "original":
			h.ServeOriginal(id)(w, r)
		case "av1":
			h.ServeVariant(id, domain.CodecAV1)(w, r)
		case "h264":
			h.ServeVariant(id, domain.CodecH264)(w, r)
		case "opus":
			h.ServeVariant(id, domain.CodecOpus)(w, r)
		default:
			h.SharePage()(w, r)
		}
	}
}

func (h *Handlers) SharePage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/v/")
		id = strings.TrimSuffix(id, "/")

		media, err := h.mediaSvc.Get(id)
		if err != nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusNotFound)
			_ = templates.ErrorPage("404", "Media not found", h.version).Render(r.Context(), w)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = templates.Share(media, h.domain).Render(r.Context(), w)
	}
}

func (h *Handlers) ServeOriginal(id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		media, err := h.mediaSvc.Get(id)
		if err != nil {
			http.Error(w, "Media not found", http.StatusNotFound)
			return
		}

		if media.OriginalPath == "" {
			http.Error(w, "Original not available", http.StatusNotFound)
			return
		}

		mimeType := detectOriginalMIMEType(media)
		w.Header().Set("Content-Type", mimeType)
		w.Header().Set("Content-Disposition", validation.ContentDisposition(media.OriginalName, true))
		http.ServeFile(w, r, media.OriginalPath)
	}
}

func (h *Handlers) ServeVariant(id string, codec domain.Codec) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		media, err := h.mediaSvc.Get(id)
		if err != nil {
			http.Error(w, "Media not found", http.StatusNotFound)
			return
		}

		v := media.VariantByCodec(codec)
		if v == nil || v.Status != domain.VariantStatusDone || v.Path == "" {
			http.Error(w, "Variant not available", http.StatusNotFound)
			return
		}

		mimeType := codecMIMEType(codec, media.Type)
		w.Header().Set("Content-Type", mimeType)
		w.Header().Set("Content-Disposition", validation.ContentDisposition(variantFilename(media.OriginalName, codec), true))
		http.ServeFile(w, r, v.Path)
	}
}

func (h *Handlers) ServeRaw() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/v/")
		id = strings.TrimSuffix(id, "/raw")
		id = strings.TrimSuffix(id, "/raw.mp4")
		id = strings.TrimSuffix(id, "/")

		media, err := h.mediaSvc.Get(id)
		if err != nil {
			http.Error(w, "Media not found", http.StatusNotFound)
			return
		}

		// Serve best available: first done variant, then converted path, then original
		if v := media.BestVariantForAccept(r.Header.Get("Accept")); v != nil && v.Path != "" {
			mimeType := codecMIMEType(v.Codec, media.Type)
			w.Header().Set("Content-Type", mimeType)
			w.Header().Set("Content-Disposition", validation.ContentDisposition(media.OriginalName, true))
			http.ServeFile(w, r, v.Path)
			return
		}

		// Fall back to legacy converted path or original
		servePath := media.ConvertedPath
		if servePath == "" {
			servePath = media.OriginalPath
		}

		if servePath == "" {
			http.Error(w, "Media not ready", http.StatusServiceUnavailable)
			return
		}

		mimeType := detectMIMEType(media)
		w.Header().Set("Content-Type", mimeType)
		w.Header().Set("Content-Disposition", validation.ContentDisposition(media.OriginalName, true))
		http.ServeFile(w, r, servePath)
	}
}

func detectMIMEType(media *domain.Media) string {
	switch media.Type {
	case domain.MediaTypeImage:
		return detectOriginalMIMEType(media)
	case domain.MediaTypeAudio:
		ext := strings.ToLower(filepath.Ext(media.OriginalName))
		switch ext {
		case ".mp3":
			return "audio/mpeg"
		case ".ogg":
			return "audio/ogg"
		case ".wav":
			return "audio/wav"
		case ".flac":
			return "audio/flac"
		default:
			return "audio/mpeg"
		}
	default: // video
		if media.Codec == domain.CodecAV1 {
			return "video/webm"
		}
		return "video/mp4"
	}
}

func detectOriginalMIMEType(media *domain.Media) string {
	ext := strings.ToLower(filepath.Ext(media.OriginalName))
	switch ext {
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".svg":
		return "image/svg+xml"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".mp4", ".m4v":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	case ".mov":
		return "video/quicktime"
	case ".avi":
		return "video/x-msvideo"
	case ".mkv":
		return "video/x-matroska"
	case ".mp3":
		return "audio/mpeg"
	case ".ogg", ".opus":
		return "audio/ogg"
	case ".wav":
		return "audio/wav"
	case ".flac":
		return "audio/flac"
	case ".aac":
		return "audio/aac"
	case ".m4a":
		return "audio/mp4"
	default:
		return "application/octet-stream"
	}
}

func codecMIMEType(codec domain.Codec, mediaType domain.MediaType) string {
	switch codec {
	case domain.CodecAV1:
		return "video/webm"
	case domain.CodecH264:
		return "video/mp4"
	case domain.CodecOpus:
		return "audio/ogg"
	default:
		if mediaType == domain.MediaTypeAudio {
			return "audio/mpeg"
		}
		return "video/mp4"
	}
}

func variantFilename(originalName string, codec domain.Codec) string {
	ext := filepath.Ext(originalName)
	base := strings.TrimSuffix(originalName, ext)
	switch codec {
	case domain.CodecAV1:
		return base + ".av1.webm"
	case domain.CodecH264:
		return base + ".h264.mp4"
	case domain.CodecOpus:
		return base + ".opus.ogg"
	default:
		return originalName
	}
}

func (h *Handlers) ServeThumb() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/v/")
		id = strings.TrimSuffix(id, "/thumb")
		id = strings.TrimSuffix(id, "/")

		media, err := h.mediaSvc.Get(id)
		if err != nil {
			http.Error(w, "Media not found", http.StatusNotFound)
			return
		}

		if media.ThumbPath == "" {
			http.Error(w, "Thumbnail not available", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "image/jpeg")
		http.ServeFile(w, r, media.ThumbPath)
	}
}
