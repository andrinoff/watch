// Command watch is a movies shelf: it scans a folder, shows it in a web UI,
// and announces itself over DLNA so televisions can find it.
package main

import (
	crand "crypto/rand"
	"encoding/binary"
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

func deviceUUID(path string) string {
	if bytes, err := os.ReadFile(path); err == nil {
		if text := strings.TrimSpace(string(bytes)); text != "" {
			return text
		}
	}
	id := fmt.Sprintf("%x-%x-%x-%x-%x", randUint(), randUint(), randUint(), randUint(), randUint())
	if err := os.WriteFile(path, []byte(id), 0o600); err != nil {
		log.Printf("could not persist the device uuid: %v", err)
	}
	return id
}

func countOf(manager *library.Manager) int {
	if lib := manager.Snapshot(); lib != nil {
		return len(lib.Items)
	}
	return 0
}

// randUint feeds the UDN once, on first boot; crypto/rand is the honest
// source and never blocks like a full read of /dev/urandom would.
func randUint() uint32 {
	b := make([]byte, 4)
	if _, err := crand.Read(b); err != nil {
		return uint32(time.Now().UnixNano())
	}
	return binary.LittleEndian.Uint32(b)
}
