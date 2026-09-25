package library

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseTitleReleasesAndRecordings(t *testing.T) {
	cases := []struct {
		in    string
		title string
		year  int
	}{
		{"The.Neon.Archive.2021.1080p.WEB-DL.x264", "The Neon Archive", 2021},
		{"the-neon-archive-2021-4k-hdr-hevc", "the neon archive", 2021},
		{"Spider-Man.No.Way.Home.2021.1080p", "Spider-Man No Way Home", 2021},
		{"The Neon Archive (2021) [Extended]", "The Neon Archive", 2021},
		{"2026-09-25 13-31-03", "2026 09 25 13 31 03", 2026},
		{"2026-09-24_17-49-31", "2026 09 24 17 49 31", 2026},
		{"no numbers at all", "no numbers at all", 0},
		{"We.1994.a.very.old.film", "We a very old film", 1994},
		{"single word", "single word", 0},
	}
	for _, c := range cases {
		title, year := parseTitle(c.in)
		if title != c.title || year != c.year {
			t.Errorf("parseTitle(%q) = (%q, %d), want (%q, %d)", c.in, title, year, c.title, c.year)
		}
	}
}

func TestIDStability(t *testing.T) {
	a := IDForPath("The.Neon.Archive.2021.mp4")
	b := IDForPath("The.Neon.Archive.2021.mp4")
	c := IDForPath("Other.mp4")
	if a != b {
		t.Error("same path, different ids")
	}
	if a == c {
		t.Error("different paths, same id")
	}
	if len(a) != 16 {
		t.Errorf("id length = %d, want 16", len(a))
	}
}

func TestScanCollectsVideosAndSkipsTheRest(t *testing.T) {
	dir := t.TempDir()
	files := map[string]int64{
		"The.Neon.Archive.2021.1080p.mp4": 100,
		"2026-09-25 13-31-03.mov":         200,
		"Notes/readme.txt":                50,
		"Trailers/Clip.webm":              300,
	}
	for name, size := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	lib, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(lib.Items) != 3 {
		t.Fatalf("got %d items, want 3", len(lib.Items))
	}
	// Notes/ holds no video, so only Trailers/ counts.
	if len(lib.Folders) != 1 || lib.Folders[0].Name != "Trailers" || lib.Folders[0].Count != 1 {
		t.Fatalf("folders = %#v, want one Trailers with one video", lib.Folders)
	}

	byTitle := map[string]Item{}
	for _, item := range lib.Items {
		byTitle[item.Title] = item
	}
	if item, ok := byTitle["The Neon Archive"]; !ok {
		t.Errorf("release title missing: %#v", lib.Items)
	} else {
		if item.Year != 2021 {
			t.Errorf("year = %d", item.Year)
		}
		if item.Mime != "video/mp4" {
			t.Errorf("mime = %q", item.Mime)
		}
		if item.Size != 100 {
			t.Errorf("size = %d", item.Size)
		}
	}
	if item, ok := byTitle["2026 09 25 13 31 03"]; !ok {
		t.Errorf("recording title missing")
	} else if item.Folder != "" {
		t.Errorf("folder = %q, want root", item.Folder)
	}
	if _, ok := byTitle["Clip"]; !ok {
		t.Errorf("nested video missing")
	}
}

func TestScanKeepsStableIDsAcrossScans(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "A.mp4")
	if err := os.WriteFile(path, make([]byte, 10), 0o644); err != nil {
		t.Fatal(err)
	}
	first, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Millisecond)
	second, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}
	if first.Items[0].ID != second.Items[0].ID {
		t.Error("ids changed between scans")
	}
	if first.Items[0].RelPath != second.Items[0].RelPath {
		t.Error("rel path changed between scans")
	}
}

func TestPosterNextToVideo(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "A.mp4")
	poster := filepath.Join(dir, "A.jpg")
	if err := os.WriteFile(video, make([]byte, 10), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := posterFor(video); got != "" {
		t.Errorf("no poster yet, got %q", got)
	}
	if err := os.WriteFile(poster, make([]byte, 10), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := posterFor(video); got != poster {
		t.Errorf("posterFor = %q, want %q", got, poster)
	}
}

func TestScanEmptyAndMissingDirs(t *testing.T) {
	if _, err := Scan(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Error("expected an error for a missing dir")
	}
	if _, err := Scan(filepath.Join(t.TempDir(), "file.txt")); err == nil {
		t.Error("expected an error for a file, not a dir")
	}
	lib, err := Scan(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(lib.Items) != 0 || len(lib.Folders) != 0 {
		t.Errorf("empty dir should produce an empty shelf")
	}
}
