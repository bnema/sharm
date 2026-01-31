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
	"github.com/bnema/sharm/internal/domain"
	"github.com/bnema/sharm/internal/infrastructure/logger"
)

type MediaService interface {
	Upload(filename string, file *os.File, retentionDays int, mediaType domain.MediaType, codecs []domain.Codec, fps int) (*domain.Media, error)
	Get(id string) (*domain.Media, error)
	ListAll() ([]*domain.Media, error)
	Delete(id string) error
}

type Handlers struct {
	mediaSvc  MediaService
	domain    string
	maxSizeMB int
}

func NewHandlers(mediaSvc MediaService, domain string, maxSizeMB int) *Handlers {
	return &Handlers{
		mediaSvc:  mediaSvc,
		domain:    domain,
		maxSizeMB: maxSizeMB,
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
		_ = templates.Dashboard(media, h.domain).Render(r.Context(), w)
	}
}

func (h *Handlers) UploadPage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = templates.Upload().Render(r.Context(), w)
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

		tmpFile, err := os.CreateTemp("", "upload-*.tmp")
		if err != nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusInternalServerError)
			_ = templates.ErrorInline("Failed to process upload").Render(r.Context(), w)
			return
		}
		defer tmpFile.Close() //nolint:errcheck

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
				_ = templates.ErrorPage("404", "Media not found").Render(r.Context(), w)
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

		_ = templates.StatusPage(media.ID).Render(r.Context(), w)
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
		case "raw":
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
			_ = templates.ErrorPage("404", "Media not found").Render(r.Context(), w)
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
		w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", media.OriginalName))
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
		w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", variantFilename(media.OriginalName, codec)))
		http.ServeFile(w, r, v.Path)
	}
}

func (h *Handlers) ServeRaw() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/v/")
		id = strings.TrimSuffix(id, "/raw")
		id = strings.TrimSuffix(id, "/")

		media, err := h.mediaSvc.Get(id)
		if err != nil {
			http.Error(w, "Media not found", http.StatusNotFound)
			return
		}

		// Serve best available: first done variant, then converted path, then original
		if v := media.BestVariant(); v != nil && v.Path != "" {
			mimeType := codecMIMEType(v.Codec, media.Type)
			w.Header().Set("Content-Type", mimeType)
			w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", media.OriginalName))
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
		w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", media.OriginalName))
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
