package scanner

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOutputPath(t *testing.T) {
	got, err := OutputPath("/tmp/strm-out", "/电影/A/Movie.mkv", true)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join("/tmp/strm-out", "电影", "A", "Movie.mkv.strm")
	if got != want {
		t.Fatalf("OutputPath() = %s, want %s", got, want)
	}
	if _, err := OutputPath("/", "/x.mkv", true); err == nil {
		t.Fatal("expected / output root to be rejected")
	}
}

func TestNormalizeExtensionsAndSuffix(t *testing.T) {
	exts := NormalizeExtensions([]string{"mkv", ".MP4", "", "mkv"})
	if len(exts) != 2 || exts[0] != ".mkv" || exts[1] != ".mp4" {
		t.Fatalf("NormalizeExtensions() = %#v", exts)
	}
	if !HasConfiguredSuffix("Movie.MKV", exts) {
		t.Fatal("expected suffix match")
	}
	if HasConfiguredSuffix("Movie.txt", exts) {
		t.Fatal("did not expect suffix match")
	}
}

func TestMatchBlacklist(t *testing.T) {
	patterns := []string{"**/@eaDir/**", "*.tmp", "/private/**"}
	for _, p := range []string{"/media/@eaDir/file.jpg", "/movie/a.tmp", "/private/a.mkv"} {
		if !MatchBlacklist(patterns, p) {
			t.Fatalf("expected %s to match blacklist", p)
		}
	}
	if MatchBlacklist(patterns, "/movie/a.mkv") {
		t.Fatal("did not expect normal media file to match blacklist")
	}
}

func TestNeedsDownload(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.srt")
	if err := os.WriteFile(file, []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if !NeedsDownload(file, 4, time.Time{}, false) {
		t.Fatal("expected size mismatch to require download")
	}
	if err := os.Chtimes(file, now.Add(-time.Hour), now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if !NeedsDownload(file, 3, now, true) {
		t.Fatal("expected newer remote time to require download")
	}
	if NeedsDownload(file, 3, time.Time{}, false) {
		t.Fatal("did not expect matching size without time to require download")
	}
}
