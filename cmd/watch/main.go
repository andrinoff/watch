// Command watch is a movies shelf: it scans a folder, shows it in a web UI,
// and announces itself over DLNA so televisions can find it.
package main

import (
	crand "crypto/rand"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/andrinoff/watch/internal/api"
	"github.com/andrinoff/watch/internal/dlna"
	"github.com/andrinoff/watch/internal/library"
)

func main() {
	addr := flag.String("addr", "0.0.0.0:8092", "listen address; needs a LAN address for DLNA")
	dir := flag.String("dir", defaultMoviesDir(), "movies folder")
	name := flag.String("name", "", "friendly DLNA name (default: <host> watch)")
	dataDir := flag.String("data", "./data", "cache directory (uuid, thumbnails)")
	noDLNA := flag.Bool("no-dlna", false, "turn the TV announcements off")
	rescan := flag.Int("rescan", 60, "seconds between scans")
	flag.Parse()

	if err := os.MkdirAll(*dataDir, 0o755); err != nil {
		log.Fatalf("data dir: %v", err)
	}
	uuid := deviceUUID(filepath.Join(*dataDir, "uuid"))

	friendly := *name
	if friendly == "" {
		host, _ := os.Hostname()
		friendly = strings.TrimSuffix(host, ".local") + " watch"
	}

	manager := library.NewManager(*dir)
	if _, err := manager.Scan(); err != nil {
		log.Printf("library: %v (the shelf will be empty until this is fixed)", err)
	}
	manager.Start(time.Duration(*rescan) * time.Second)

	server := &http.Server{
		Addr:              *addr,
		Handler:           api.New(manager, uuid, friendly, *dataDir).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}

	if !*noDLNA {
		announcer, err := dlna.Start(uuid, listener.Addr().(*net.TCPAddr).Port, friendly)
		if err != nil {
			log.Printf("dlna: %v (televisions will not find the shelf; the web UI still works)", err)
		} else {
			defer announcer.Stop()
		}
	}

	log.Printf("watch: %s on http://%s (%d items, friendly name %q)", *dir, *addr, countOf(manager), friendly)
	if err := server.Serve(listener); err != nil {
		log.Fatal(err)
	}
}

func defaultMoviesDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "./Movies"
	}
	return filepath.Join(home, "Movies")
}

// deviceUUID keeps one canonical UUID per machine. Renderers (cling-based
// stacks in particular) validate the UDN format, so a malformed id from an
// earlier build gets regenerated instead of persisting forever.
func deviceUUID(path string) string {
	if bytes, err := os.ReadFile(path); err == nil {
		if text := strings.TrimSpace(string(bytes)); text != "" && canonicalUUID(text) {
			return text
		}
	}
	id := canonical()
	if err := os.WriteFile(path, []byte(id), 0o600); err != nil {
		log.Printf("could not persist the device uuid: %v", err)
	}
	return id
}

// canonical is the 8-4-4-4-12 shape with the version and variant bits set,
// like any other UUID v4.
func canonical() string {
	b := make([]byte, 16)
	if _, err := crand.Read(b); err != nil {
		log.Fatalf("read randomness: %v", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func canonicalUUID(s string) bool {
	parts := strings.Split(s, "-")
	if len(parts) != 5 {
		return false
	}
	lengths := []int{8, 4, 4, 4, 12}
	for i, part := range parts {
		if len(part) != lengths[i] {
			return false
		}
		for _, ch := range part {
			if !(ch >= '0' && ch <= '9' || ch >= 'a' && ch <= 'f') {
				return false
			}
		}
	}
	return true
}

func countOf(manager *library.Manager) int {
	if lib := manager.Snapshot(); lib != nil {
		return len(lib.Items)
	}
	return 0
}
