package server

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/guru-bharadwaj20/satark/apps/gateway-api/internal/api"
	"github.com/guru-bharadwaj20/satark/apps/gateway-api/internal/config"
)

type fakeScanner struct {
	gotText api.TextScanRequest
	gotQR   api.QRScanRequest
	err     error
}

func (f *fakeScanner) ScanText(_ context.Context, req api.TextScanRequest) (api.ScanResponse, error) {
	f.gotText = req
	return api.ScanResponse{ScanID: "t1", Verdict: api.VerdictSuspicious, Lang: req.Lang}, f.err
}

func (f *fakeScanner) ScanQR(_ context.Context, req api.QRScanRequest) (api.ScanResponse, error) {
	f.gotQR = req
	return api.ScanResponse{ScanID: "q1", Verdict: api.VerdictLikelyScam, Lang: req.Lang}, f.err
}

var testCfg = config.Config{MaxTextChars: 50, MaxImageBytes: 1024}

// pngHeader is enough for http.DetectContentType to report image/png.
var pngHeader = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")

func doText(t *testing.T, h http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/scan/text", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func doQR(t *testing.T, h http.Handler, img []byte, fields map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if img != nil {
		fw, _ := mw.CreateFormFile("image", "qr.png")
		_, _ = fw.Write(img)
	}
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/v1/scan/qr", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestScanTextOK(t *testing.T) {
	fs := &fakeScanner{}
	h := newTestServer(t, testCfg).WithScanner(fs).Handler()
	rec := doText(t, h, `{"text":"  Share your OTP  ","lang":"hi"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d: %s", rec.Code, rec.Body)
	}
	if fs.gotText.Text != "Share your OTP" || fs.gotText.Lang != "hi" || fs.gotText.SenderType != api.SenderUnknown {
		t.Fatalf("unexpected request passed to scanner: %+v", fs.gotText)
	}
	var resp api.ScanResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || resp.ScanID != "t1" {
		t.Fatalf("bad response %s (%v)", rec.Body, err)
	}
}

func TestScanTextValidation(t *testing.T) {
	h := newTestServer(t, testCfg).WithScanner(&fakeScanner{}).Handler()
	cases := map[string]struct {
		body string
		want int
	}{
		"not json":      {`hello`, http.StatusBadRequest},
		"empty":         {`{"text":"   "}`, http.StatusBadRequest},
		"unknown field": {`{"text":"x","foo":1}`, http.StatusBadRequest},
		"bad lang":      {`{"text":"x","lang":"fr"}`, http.StatusBadRequest},
		"bad sender":    {`{"text":"x","sender_type":"boss"}`, http.StatusBadRequest},
		"too long":      {`{"text":"` + strings.Repeat("a", 51) + `"}`, http.StatusRequestEntityTooLarge},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if rec := doText(t, h, tc.body); rec.Code != tc.want {
				t.Fatalf("got %d want %d: %s", rec.Code, tc.want, rec.Body)
			}
		})
	}
}

func TestScanTextWrongContentType(t *testing.T) {
	h := newTestServer(t, testCfg).WithScanner(&fakeScanner{}).Handler()
	req := httptest.NewRequest(http.MethodPost, "/v1/scan/text", strings.NewReader(`{"text":"x"}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestScanUnavailable(t *testing.T) {
	h := newTestServer(t, testCfg).WithScanner(&fakeScanner{err: ErrUnavailable}).Handler()
	if rec := doText(t, h, `{"text":"x"}`); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d", rec.Code)
	}
	h = newTestServer(t, testCfg).Handler() // no scanner configured
	if rec := doText(t, h, `{"text":"x"}`); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestScanQROK(t *testing.T) {
	fs := &fakeScanner{}
	h := newTestServer(t, testCfg).WithScanner(fs).Handler()
	rec := doQR(t, h, pngHeader, map[string]string{"lang": "kn", "context": "scan to receive"})
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d: %s", rec.Code, rec.Body)
	}
	if fs.gotQR.Lang != "kn" || fs.gotQR.Context != "scan to receive" || !bytes.Equal(fs.gotQR.Image, pngHeader) {
		t.Fatalf("unexpected request passed to scanner: %+v", fs.gotQR)
	}
}

func TestScanQRValidation(t *testing.T) {
	h := newTestServer(t, testCfg).WithScanner(&fakeScanner{}).Handler()
	if rec := doQR(t, h, nil, nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing image: got %d", rec.Code)
	}
	if rec := doQR(t, h, []byte("plain text"), nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("non-image: got %d", rec.Code)
	}
	if rec := doQR(t, h, pngHeader, map[string]string{"lang": "xx"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad lang: got %d", rec.Code)
	}
	big := append(append([]byte{}, pngHeader...), make([]byte, 2048)...)
	if rec := doQR(t, h, big, nil); rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("too large: got %d", rec.Code)
	}
}

func TestWrongMethodRejected(t *testing.T) {
	h := newTestServer(t, testCfg).WithScanner(&fakeScanner{}).Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/scan/text", nil))
	// The JSON catch-all answers 404 for any unregistered method+path.
	if rec.Code != http.StatusNotFound {
		t.Fatalf("got %d", rec.Code)
	}
}
