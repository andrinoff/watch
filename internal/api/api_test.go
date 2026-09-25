package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andrinoff/watch/internal/library"
)

func newTestServer(t *testing.T) (*httptest.Server, *library.Manager) {
	t.Helper()
	dir := t.TempDir()
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("The.Neon.Archive.2021.mp4", "0123456789")
	write("2026-09-25 13-31-03.mov", "abcdefgh")

	manager := library.NewManager(dir)
	if _, err := manager.Scan(); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(New(manager, "test-uuid", "test watch", t.TempDir()).Handler())
	t.Cleanup(ts.Close)
	return ts, manager
}

func get(t *testing.T, url string, headers map[string]string) (int, http.Header, []byte) {
	t.Helper()
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	return res.StatusCode, res.Header, body
}

func TestLibraryListsTheShelf(t *testing.T) {
	ts, _ := newTestServer(t)
	status, _, body := get(t, ts.URL+"/api/library", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d: %s", status, body)
	}
	var lib struct {
		Root  string `json:"root"`
		Items []struct {
			ID      string  `json:"id"`
			Title   string  `json:"title"`
			Due     float64 `json:"duration"`
			RelPath string  `json:"relPath"`
		} `json:"items"`
		Folders []any `json:"folders"`
	}
	if err := json.Unmarshal(body, &lib); err != nil {
		t.Fatalf("decode: %v\n%s", err, body)
	}
	if len(lib.Items) != 2 {
		t.Fatalf("items = %d, want 2 (%s)", len(lib.Items), body)
	}
	titles := map[string]bool{}
	for _, item := range lib.Items {
		titles[item.Title] = true
	}
	if !titles["The Neon Archive"] || !titles["2026 09 25 13 31 03"] {
		t.Errorf("titles = %#v", titles)
	}
}

func TestMediaStreamsBytesAndRanges(t *testing.T) {
	ts, _ := newTestServer(t)
	_, _, body := get(t, ts.URL+"/api/library", nil)
	var lib struct {
		Items []struct {
			ID   string `json:"id"`
			Mime string `json:"mime"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &lib); err != nil {
		t.Fatal(err)
	}
	id := ""
	mime := ""
	for _, item := range lib.Items {
		if item.Mime == "video/mp4" {
			id, mime = item.ID, item.Mime
			break
		}
	}
	if id == "" {
		t.Fatal("no mp4 item to stream")
	}

	status, header, body := get(t, ts.URL+"/api/media/"+id, map[string]string{"Range": "bytes=0-3"})
	if status != http.StatusPartialContent {
		t.Fatalf("range status = %d, want 206", status)
	}
	if got := header.Get("Content-Range"); !strings.HasPrefix(got, "bytes 0-3/") {
		t.Errorf("Content-Range = %q", got)
	}
	if string(body) != "0123" {
		t.Errorf("range body = %q", body)
	}
	if ct := header.Get("Content-Type"); ct != mime {
		t.Errorf("Content-Type = %q, want %q", ct, mime)
	}

	status, _, body = get(t, ts.URL+"/api/media/"+id, nil)
	if status != http.StatusOK || len(body) != 10 {
		t.Errorf("full read: status %d len %d", status, len(body))
	}

	status, _, _ = get(t, ts.URL+"/api/media/not-an-id", nil)
	if status != http.StatusNotFound {
		t.Errorf("unknown id: status %d", status)
	}
}

func TestRescanAndUnknownAPIPaths(t *testing.T) {
	ts, _ := newTestServer(t)
	res, err := http.Post(ts.URL+"/api/rescan", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Errorf("rescan status = %d", res.StatusCode)
	}
	var out struct {
		Items int `json:"items"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil || out.Items != 2 {
		t.Errorf("rescan body: %v %v", out, err)
	}

	status, _, body := get(t, ts.URL+"/api/nope", nil)
	if status != http.StatusNotFound {
		t.Errorf("unknown api path: status %d %s", status, body)
	}
}

func TestDeviceDescriptionAndControlOnTheRealHandler(t *testing.T) {
	ts, _ := newTestServer(t)
	status, _, body := get(t, ts.URL+"/dlna/device.xml", nil)
	if status != http.StatusOK {
		t.Fatalf("device.xml status = %d", status)
	}
	text := string(body)
	for _, want := range []string{"MediaServer:1", "uuid:test-uuid", "test watch", "/dlna/cds/control"} {
		if !strings.Contains(text, want) {
			t.Errorf("device.xml missing %q:\n%s", want, text)
		}
	}

	soap := `<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body><u:Browse xmlns:u="urn:schemas-upnp-org:service:ContentDirectory:1"><ObjectID>0</ObjectID><BrowseFlag>BrowseDirectChildren</BrowseFlag><Filter>*</Filter><StartingIndex>0</StartingIndex><RequestedCount>0</RequestedCount><SortCriteria></SortCriteria></u:Browse></s:Body></s:Envelope>`
	res, err := http.Post(ts.URL+"/dlna/cds/control", "text/xml", strings.NewReader(soap))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	answer, _ := io.ReadAll(res.Body)
	text = string(answer)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("control status = %d: %s", res.StatusCode, text)
	}
	if !strings.Contains(text, "&lt;item") && !strings.Contains(text, "<item") {
		t.Errorf("no items in the DIDL: %s", text)
	}
	if !strings.Contains(text, ts.Listener.Addr().String()) {
		t.Errorf("media URLs should be absolute on this host: %s", text)
	}
}

func TestPosterAlwaysAnswersSomething(t *testing.T) {
	ts, _ := newTestServer(t)
	_, _, body := get(t, ts.URL+"/api/library", nil)
	var lib struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &lib); err != nil {
		t.Fatal(err)
	}
	for _, item := range lib.Items {
		status, header, body := get(t, ts.URL+"/api/poster/"+item.ID, nil)
		if status != http.StatusOK {
			t.Fatalf("poster for %s: status %d", item.ID, status)
		}
		if header.Get("Content-Type") == "" {
			t.Errorf("poster for %s: no content type", item.ID)
		}
		if len(body) == 0 {
			t.Errorf("poster for %s: empty body", item.ID)
		}
	}
}
