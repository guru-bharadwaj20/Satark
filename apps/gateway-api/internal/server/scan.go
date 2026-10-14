package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/guru-bharadwaj20/satark/apps/gateway-api/internal/api"
)

// Scanner runs the analysis behind the scan endpoints.
type Scanner interface {
	ScanText(ctx context.Context, req api.TextScanRequest) (api.ScanResponse, error)
	ScanQR(ctx context.Context, req api.QRScanRequest) (api.ScanResponse, error)
}

// ErrUnavailable is returned by a Scanner when no analyzer answered in time.
var ErrUnavailable = errors.New("analyzers unavailable")

// ErrBadImage is returned by a Scanner when the image could not be read.
var ErrBadImage = errors.New("image could not be read")

// WithScanner sets the Scanner used by the scan endpoints.
func (s *Server) WithScanner(sc Scanner) *Server {
	s.scanner = sc
	return s
}

func (s *Server) scanText(w http.ResponseWriter, r *http.Request) {
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "use application/json")
		return
	}
	// 4 bytes per rune is the UTF-8 worst case; add room for the JSON envelope.
	r.Body = http.MaxBytesReader(w, r.Body, int64(s.cfg.MaxTextChars)*4+4096)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var req api.TextScanRequest
	if err := dec.Decode(&req); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeError(w, http.StatusRequestEntityTooLarge, "too_large", "text is too long")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid_request", "body must be a JSON object with a text field")
		return
	}
	if err := normalizeText(&req, s.cfg.MaxTextChars); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, errTooLong) {
			status = http.StatusRequestEntityTooLarge
		}
		writeError(w, status, "invalid_request", err.Error())
		return
	}
	resp, err := s.scanner.ScanText(r.Context(), req)
	s.respond(w, resp, err)
}

func (s *Server) scanQR(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxImageBytes+64*1024)
	if err := r.ParseMultipartForm(s.cfg.MaxImageBytes); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeError(w, http.StatusRequestEntityTooLarge, "too_large", "image is too large")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid_request", "body must be multipart/form-data with an image field")
		return
	}
	defer func() { _ = r.MultipartForm.RemoveAll() }()
	file, _, err := r.FormFile("image")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing image field")
		return
	}
	defer file.Close()
	img, err := io.ReadAll(io.LimitReader(file, s.cfg.MaxImageBytes+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "could not read image")
		return
	}
	if int64(len(img)) > s.cfg.MaxImageBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "too_large", "image is too large")
		return
	}
	if len(img) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "image is empty")
		return
	}
	switch http.DetectContentType(img) {
	case "image/png", "image/jpeg", "image/webp", "image/gif", "image/bmp":
	default:
		writeError(w, http.StatusBadRequest, "invalid_request", "image must be PNG, JPEG or WebP")
		return
	}
	lang := r.FormValue("lang")
	if lang == "" {
		lang = api.LangEnglish
	}
	if !api.ValidLang(lang) {
		writeError(w, http.StatusBadRequest, "invalid_request", "lang must be en, hi or kn")
		return
	}
	caption := r.FormValue("context")
	if utf8.RuneCountInString(caption) > 2000 {
		caption = string([]rune(caption)[:2000])
	}
	resp, err := s.scanner.ScanQR(r.Context(), api.QRScanRequest{Image: img, Lang: lang, Context: caption})
	s.respond(w, resp, err)
}

func (s *Server) respond(w http.ResponseWriter, resp api.ScanResponse, err error) {
	if err != nil {
		if errors.Is(err, ErrBadImage) {
			writeError(w, http.StatusBadRequest, "invalid_image", "the image could not be read")
			return
		}
		if errors.Is(err, ErrUnavailable) || errors.Is(err, context.DeadlineExceeded) {
			writeError(w, http.StatusServiceUnavailable, "unavailable", "analysis is temporarily unavailable, please retry")
			return
		}
		if errors.Is(err, context.Canceled) {
			return // client went away
		}
		s.log.Error("scan failed", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

var errTooLong = errors.New("text is too long")

func normalizeText(req *api.TextScanRequest, maxChars int) error {
	if !utf8.ValidString(req.Text) {
		return errors.New("text must be valid UTF-8")
	}
	req.Text = strings.TrimSpace(req.Text)
	if req.Text == "" {
		return errors.New("text must not be empty")
	}
	if utf8.RuneCountInString(req.Text) > maxChars {
		return errTooLong
	}
	if req.Lang == "" {
		req.Lang = api.LangEnglish
	}
	if !api.ValidLang(req.Lang) {
		return errors.New("lang must be en, hi or kn")
	}
	if req.SenderType == "" {
		req.SenderType = api.SenderUnknown
	}
	if !api.ValidSender(req.SenderType) {
		return errors.New("sender_type is not supported")
	}
	return nil
}
