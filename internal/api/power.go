package api

import (
	"net/http"
	"strconv"
	"time"

	"lapguard/internal/power"
	"lapguard/internal/storage"
)

type powerResponse struct {
	Timestamp time.Time           `json:"timestamp"`
	Source    power.Source        `json:"source"`
	Adapters  []power.Adapter     `json:"adapters"`
	Reason    string              `json:"reason,omitempty"`
	Watcher   power.WatcherStatus `json:"watcher"`
}

func (s *Server) handlePower(w http.ResponseWriter, _ *http.Request) {
	cfg := s.currentConfig()
	s.mu.RLock()
	watcher := s.watcher
	s.mu.RUnlock()

	var snap power.Snapshot
	var status power.WatcherStatus
	if watcher != nil {
		snap = watcher.Snapshot()
		status = watcher.Status()
	} else {
		snap = power.Scan(cfg.SysfsRoot)
		status = power.WatcherStatus{
			IntervalSeconds: cfg.PowerPoll.Seconds(),
			DebounceSeconds: cfg.PowerDebounce.Seconds(),
		}
	}
	if snap.Adapters == nil {
		snap.Adapters = []power.Adapter{}
	}
	s.writeJSON(w, http.StatusOK, powerResponse{
		Timestamp: snap.Timestamp,
		Source:    snap.Source,
		Adapters:  snap.Adapters,
		Reason:    snap.Reason,
		Watcher:   status,
	})
}

type eventsResponse struct {
	Events    []storage.Event `json:"events"`
	Available bool            `json:"available"`
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	store := s.events
	s.mu.RUnlock()

	q := r.URL.Query()
	limit := 50
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			s.writeError(w, http.StatusBadRequest, "invalid limit", nil)
			return
		}
		limit = n
	}
	eventType := q.Get("type")
	if eventType != "" && !power.ValidEventType(eventType) {
		s.writeError(w, http.StatusBadRequest, "invalid event type", nil)
		return
	}
	if store == nil {
		s.writeJSON(w, http.StatusOK, eventsResponse{Events: []storage.Event{}, Available: false})
		return
	}
	list, err := store.List(r.Context(), storage.ListOptions{Limit: limit, Type: eventType})
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "failed to read event log", err)
		return
	}
	if list == nil {
		list = []storage.Event{}
	}
	s.writeJSON(w, http.StatusOK, eventsResponse{Events: list, Available: true})
}
