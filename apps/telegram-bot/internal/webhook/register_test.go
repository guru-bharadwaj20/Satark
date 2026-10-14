package webhook

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func fakeAPI(t *testing.T, check func(method string, form url.Values), reply string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/botTOKEN/") {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(body))
		check(strings.TrimPrefix(r.URL.Path, "/botTOKEN/"), form)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, reply)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestSetWebhook(t *testing.T) {
	srv := fakeAPI(t, func(method string, f url.Values) {
		if method != "setWebhook" {
			t.Errorf("method %s", method)
		}
		if f.Get("url") != "https://bot.example.in/tg" || f.Get("secret_token") != testSecret {
			t.Errorf("form %v", f)
		}
		if f.Get("allowed_updates") != `["message","callback_query"]` || f.Get("drop_pending_updates") != "true" {
			t.Errorf("form %v", f)
		}
	}, `{"ok":true,"result":true}`)
	c := &Client{Token: "TOKEN", APIBase: srv.URL}
	err := c.SetWebhook(context.Background(), SetWebhookParams{
		URL: "https://bot.example.in/tg", SecretToken: testSecret,
		AllowedUpdates: []string{"message", "callback_query"}, DropPendingUpdates: true,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSetWebhookValidation(t *testing.T) {
	c := &Client{Token: "TOKEN", APIBase: "http://unused"}
	if err := c.SetWebhook(context.Background(), SetWebhookParams{URL: "http://insecure", SecretToken: testSecret}); err == nil {
		t.Fatal("want error for http URL")
	}
	if err := c.SetWebhook(context.Background(), SetWebhookParams{URL: "https://ok.example", SecretToken: "bad secret"}); err == nil {
		t.Fatal("want error for bad secret")
	}
}

func TestAPIError(t *testing.T) {
	srv := fakeAPI(t, func(string, url.Values) {}, `{"ok":false,"error_code":401,"description":"Unauthorized"}`)
	c := &Client{Token: "TOKEN", APIBase: srv.URL}
	err := c.DeleteWebhook(context.Background(), false)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != 401 {
		t.Fatalf("got %v", err)
	}
}

func TestErrorDoesNotLeakToken(t *testing.T) {
	c := &Client{Token: "SUPERSECRETTOKEN", APIBase: "http://127.0.0.1:1"}
	err := c.DeleteWebhook(context.Background(), false)
	if err == nil || strings.Contains(err.Error(), "SUPERSECRETTOKEN") {
		t.Fatalf("error leaks token or is nil: %v", err)
	}
}

func TestGetWebhookInfo(t *testing.T) {
	srv := fakeAPI(t, func(string, url.Values) {},
		`{"ok":true,"result":{"url":"https://bot.example.in/tg","pending_update_count":3,"last_error_message":"timeout"}}`)
	c := &Client{Token: "TOKEN", APIBase: srv.URL}
	info, err := c.GetWebhookInfo(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.PendingUpdateCount != 3 || info.LastErrorMessage != "timeout" {
		t.Fatalf("info %+v", info)
	}
}

func TestParseEnv(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

	cfg, err := parseEnv(env(nil))
	if err != nil || cfg.Mode != ModePolling || cfg.ListenAddr != ":8082" {
		t.Fatalf("default: %+v %v", cfg, err)
	}

	cfg, err = parseEnv(env(map[string]string{
		"TELEGRAM_MODE":           "Webhook",
		"TELEGRAM_WEBHOOK_URL":    "https://bot.example.in/telegram/webhook",
		"TELEGRAM_WEBHOOK_SECRET": testSecret,
	}))
	if err != nil || cfg.Mode != ModeWebhook || cfg.Path != "/telegram/webhook" {
		t.Fatalf("webhook: %+v %v", cfg, err)
	}

	bad := []map[string]string{
		{"TELEGRAM_MODE": "carrier-pigeon"},
		{"TELEGRAM_MODE": "webhook", "TELEGRAM_WEBHOOK_URL": "http://x", "TELEGRAM_WEBHOOK_SECRET": testSecret},
		{"TELEGRAM_MODE": "webhook", "TELEGRAM_WEBHOOK_URL": "https://x"},
	}
	for _, m := range bad {
		if _, err := parseEnv(env(m)); err == nil {
			t.Errorf("want error for %v", m)
		}
	}
}
