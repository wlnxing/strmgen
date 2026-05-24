package scanner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"strm/internal/db"
	"strm/internal/models"
	"strm/internal/openlist"
)

func TestServiceRunOpenListFlow(t *testing.T) {
	var loginCount atomic.Int64
	var authorizedCount atomic.Int64
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/auth/login/hash":
			loginCount.Add(1)
			var req map[string]string
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req["username"] != "user" || req["password"] != openlist.HashPassword("secret") {
				t.Fatalf("unexpected login body: %#v", req)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": map[string]any{"token": "token-1"}})
		case "/api/fs/list":
			if r.Header.Get("Authorization") != "token-1" {
				t.Fatalf("list used wrong token: %q", r.Header.Get("Authorization"))
			}
			authorizedCount.Add(1)
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req["path"] == "/media" {
				_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": map[string]any{"content": []map[string]any{
					{"name": "Movie.mkv", "size": 10, "is_dir": false, "sign": "sig-video", "modified": "2026-05-23T10:00:00Z"},
					{"name": "Movie.srt", "size": 8, "is_dir": false, "sign": "sig-sub", "modified": "2026-05-23T10:01:00Z"},
				}}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": map[string]any{"content": []any{}}})
		case "/d/media/Movie.srt":
			if r.URL.Query().Get("sign") != "sig-sub" {
				t.Fatalf("download missing sign: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte("subtitle"))
		default:
			if strings.HasPrefix(r.URL.Path, "/d/") {
				_, _ = w.Write([]byte("download"))
				return
			}
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	store, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveOpenListSettings(context.Background(), models.OpenListSettings{BaseURL: serverURL, Username: "user", PasswordHash: openlist.HashPassword("secret")}); err != nil {
		t.Fatal(err)
	}

	out := t.TempDir()
	task := DefaultTask()
	task.OutputRoot = out
	task.ScanDirs = []string{"/media"}
	task.StrmExtensions = []string{".mkv"}
	task.DownloadExtensions = []string{".srt"}
	task.DownloadConcurrency = 1
	task.DownloadTimeoutSeconds = 5

	svc := &Service{Store: store, HTTPClient: server.Client()}
	stats, err := svc.Run(context.Background(), 0, task)
	if err != nil {
		t.Fatal(err)
	}
	if loginCount.Load() != 1 {
		t.Fatalf("login count = %d, want 1", loginCount.Load())
	}
	if authorizedCount.Load() == 0 {
		t.Fatal("expected authorized OpenList requests")
	}
	if stats.StrmWritten != 1 || stats.Downloads != 1 {
		t.Fatalf("unexpected stats: %#v", stats)
	}
	strmPath := filepath.Join(out, "media", "Movie.mkv.strm")
	content, err := os.ReadFile(strmPath)
	if err != nil {
		t.Fatal(err)
	}
	wantURL := serverURL + "/d/media/Movie.mkv?sign=sig-video\n"
	if string(content) != wantURL {
		t.Fatalf("strm content = %q, want %q", content, wantURL)
	}
	downloaded, err := os.ReadFile(filepath.Join(out, "media", "Movie.srt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(downloaded) != "subtitle" {
		t.Fatalf("downloaded = %q", downloaded)
	}
	info, err := os.Stat(filepath.Join(out, "media", "Movie.srt"))
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().After(time.Date(2026, 5, 23, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("mtime was not updated: %s", info.ModTime())
	}
}
