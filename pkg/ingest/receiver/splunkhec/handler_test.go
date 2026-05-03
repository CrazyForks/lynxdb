package splunkhec

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lynxbase/lynxdb/pkg/event"
)

func TestHandler_EventRequiresSplunkToken(t *testing.T) {
	h := NewHandler(Config{Auth: AuthConfig{Enabled: true}}, func(context.Context, []*event.Event) error { return nil })
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/services/collector/event", strings.NewReader(`{"event":"hello"}`))

	h.HandleEvent(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
}

func TestHandler_Raw_LinePerEvent_AllParsed(t *testing.T) {
	var submitted int
	h := NewHandler(Config{Auth: AuthConfig{Enabled: true}}, func(_ context.Context, events []*event.Event) error {
		submitted += len(events)
		return nil
	})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/services/collector/raw?source=raw-src", strings.NewReader("one\ntwo\n"))
	req.Header.Set("Authorization", "Splunk token")

	h.HandleRaw(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if submitted != 2 {
		t.Fatalf("submitted = %d, want 2", submitted)
	}
}

func TestHandler_EventSubmitError_UsesConfiguredResponder(t *testing.T) {
	submitErr := errors.New("backpressure")
	h := NewHandler(Config{
		Auth: AuthConfig{Enabled: true},
		RespondIngestError: func(w http.ResponseWriter, err error) {
			if !errors.Is(err, submitErr) {
				t.Fatalf("err = %v, want submitErr", err)
			}
			w.Header().Set("Retry-After", "5")
			respond(w, http.StatusServiceUnavailable, "custom", 9)
		},
	}, func(context.Context, []*event.Event) error {
		return submitErr
	})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/services/collector/event", strings.NewReader(`{"event":"hello"}`))
	req.Header.Set("Authorization", "Splunk token")

	h.HandleEvent(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rr.Code)
	}
	if got := rr.Header().Get("Retry-After"); got != "5" {
		t.Fatalf("Retry-After = %q, want 5", got)
	}
}

func TestHandler_Health_Returns200(t *testing.T) {
	h := NewHandler(Config{}, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/services/collector/health", nil)

	h.HandleHealth(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
}
