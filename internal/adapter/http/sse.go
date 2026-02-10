package http

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/bnema/sharm/internal/adapter/http/templates"
	"github.com/bnema/sharm/internal/domain"
	"github.com/bnema/sharm/internal/service"
)

type SSEHandler struct {
	eventBus *service.EventBus
	mediaSvc MediaService
	domain   string
}

type renderedFragments struct {
	statusHTML string
	rowHTML    string
}

func NewSSEHandler(eventBus *service.EventBus, mediaSvc MediaService, domainName string) *SSEHandler {
	return &SSEHandler{
		eventBus: eventBus,
		mediaSvc: mediaSvc,
		domain:   domainName,
	}
}

// renderStatusHTML renders the status page fragment for a media item.
func (h *SSEHandler) renderStatusHTML(media *domain.Media) (string, error) {
	var buf bytes.Buffer
	var err error

	switch media.Status {
	case domain.MediaStatusDone:
		shareURL := fmt.Sprintf("https://%s/v/%s", h.domain, media.ID)
		err = templates.StatusDone(media, shareURL).Render(context.Background(), &buf)
	case domain.MediaStatusFailed:
		err = templates.StatusFailed(media.ErrorMessage).Render(context.Background(), &buf)
	default:
		err = templates.StatusPolling(media.ID).Render(context.Background(), &buf)
	}

	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

// renderRowHTML renders the inner content of a dashboard row for SSE innerHTML swap.
func (h *SSEHandler) renderRowHTML(media *domain.Media) (string, error) {
	var buf bytes.Buffer
	err := templates.DashboardRowContent(media, h.domain).Render(context.Background(), &buf)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

// sseWrite writes an SSE event, handling multi-line data correctly.
func sseWrite(w http.ResponseWriter, eventName, data string) {
	_, _ = fmt.Fprintf(w, "event: %s\n", eventName)
	for line := range strings.SplitSeq(data, "\n") {
		_, _ = fmt.Fprintf(w, "data: %s\n", line)
	}
	_, _ = fmt.Fprint(w, "\n")
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// sendAllEvents sends changed "status" and "row" SSE fragments for a media item.
func (h *SSEHandler) sendAllEvents(w http.ResponseWriter, media *domain.Media, previous *renderedFragments) (*renderedFragments, error) {
	statusHTML, err := h.renderStatusHTML(media)
	if err != nil {
		return nil, err
	}

	rowHTML, err := h.renderRowHTML(media)
	if err != nil {
		return nil, err
	}

	if previous == nil || previous.statusHTML != statusHTML {
		sseWrite(w, "status", statusHTML)
	}
	if previous == nil || previous.rowHTML != rowHTML {
		sseWrite(w, "row", rowHTML)
	}

	return &renderedFragments{
		statusHTML: statusHTML,
		rowHTML:    rowHTML,
	}, nil
}

func (h *SSEHandler) Events() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/events/")
		id = strings.TrimSuffix(id, "/")

		if id == "" {
			http.Error(w, "Missing media ID", http.StatusBadRequest)
			return
		}

		media, err := h.mediaSvc.Get(id)
		if err != nil {
			http.Error(w, "Media not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		// If already terminal, send final events and close
		if media.Status == domain.MediaStatusDone || media.Status == domain.MediaStatusFailed {
			_, _ = h.sendAllEvents(w, media, nil)
			return
		}

		// Send current state
		state, _ := h.sendAllEvents(w, media, nil)

		// Subscribe to events
		ch := h.eventBus.Subscribe(id)
		defer h.eventBus.Unsubscribe(id, ch)

		ctx := r.Context()
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-ch:
				if !ok {
					return
				}
				// Re-fetch media to get full state for rendering
				media, err := h.mediaSvc.Get(id)
				if err != nil {
					return
				}
				state, _ = h.sendAllEvents(w, media, state)

				// Close on terminal states
				if event.Status == string(domain.MediaStatusDone) || event.Status == string(domain.MediaStatusFailed) {
					return
				}
			}
		}
	}
}
