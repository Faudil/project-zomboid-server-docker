package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/faudil/project-zomboid-server-docker/internal/config"
	"github.com/faudil/project-zomboid-server-docker/internal/webhook"
)

type fakeServer struct {
	stopCalled bool
	stopErr    error
}

func (f *fakeServer) Start() error { return nil }
func (f *fakeServer) Wait() error  { return nil }
func (f *fakeServer) Stop() error {
	f.stopCalled = true
	return f.stopErr
}

type fakeBackup struct {
	runCalled bool
}

func (f *fakeBackup) Run() { f.runCalled = true }

type testDiscordServer struct {
	mu   sync.Mutex
	body bytes.Buffer
	srv  *httptest.Server
}

func newTestDiscordServer(t *testing.T) *testDiscordServer {
	t.Helper()
	ts := &testDiscordServer{}
	ts.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := new(bytes.Buffer)
		if _, err := buf.ReadFrom(r.Body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		ts.mu.Lock()
		ts.body.Write(buf.Bytes())
		ts.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(ts.srv.Close)
	return ts
}

func (ts *testDiscordServer) assertBodyContains(t *testing.T, substr string) {
	t.Helper()
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if !strings.Contains(ts.body.String(), substr) {
		t.Errorf("discord body missing %q: %s", substr, ts.body.String())
	}
}

func TestUpdateReason(t *testing.T) {
	cases := []struct {
		name string
		mods []string
		game bool
		want string
	}{
		{"mods only", []string{"111"}, false, "workshop mod update(s): 111"},
		{"game only", nil, true, "a new game build"},
		{"both", []string{"111", "222"}, true, "a new game build and workshop mod update(s): 111, 222"},
		{"none", nil, false, "unknown change"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := updateReason(tc.mods, tc.game); got != tc.want {
				t.Errorf("updateReason = %q, want %q", got, tc.want)
			}
		})
	}
}

// captureExit replaces the process exit with a recording non-returning
// substitute, simulating real os.Exit semantics (callers must not continue).
// It returns the goroutine to wait on and a pointer to the recorded code.
func captureExit(t *testing.T) (chan struct{}, *int) {
	t.Helper()
	exitCode := -1
	done := make(chan struct{})
	origExit := exitProcess
	exitProcess = func(code int) {
		exitCode = code
		close(done)
		runtime.Goexit()
	}
	t.Cleanup(func() { exitProcess = origExit })
	return done, &exitCode
}

func TestRestartForUpdatesGraceful(t *testing.T) {
	srv := &fakeServer{}
	bk := &fakeBackup{}
	done, exitCode := captureExit(t)

	cfg := config.DefaultConfig()
	cfg.AutoUpdateWaitEmpty = false
	cfg.AutoUpdateAnnounce = 0

	go restartForUpdates(cfg, []string{"111"}, true, srv, bk, nil)
	<-done

	if *exitCode != 0 {
		t.Errorf("exit code = %d, want 0", *exitCode)
	}
	if !srv.stopCalled {
		t.Error("server Stop not called")
	}
	if !bk.runCalled {
		t.Error("final backup not run")
	}
}

func TestRestartForUpdatesStopErrorExitsNonZero(t *testing.T) {
	srv := &fakeServer{stopErr: errTest}
	done, exitCode := captureExit(t)

	cfg := config.DefaultConfig()
	cfg.AutoUpdateWaitEmpty = false
	cfg.AutoUpdateAnnounce = 0

	go restartForUpdates(cfg, []string{"111"}, false, srv, &fakeBackup{}, nil)
	<-done

	if *exitCode != 1 {
		t.Errorf("exit code = %d, want 1", *exitCode)
	}
}

func TestRestartForUpdatesNotifiesDiscord(t *testing.T) {
	ts := newTestDiscordServer(t)

	cfg := config.DefaultConfig()
	cfg.DiscordURL = ts.srv.URL
	cfg.DiscordUpdate = true
	cfg.PublicName = "Test Server"
	cfg.AutoUpdateWaitEmpty = false
	cfg.AutoUpdateAnnounce = 0

	done, exitCode := captureExit(t)

	go restartForUpdates(cfg, []string{"111"}, true, &fakeServer{}, &fakeBackup{}, webhook.NewDiscord(cfg))
	<-done

	if *exitCode != 0 {
		t.Errorf("exit code = %d, want 0", *exitCode)
	}
	ts.assertBodyContains(t, "Restarting for Updates")
	ts.assertBodyContains(t, "workshop mod")
}

// noopUpdater sets the RCON seams to harmless no-ops for waitForEmpty tests.
func noopUpdater(t *testing.T) {
	t.Helper()
	origCount := rconPlayerCount
	origBroadcast := rconBroadcast
	origSleep := sleep
	origPoll := emptyPoll
	origNow := now
	rconBroadcast = func(*config.ServerConfig, string) error { return nil }
	sleep = func(time.Duration) {}
	emptyPoll = 0
	t.Cleanup(func() {
		rconPlayerCount = origCount
		rconBroadcast = origBroadcast
		sleep = origSleep
		emptyPoll = origPoll
		now = origNow
	})
}

func TestWaitForEmptyAlreadyEmpty(t *testing.T) {
	noopUpdater(t)
	rconPlayerCount = func(*config.ServerConfig) (int, error) { return 0, nil }

	waitForEmpty(config.DefaultConfig(), "test")
}

func TestWaitForEmptyRCONUnreachableGivesUp(t *testing.T) {
	noopUpdater(t)
	rconPlayerCount = func(*config.ServerConfig) (int, error) { return 0, errTest }

	waitForEmpty(config.DefaultConfig(), "test")
}

func TestWaitForEmptyGivesUpAtDeadline(t *testing.T) {
	noopUpdater(t)
	rconPlayerCount = func(*config.ServerConfig) (int, error) { return 3, nil }

	cfg := config.DefaultConfig()
	cfg.AutoUpdateWaitMax = 2

	start := time.Now()
	calls := 0
	now = func() time.Time {
		calls++
		// Two calls precede the loop's deadline check (lastNotice, deadline);
		// from the third call on report a time past the wait max.
		if calls > 2 {
			return start.Add(3 * time.Hour)
		}
		return start
	}

	waitForEmpty(cfg, "test")
}

type testErr struct{}

func (e *testErr) Error() string { return "test error" }

var errTest = &testErr{}
