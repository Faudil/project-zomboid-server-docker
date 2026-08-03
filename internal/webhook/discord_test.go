package webhook

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/faudil/project-zomboid-server-docker/internal/config"
)

// A nil DiscordWebhook (no webhook URL configured) must never panic.
func TestNotifyMethodsNilReceiver(t *testing.T) {
	var d *DiscordWebhook = nil
	d.NotifyStart()
	d.NotifyStop()
	d.NotifyCrash(errors.New("boom"))
}

func TestNewDiscordNilWithoutURL(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.DiscordURL = ""
	if got := NewDiscord(cfg); got != nil {
		t.Errorf("NewDiscord = %v, want nil", got)
	}
}

func TestNotifyStartDisabled(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.DiscordURL = "http://example.invalid/hook"
	cfg.DiscordStart = false

	// Must not attempt an HTTP request when the notify flag is off.
	d := NewDiscord(cfg)
	d.NotifyStart()
}

func TestSendPost(t *testing.T) {
	var gotPath, gotCT string
	var gotBody []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotCT = r.Header.Get("Content-Type")
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		gotBody = buf[:n]
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	cfg := config.DefaultConfig()
	cfg.DiscordURL = server.URL
	cfg.ServerName = "testworld"
	cfg.PublicName = "My Server"

	d := NewDiscord(cfg)
	if err := d.Send("Title", "Description", 0x57F287); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if gotPath != "/" {
		t.Errorf("path = %q", gotPath)
	}
	if gotCT != "application/json" {
		t.Errorf("content-type = %q", gotCT)
	}
	body := string(gotBody)
	if !strings.Contains(body, `"title":"Title"`) {
		t.Errorf("body missing title: %s", body)
	}
	if !strings.Contains(body, "testworld") {
		t.Errorf("body missing server footer: %s", body)
	}
}

func TestSendServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	cfg := config.DefaultConfig()
	cfg.DiscordURL = server.URL
	d := NewDiscord(cfg)
	if err := d.Send("t", "d", 0); err == nil {
		t.Fatal("Send should fail on 400")
	}
}
