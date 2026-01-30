package http

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/bnema/sharm/internal/adapter/http/templates"
	"github.com/bnema/sharm/internal/domain"
)

type MediaService interface {
	Upload(filename string, file *os.File, retentionDays int) (*domain.Media, error)
	Get(id string) (*domain.Media, error)
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

func (h *Handlers) UploadPage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		templates.Upload().Render(r.Context(), w)
	}
}

func (h *Handlers) Upload() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, int64(h.maxSizeMB)*1024*1024)

		if err := r.ParseMultipartForm(int64(h.maxSizeMB) * 1024 * 1024); err != nil {
			http.Error(w, "File too large", http.StatusRequestEntityTooLarge)
			return
		}

		file, header, err := r.FormFile("video")
		if err != nil {
			http.Error(w, "Invalid file upload", http.StatusBadRequest)
			return
		}
		defer file.Close()

		tmpFile, err := os.CreateTemp("", "upload-*.tmp")
		if err != nil {
			http.Error(w, "Failed to create temp file", http.StatusInternalServerError)
			return
		}
		defer tmpFile.Close()

		if _, err := io.Copy(tmpFile, file); err != nil {
			http.Error(w, "Failed to save file", http.StatusInternalServerError)
			return
		}

		retentionStr := r.FormValue("retention")
		retentionDays, err := strconv.Atoi(retentionStr)
		if err != nil {
			retentionDays = 7
		}

		media, err := h.mediaSvc.Upload(header.Filename, tmpFile, retentionDays)
		if err != nil {
			http.Error(w, "Failed to upload", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		templates.StatusPolling(media.ID).Render(r.Context(), w)
	}
}

func (h *Handlers) Status() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/status/")

		media, err := h.mediaSvc.Get(id)
		if err != nil {
			http.Error(w, "Media not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")

		switch media.Status {
		case domain.MediaStatusConverting:
			templates.StatusPolling(media.ID).Render(r.Context(), w)
		case domain.MediaStatusDone:
			shareURL := fmt.Sprintf("https://%s/v/%s", h.domain, media.ID)
			templates.StatusDone(media, shareURL).Render(r.Context(), w)
		case domain.MediaStatusFailed:
			templates.StatusFailed(media.ErrorMessage).Render(r.Context(), w)
		}
	}
}

func (h *Handlers) SharePage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/v/")
		id = strings.TrimSuffix(id, "/")

		media, err := h.mediaSvc.Get(id)
		if err != nil {
			http.Error(w, "Media not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		templates.Share(media, h.domain).Render(r.Context(), w)
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

		if media.Status != domain.MediaStatusDone {
			http.Error(w, "Media not ready", http.StatusServiceUnavailable)
			return
		}

		mimeType := "video/mp4"
		if media.Codec == domain.CodecAV1 {
			mimeType = "video/webm"
		}

		w.Header().Set("Content-Type", mimeType)
		w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%s", media.OriginalName))
		http.ServeFile(w, r, media.ConvertedPath)
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

		if media.Status != domain.MediaStatusDone || media.ThumbPath == "" {
			http.Error(w, "Thumbnail not available", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "image/jpeg")
		http.ServeFile(w, r, media.ThumbPath)
	}
}
