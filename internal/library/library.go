// Package library scans a folder of videos and turns filenames into a
// shelf: stable ids, readable titles, years, durations and poster art.
package library

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

// Video extensions that both browsers and DLNA renderers handle well enough.
var videoExtensions = map[string]string{
	".mp4":  "video/mp4",
	".m4v":  "video/mp4",
	".mov":  "video/quicktime",
	".mkv":  "video/x-matroska",
	".webm": "video/webm",
	".avi":  "video/x-msvideo",
	".ts":   "video/mp2t",
	".m2ts": "video/mp2t",
	".mpg":  "video/mpeg",
	".mpeg": "video/mpeg",
}

// Image extensions accepted as cover art sitting next to a video.
var posterExtensions = []string{".jpg", ".jpeg", ".png", ".webp"}

// Item is one thing worth watching.
type Item struct {
	ID        string  `json:"id"`
	Title     string  `json:"title"`
	RelPath   string  `json:"relPath"`
	Folder    string  `json:"folder"`
	Size      int64   `json:"size"`
	ModTime   string  `json:"modTime"`
	Mime      string  `json:"mime"`
	Duration  float64 `json:"duration"`
	Width     int     `json:"width,omitempty"`
	Height    int     `json:"height,omitempty"`
	Year      int     `json:"year,omitempty"`
	HasPoster bool    `json:"hasPoster"`
}

// Folder is one directory on the shelf, with its stable id for DLNA.
type Folder struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	RelPath string `json:"relPath"`
	Count   int    `json:"count"`
}

// Library is a scanned view of the movies directory.
type Library struct {
	Root      string    `json:"root"`
	Items     []Item    `json:"items"`
	Folders   []Folder  `json:"folders"`
	ScannedAt time.Time `json:"scannedAt"`
}

// IDForPath derives the id used in URLs from a path relative to the root.
// Content-addressed rather than sequential, so URLs survive rescans.
func IDForPath(relPath string) string {
	sum := sha1.Sum([]byte(filepath.ToSlash(relPath)))
	return hex.EncodeToString(sum[:8])
}

// IsVideo reports whether a filename is something we can play.
func IsVideo(name string) (mime string, ok bool) {
	mime, ok = videoExtensions[strings.ToLower(filepath.Ext(name))]
	return
}

// Scan walks the root, collecting videos and folders. It never fails hard:
// an unreadable subtree is skipped and counted in the returned warning.
func Scan(root string) (*Library, error) {
	stat, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !stat.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", root)
	}

	lib := &Library{Root: root, ScannedAt: time.Now(), Items: []Item{}, Folders: []Folder{}}
	counts := map[string]int{}

	type row struct {
		path    string
		relPath string
	}
	var files []row

	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil // unreadable: keep going
		}
		if entry.IsDir() {
			if path == root {
				return nil
			}
			return nil
		}
		if _, ok := videoExtensions[strings.ToLower(filepath.Ext(entry.Name()))]; ok {
			rel, _ := filepath.Rel(root, path)
			files = append(files, row{path: path, relPath: filepath.ToSlash(rel)})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		info, err := os.Stat(file.path)
		if err != nil {
			continue
		}
		rel := file.relPath
		folder := ""
		if parent := filepath.Dir(rel); parent != "." {
			folder = filepath.ToSlash(parent)
			counts[folder]++
		}
		base := filepath.Base(rel)
		title, year := parseTitle(strings.TrimSuffix(base, filepath.Ext(base)))
		lib.Items = append(lib.Items, Item{
			ID:        IDForPath(rel),
			Title:     title,
			RelPath:   rel,
			Folder:    folder,
			Size:      info.Size(),
			ModTime:   info.ModTime().UTC().Format(time.RFC3339),
			Mime:      videoExtensions[strings.ToLower(filepath.Ext(base))],
			Year:      year,
			HasPoster: posterFor(file.path) != "",
		})
	}

	sort.Slice(lib.Items, func(i, j int) bool {
		return modTimeOf(lib.Items[i]).After(modTimeOf(lib.Items[j]))
	})

	for folder, count := range counts {
		lib.Folders = append(lib.Folders, Folder{
			ID:      "d" + IDForPath(folder),
			Name:    filepath.Base(folder),
			RelPath: folder,
			Count:   count,
		})
	}
	sort.Slice(lib.Folders, func(i, j int) bool { return lib.Folders[i].Name < lib.Folders[j].Name })

	return lib, nil
}

// posterFor finds cover art sitting next to a video, named like it.
func posterFor(videoPath string) string {
	base := strings.TrimSuffix(videoPath, filepath.Ext(videoPath))
	for _, ext := range posterExtensions {
		if stat, err := os.Stat(base + ext); err == nil && !stat.IsDir() {
			return base + ext
		}
	}
	return ""
}

// Find returns the item with the given id, wherever it is on the shelf.
func (l *Library) Find(id string) (Item, bool) {
	for _, item := range l.Items {
		if item.ID == id {
			return item, true
		}
	}
	return Item{}, false
}

// FolderByDLNAID finds a folder by the id the ContentDirectory hands out.
func (l *Library) FolderByDLNAID(id string) (Folder, bool) {
	for _, folder := range l.Folders {
		if folder.ID == id {
			return folder, true
		}
	}
	return Folder{}, false
}

// ItemsInFolder returns the videos living directly under a folder.
func (l *Library) ItemsInFolder(folderRel string) []Item {
	var out []Item
	for _, item := range l.Items {
		if item.Folder == folderRel {
			out = append(out, item)
		}
	}
	return out
}

func modTimeOf(item Item) time.Time {
	if t, err := time.Parse(time.RFC3339, item.ModTime); err == nil {
		return t
	}
	return time.Time{}
}

// --- filename titles ---

var (
	yearRe  = regexp.MustCompile(`\((19|20)\d{2}\)|\b(19|20)\d{2}\b`)
	spaceRe = regexp.MustCompile(`[\._]+`)
	// A name starting like 2026-09-25 13-31-03 is a recording, not a release:
	// the date is the identity, so it stays whole.
	leadingDate = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}`)
	tagWords    = []string{"1080p", "720p", "2160p", "480p", "4k", "web", "webrip", "web-dl",
		"bluray", "blu-ray", "brrip", "bdrip", "dvdrip", "hdrip", "hdtv", "hdr",
		"x264", "x265", "h264", "h265", "hevc", "avc", "xvid", "divx",
		"aac", "aac2", "ac3", "eac3", "dts", "dts-hd", "flac", "mp3", "truehd", "atmos",
		"remux", "proper", "repack", "extended", "unrated", "remastered", "imax",
		"multi", "dual", "audio", "subbed", "sub", "ita", "eng", "rus", "ger", "fre", "jpn",
	}
)

// normalizeSeparators turns dots, underscores and structural hyphens into
// spaces. A hyphen stays only when a capital letter follows it, which is how
// title compounds are written: "Spider-Man" and "X-Men" survive, while
// "the-neon-archive-2021" and "4k-hdr-hevc" come apart.
func normalizeSeparators(name string) string {
	runes := []rune(spaceRe.ReplaceAllString(name, " "))
	for i, ch := range runes {
		if ch != '-' {
			continue
		}
		if i+1 < len(runes) && unicode.IsUpper(runes[i+1]) {
			continue
		}
		runes[i] = ' '
	}
	return string(runes)
}

// parseTitle turns "The.Neon.Archive.2021.1080p.WEB-DL.x264" into
// "The Neon Archive", 2021. Release tags go, the title stays.
func parseTitle(name string) (title string, year int) {
	if leadingDate.MatchString(name) {
		if parsed, err := strconv.Atoi(name[:4]); err == nil && parsed >= 1900 {
			year = parsed
		}
		return strings.TrimSpace(normalizeSeparators(name)), year
	}
	cleaned := name
	if match := yearRe.FindString(cleaned); match != "" {
		digits := regexp.MustCompile(`\d{4}`).FindString(match)
		if parsed, err := strconv.Atoi(digits); err == nil && parsed >= 1900 && parsed <= 2100 {
			year = parsed
		}
		cleaned = strings.TrimSpace(strings.Replace(cleaned, match, "", 1))
	}

	cleaned = normalizeSeparators(cleaned)
	words := strings.Fields(cleaned)
	kept := make([]string, 0, len(words))
	for _, word := range words {
		if isTagWord(word) {
			break // everything after a release tag is tags, not title
		}
		kept = append(kept, word)
	}
	title = strings.TrimSpace(strings.Join(kept, " "))
	if title == "" {
		title = strings.TrimSpace(cleaned)
	}
	if title == "" {
		title = name
	}
	return title, year
}

func isTagWord(word string) bool {
	lower := strings.ToLower(strings.Trim(word, "[](){}-"))
	if lower == "" {
		return false
	}
	for _, tag := range tagWords {
		if lower == tag || lower == tag+"s" {
			return true
		}
	}
	return false
}

// --- metadata enrichment (ffprobe, optional) ---

// Metadata is what ffprobe tells us about one file, when it is installed.
type Metadata struct {
	Duration float64
	Width    int
	Height   int
}

// Prober reads duration and dimensions. It is nil-safe: without ffprobe,
// metadata simply stays empty and players figure the file out themselves.
type Prober struct {
	mu       sync.Mutex
	previous map[string]Metadata
}

func NewProber() *Prober {
	return &Prober{previous: map[string]Metadata{}}
}

// Enrich fills in duration and resolution for every item that lacks them.
// Keys are stable per rel path and modification time, so nothing is probed
// twice unless the file actually changed.
func (p *Prober) Enrich(items []Item, root string) {
	for i, item := range items {
		abs := filepath.Join(root, filepath.FromSlash(item.RelPath))
		key := fmt.Sprintf("%s|%s", item.RelPath, item.ModTime)
		p.mu.Lock()
		cached, hit := p.previous[key]
		p.mu.Unlock()
		if hit {
			items[i].Duration = cached.Duration
			items[i].Width = cached.Width
			items[i].Height = cached.Height
			continue
		}
		meta := probe(abs)
		p.mu.Lock()
		p.previous[key] = meta
		p.mu.Unlock()
		items[i].Duration = meta.Duration
		items[i].Width = meta.Width
		items[i].Height = meta.Height
	}
}

func probe(absPath string) Metadata {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "format=duration:stream=width,height",
		"-of", "json", absPath)
	out, err := cmd.Output()
	if err != nil {
		return Metadata{}
	}
	var parsed struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
		Streams []struct {
			Width  int `json:"width"`
			Height int `json:"height"`
		} `json:"streams"`
	}
	if json.Unmarshal(out, &parsed) != nil {
		return Metadata{}
	}
	meta := Metadata{}
	if parsed.Format.Duration != "" {
		if seconds, err := strconv.ParseFloat(parsed.Format.Duration, 64); err == nil {
			meta.Duration = seconds
		}
	}
	if len(parsed.Streams) > 0 {
		meta.Width = parsed.Streams[0].Width
		meta.Height = parsed.Streams[0].Height
	}
	return meta
}
