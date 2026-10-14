package api

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/guru-bharadwaj20/satark/apps/reputation/internal/identifier"
	"github.com/guru-bharadwaj20/satark/apps/reputation/internal/store"
)

// Delisting flow (see docs/sops/SOP-07-delisting-requests.md):
//
//	POST /v1/delisting-requests                  public: owner asks for review
//	GET  /v1/admin/delisting-requests?status=    reviewer: list requests
//	POST /v1/admin/delisting-requests/{id}       reviewer: approve or reject
//
// Approval deletes every existing report about the subject; new reports
// after that start from zero and must again pass the threshold.

func (s *Server) registerDelisting() {
	s.mux.HandleFunc("POST /v1/delisting-requests", s.createDelisting)
	s.mux.HandleFunc("GET /v1/admin/delisting-requests", s.admin(s.listDelistings))
	s.mux.HandleFunc("POST /v1/admin/delisting-requests/{id}", s.admin(s.decideDelisting))
}

type delistingRequest struct {
	Kind    string `json:"kind"`
	Value   string `json:"value"`
	Reason  string `json:"reason"`
	Contact string `json:"contact"`
}

func (s *Server) createDelisting(w http.ResponseWriter, r *http.Request) {
	var req delistingRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	kind, norm, ok := s.parse(w, req.Kind, req.Value)
	if !ok {
		return
	}
	req.Reason = strings.TrimSpace(req.Reason)
	if len(req.Reason) < 10 || len(req.Reason) > 2000 {
		writeErr(w, http.StatusBadRequest, "reason must be 10-2000 characters")
		return
	}
	if len(req.Contact) > 200 {
		writeErr(w, http.StatusBadRequest, "contact too long")
		return
	}
	d, err := s.cfg.Store.CreateDelisting(r.Context(), store.Delisting{
		Kind:          string(kind),
		SubjectHash:   s.cfg.Hasher.Subject(kind, norm),
		SubjectMasked: identifier.Mask(kind, norm),
		Reason:        req.Reason,
		Contact:       strings.TrimSpace(req.Contact),
		CreatedAt:     s.cfg.Now().UTC(),
	})
	if err != nil {
		s.internal(w, "create delisting", err)
		return
	}
	s.cfg.Metrics.Inc("satark_reputation_delisting_requests_total", "kind", string(kind))
	writeJSON(w, http.StatusAccepted, map[string]any{
		"id": d.ID, "status": d.Status,
		"message": "Thank you. A reviewer will look at this request, usually within 7 days.",
	})
}

func (s *Server) listDelistings(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	if status != "" && status != store.DelistPending && status != store.DelistApproved && status != store.DelistRejected {
		writeErr(w, http.StatusBadRequest, "unknown status")
		return
	}
	list, err := s.cfg.Store.ListDelistings(r.Context(), status)
	if err != nil {
		s.internal(w, "list delistings", err)
		return
	}
	if list == nil {
		list = []store.Delisting{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"requests": list})
}

type decision struct {
	Decision string `json:"decision"` // "approve" | "reject"
	Note     string `json:"note"`
}

func (s *Server) decideDelisting(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req decision
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	status := map[string]string{"approve": store.DelistApproved, "reject": store.DelistRejected}[req.Decision]
	if status == "" {
		writeErr(w, http.StatusBadRequest, "decision must be approve or reject")
		return
	}
	d, err := s.cfg.Store.DecideDelisting(r.Context(), id, status, req.Note, s.cfg.Now().UTC())
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "no pending request with that id")
		return
	}
	if err != nil {
		s.internal(w, "decide delisting", err)
		return
	}
	removed := 0
	if status == store.DelistApproved {
		if removed, err = s.cfg.Store.DeleteSubject(r.Context(), d.Kind, d.SubjectHash); err != nil {
			s.internal(w, "delete subject", err)
			return
		}
	}
	s.cfg.Logger.Info("delisting decided", "id", d.ID, "status", status, "reports_removed", removed)
	s.cfg.Metrics.Inc("satark_reputation_delisting_decisions_total", "decision", status)
	writeJSON(w, http.StatusOK, map[string]any{"request": d, "reports_removed": removed})
}

// admin guards reviewer endpoints with a bearer token. If no token is
// configured the endpoints are disabled entirely.
func (s *Server) admin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		want := s.cfg.AdminToken
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if want == "" || subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
			writeErr(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next(w, r)
	}
}
