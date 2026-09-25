// Package api serves the web UI, the JSON library, the video bytes with
// ranges, poster art, and the DLNA endpoints a television browses.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/andrinoff/watch/internal/dlna"
	"github.com/andrinoff/watch/internal/library"
	"github.com/andrinoff/watch/web"
)

// Server is the whole HTTP surface.
type Server struct {
	manager *library.Manager
	uuid    string
	name    string
	dataDir string
}

func New(manager *library.Manager, uuid, name, dataDir string) *Server {
	return &Server{manager: manager, uuid: uuid, name: name, dataDir: dataDir}
}

// Handler builds the router.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /api/library", s.getLibrary)
	mux.HandleFunc("POST /api/rescan", s.rescan)
	mux.HandleFunc("GET /api/media/{id}", s.getMedia)
	mux.HandleFunc("GET /api/media/{id}/{name}", s.getMedia)
	mux.HandleFunc("GET /api/poster/{id}", s.getPoster)

	// DLNA: the device, its descriptions, the SOAP control, and the media
	// URLs whose path carries the extension picky renderers want ({id}
	// matches a whole dotted segment, which item() strips back down).
	mux.HandleFunc("GET /dlna/device.xml", s.deviceDescription)
	mux.HandleFunc("GET /dlna/cds.xml", func(w http.ResponseWriter, r *http.Request) {
		xmlBody(w, dlna.ContentDirectoryXML())
	})
	mux.HandleFunc("GET /dlna/cm.xml", func(w http.ResponseWriter, r *http.Request) {
		xmlBody(w, dlna.ConnectionManagerXML())
	})
	mux.HandleFunc("POST /dlna/cds/control", s.contentDirectoryControl)
	mux.HandleFunc("POST /dlna/cm/control", s.connectionManagerControl)
	mux.HandleFunc("GET /dlna/media/{id}", s.getMedia)
	mux.HandleFunc("POST /dlna/cds/events", s.subscribe)
	mux.HandleFunc("SUBSCRIBE /dlna/cds/events", s.subscribe)

	mux.HandleFunc("/", s.static)

	return logRequests(mux)
}

// --- JSON ---

func (s *Server) getLibrary(w http.ResponseWriter, r *http.Request) {
	lib := s.manager.Snapshot()
	if lib == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "still scanning"})
		return
	}
	writeJSON(w, http.StatusOK, lib)
}

func (s *Server) rescan(w http.ResponseWriter, r *http.Request) {
	count, err := s.manager.Scan()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"items": count})
}

// --- media ---

// getMedia streams a video. http.ServeContent does ranges, HEAD and
// Last-Modified; the mime type comes from the shelf, not the sniffing.
func (s *Server) getMedia(w http.ResponseWriter, r *http.Request) {
	item, ok := s.item(w, r)
	if !ok {
		return
	}
	file, err := os.Open(s.manager.PathOf(item))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not open the video")
		return
	}
	defer file.Close()

	if r.URL.Query().Get("download") == "1" {
		w.Header().Set("Content-Disposition",
			fmt.Sprintf("attachment; filename=%q", downloadName(item)))
	}
	w.Header().Set("Content-Type", item.Mime)
	http.ServeContent(w, r, item.Title+filepath.Ext(item.RelPath), time.Time{}, file)
}

func downloadName(item library.Item) string {
	name := item.Title + filepath.Ext(item.RelPath)
	for _, ch := range name {
		if ch < ' ' || strings.ContainsRune(`"\/:;*?<>|`, ch) {
			name = strings.Map(func(r rune) rune {
				if r < ' ' || strings.ContainsRune(`"\/:;*?<>|`, r) {
					return '-'
				}
				return r
			}, name)
			break
		}
	}
	return name
}

// --- posters ---

// getPoster answers with cover art: the poster beside the video, a frame
// grabbed by ffmpeg, or a designed placeholder. Cheap enough to be lazy.
func (s *Server) getPoster(w http.ResponseWriter, r *http.Request) {
	item, ok := s.item(w, r)
	if !ok {
		return
	}
	if poster := s.manager.PosterFor(item); poster != "" {
		http.ServeFile(w, r, poster)
		return
	}

	thumbDir := filepath.Join(s.dataDir, "thumbs")
	thumb := filepath.Join(thumbDir, item.ID+".jpg")
	if stat, err := os.Stat(thumb); err == nil && stat.Size() > 0 {
		http.ServeFile(w, r, thumb)
		return
	}
	if s.grabFrame(item, thumbDir, thumb) {
		http.ServeFile(w, r, thumb)
		return
	}
	svg, etag := placeholder(item)
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "max-age=86400")
	w.Header().Set("ETag", etag)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	_, _ = io.WriteString(w, svg)
}

// grabFrame asks ffmpeg for one still, about a fifth of the way in, so the
// placeholder has something real to show on the second visit.
func (s *Server) grabFrame(item library.Item, thumbDir, target string) bool {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return false
	}
	if err := os.MkdirAll(thumbDir, 0o755); err != nil {
		return false
	}
	source := s.manager.PathOf(item)
	seek := 0.0
	if item.Duration > 0 {
		seek = item.Duration * 0.2
	}
	// ffmpeg picks its muxer from the extension, so the in-progress file
	// keeps one.
	temp := target + ".part.jpg"
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-v", "error",
		"-ss", strconv.FormatFloat(seek, 'f', 1, 64),
		"-i", source,
		"-frames:v", "1",
		"-vf", "scale=640:-2",
		"-q:v", "4",
		"-y", temp,
	)
	if err := cmd.Run(); err != nil {
		_ = os.Remove(temp)
		return false
	}
	if err := os.Rename(temp, target); err != nil {
		return false
	}
	return true
}

// placeholder is a designed tile for videos without art: a duotone picked
// from the title, the title itself, and the year.
var placeholderPalette = [][2]string{
	{"#2b2331", "#6a2f63"}, {"#24231f", "#6b5a1f"}, {"#1f2a26", "#2f6a52"},
	{"#2a1f1c", "#7c4a2a"}, {"#1d2430", "#3a5a80"}, {"#2a2028", "#8a3a5c"},
}

func placeholder(item library.Item) (string, string) {
	sum := 0
	for _, ch := range item.Title {
		sum = (sum*31 + int(ch)) % 1000
	}
	pair := placeholderPalette[sum%len(placeholderPalette)]
	year := ""
	if item.Year > 0 {
		year = strconv.Itoa(item.Year)
	}
	// One line per word, up to three, so long titles stay inside the tile.
	words := strings.Fields(strings.ToUpper(item.Title))
	lines := []string{}
	line := ""
	for _, word := range words {
		if len(line)+len(word)+1 > 16 && line != "" {
			lines = append(lines, line)
			line = word
			continue
		}
		if line == "" {
			line = word
		} else {
			line += " " + word
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	if len(lines) > 3 {
		lines = lines[:3]
	}
	var text strings.Builder
	for i, l := range lines {
		escaped := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(l)
		text.WriteString(fmt.Sprintf(
			`<text x="40" y="%d" font-family="Arial Narrow, Helvetica, sans-serif" font-size="34" font-weight="700" fill="#f4f2f5" letter-spacing="1">%s</text>`,
			360-70-(len(lines)-1-i)*44, escaped))
	}
	if year != "" {
		text.WriteString(fmt.Sprintf(
			`<text x="40" y="380" font-family="Georgia, serif" font-size="26" fill="#a09aa6">%s</text>`, year))
	}
	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="640" height="400" viewBox="0 0 640 400">
<rect width="640" height="400" fill="%s"/>
<path d="M0 400 L640 300 L640 400 Z" fill="%s" opacity="0.5"/>
<rect x="40" y="80" width="3" height="40" fill="#e8b45a"/>
%s
</svg>`, pair[0], pair[1], text.String())
	return svg, fmt.Sprintf(`"%d"`, sum)
}

// --- DLNA ---

// deviceDescription points the renderer at everything else, with absolute
// URLs built from the address it used to get here.
func (s *Server) deviceDescription(w http.ResponseWriter, r *http.Request) {
	base := "http://" + r.Host
	xmlBody(w, dlna.DeviceXML(base, s.uuid, s.name))
}

func (s *Server) contentDirectoryControl(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "could not read the request")
		return
	}
	base := "http://" + r.Host
	_, response, status := dlna.Control(body, shelf{manager: s.manager}, base)
	w.Header().Set("Content-Type", `text/xml; charset="utf-8"`)
	w.WriteHeader(status)
	_, _ = io.WriteString(w, response)
}

func (s *Server) connectionManagerControl(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", `text/xml; charset="utf-8"`)
	_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>`+
		`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">`+
		`<s:Body><u:GetProtocolInfoResponse xmlns:u="urn:schemas-upnp-org:service:ConnectionManager:1">`+
		`<Source>http-get:*:*:*,http-get:*:video/*:DLNA.ORG_PN=AVC_MP4;DLNA.ORG_OP=01</Source>`+
		`<Sink></Sink></u:GetProtocolInfoResponse></s:Body></s:Envelope>`)
}

// subscribe answers GENA subscriptions, which some televisions insist on
// before browsing. No events are sent; the shelf is small.
func (s *Server) subscribe(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("SID", "uuid:"+s.uuid+"-events")
	w.Header().Set("TIMEOUT", "Second-1800")
	w.WriteHeader(http.StatusOK)
}

// --- the shelf as a DLNA tree ---

// shelf adapts the library into the entries a ContentDirectory hands out.
// Object ids: "0" root, "d…" folders, "v…" videos.
type shelf struct {
	manager *library.Manager
}

func (t shelf) Root() []dlna.Entry {
	lib := t.manager.Snapshot()
	if lib == nil {
		return nil
	}
	var entries []dlna.Entry
	for _, folder := range lib.Folders {
		entries = append(entries, dlna.Entry{
			ID:         folder.ID,
			ParentID:   "0",
			Title:      folder.Name,
			ChildCount: folder.Count,
		})
	}
	for _, item := range lib.Items {
		if item.Folder == "" {
			entries = append(entries, t.itemEntry(item, "0"))
		}
	}
	return entries
}

func (t shelf) Children(objectID string) ([]dlna.Entry, int, bool) {
	lib := t.manager.Snapshot()
	if lib == nil {
		return nil, 0, false
	}
	if objectID == "0" {
		entries := t.Root()
		return entries, len(entries), true
	}
	if folder, ok := lib.FolderByDLNAID(objectID); ok {
		items := lib.ItemsInFolder(folder.RelPath)
		entries := make([]dlna.Entry, 0, len(items))
		for _, item := range items {
			entries = append(entries, t.itemEntry(item, folder.ID))
		}
		return entries, len(entries), true
	}
	return nil, 0, false
}

func (t shelf) Metadata(objectID string) (dlna.Entry, bool) {
	lib := t.manager.Snapshot()
	if lib == nil {
		return dlna.Entry{}, false
	}
	switch {
	case objectID == "0":
		entries := t.Root()
		return dlna.Entry{ID: "0", Title: "Movies", ChildCount: len(entries)}, true
	case strings.HasPrefix(objectID, "d"):
		if folder, ok := lib.FolderByDLNAID(objectID); ok {
			return dlna.Entry{
				ID:         folder.ID,
				ParentID:   "0",
				Title:      folder.Name,
				ChildCount: folder.Count,
			}, true
		}
	case strings.HasPrefix(objectID, "v"):
		if item, ok := lib.Find(itemID(objectID)); ok {
			parent := "0"
			if item.Folder != "" {
				parent = "d" + library.IDForPath(item.Folder)
			}
			return t.itemEntry(item, parent), true
		}
	}
	return dlna.Entry{}, false
}

func (t shelf) itemEntry(item library.Item, parentID string) dlna.Entry {
	ext := strings.TrimPrefix(filepath.Ext(item.RelPath), ".")
	return dlna.Entry{
		ID:           "v" + item.ID,
		ParentID:     parentID,
		Title:        item.Title,
		Class:        "object.item.videoItem",
		URI:          "/dlna/media/" + item.ID + "." + ext,
		ProtocolInfo: dlna.ProtocolInfo(item.Mime),
		Mime:         item.Mime,
		Size:         item.Size,
		Duration:     dlna.FormatDuration(item.Duration),
		Resolution:   resolutionOf(item),
	}
}

func itemID(objectID string) string {
	return strings.TrimPrefix(objectID, "v")
}

func resolutionOf(item library.Item) string {
	if item.Width == 0 || item.Height == 0 {
		return ""
	}
	return fmt.Sprintf("%dx%d", item.Width, item.Height)
}

// --- static + helpers ---

func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/dlna/") {
		writeError(w, http.StatusNotFound, "no such endpoint")
		return
	}
	sub, err := web.FS()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "no frontend embedded")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}
	if _, err := fs.Stat(sub, path); err != nil {
		index, readErr := fs.ReadFile(sub, "index.html")
		if readErr != nil {
			writeError(w, http.StatusServiceUnavailable, "frontend not built; run make build")
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(index)
		return
	}
	http.FileServerFS(sub).ServeHTTP(w, r)
}

func (s *Server) item(w http.ResponseWriter, r *http.Request) (library.Item, bool) {
	id := r.PathValue("id")
	// /dlna/media/<id>.<ext> carries the id in a dotted segment.
	if dot := strings.LastIndexByte(id, '.'); dot > 0 {
		id = id[:dot]
	}
	lib := s.manager.Snapshot()
	if lib == nil {
		writeError(w, http.StatusServiceUnavailable, "still scanning")
		return library.Item{}, false
	}
	item, ok := lib.Find(id)
	if !ok {
		writeError(w, http.StatusNotFound, "no such video")
		return library.Item{}, false
	}
	return item, true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = encodeJSON(w, v)
}

func encodeJSON(w io.Writer, v any) error {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(v)
}

func xmlBody(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", `text/xml; charset="utf-8"`)
	_, _ = io.WriteString(w, body)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		// The video path logs every range request, which is noise; keep it out.
		if !strings.HasPrefix(r.URL.Path, "/api/media/") && !strings.HasPrefix(r.URL.Path, "/dlna/media/") {
			log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Microsecond))
		}
		next.ServeHTTP(w, r)
	})
}
