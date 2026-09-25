# watch

A movies shelf for the household. Drop videos into the Movies folder and they
appear two places at once: in a web UI at your domain, and on your Android TV
(and VLC, and the rest) through DLNA, because the server announces itself over
SSDP so the TV finds it without being told anything.

- **Backend**: pure Go, one binary, the frontend embedded. No database: a JSON
  library snapshot in memory plus a thumbnail cache.
- **TVs**: SSDP + UPnP ContentDirectory on the LAN. Folders map to your
  folders; metadata (duration, resolution) comes from `ffprobe` when present.
- **Frontend**: React + Vite, a poster grid with a player page, resume
  positions kept in the browser.
- **Art**: a poster next to the video wins; otherwise `ffmpeg` grabs a frame;
  otherwise a designed placeholder carries the title and year.

## Run it

```bash
make build
./watch -dir "$HOME/Movies"
```

Defaults: listens on `0.0.0.0:8092`, scans every 60 seconds, DLNA on. The LAN
bind is on purpose: the TV speaks plain HTTP, and Caddy fronts the browser
side from loopback.

| Flag | Default | Meaning |
| --- | --- | --- |
| `-addr` | `0.0.0.0:8092` | Listen address. Needs to be reachable on the LAN. |
| `-dir` | `~/Movies` | The movies folder. |
| `-name` | `<host> watch` | What the TV sees in its device list. |
| `-data` | `./data` | UUID + thumbnail cache. |
| `-no-dlna` | off | No SSDP announcements (when the box sits on Tailscale only). |
| `-rescan` | `60` | Seconds between scans. |

## Titles from filenames

Release names become readable titles: `The.Neon.Archive.2021.1080p.WEB-DL.x264`
shows as "The Neon Archive (2021)". A name that starts with a date
(`2026-09-25 13-31-03.mov`) is treated as a recording and kept whole. A hyphen
stays a hyphen only when a capital letter follows, so `Spider-Man` survives.
A file named like its video (`.jpg` / `.jpeg` / `.png` / `.webp`) beside it
becomes the poster.

## Deploy on Ubuntu, behind Caddy

```bash
make build
sudo ./deploy/install.sh                      # defaults: /home/<you>/Movies
sudo ./deploy/install.sh /mnt/media/Movies    # or another folder
```

The service runs as the user who owns the folder (not root, not a service
user), so the shelf is readable without chmod acrobatics. It binds every
interface: the TV needs plain HTTP on the LAN.

Then the domain:

```bash
sudo ./deploy/setup-domain.sh watch.andrinoff.com
```

Adds one site block to `/etc/caddy/Caddyfile` alongside your other sites,
reusing the Cloudflare token already there, and reloads Caddy.

```bash
journalctl -u watch -e --no-pager     # "library: N videos, M folders"
ss -ulnp | grep 1900                  # the SSDP socket
```

## API

```
GET  /api/health
GET  /api/library                      the shelf as JSON
POST /api/rescan
GET  /api/media/{id}[?download=1]      streams with byte ranges
GET  /api/poster/{id}                  cover art, a grabbed frame, or a tile
GET  /dlna/device.xml                  UPnP device description
GET  /dlna/cds.xml                     ContentDirectory description
POST /dlna/cds/control                 SOAP Browse/GetSystemUpdateID/...
GET  /dlna/media/{id}.{ext}            the same bytes, with a media extension
```

## Develop

```bash
make dev-api   # the server on :8092 (use -no-dlna off the LAN)
make dev-web   # Vite on :5173, /api proxied
make test      # go vet + go test (titles, DIDL, SSDP, ranges)
make fmt       # gofmt + tsc
```

## Layout

```
cmd/watch/          entry point (flags, listener, announcer wiring)
internal/library/   the scan: titles, ids, ffprobe enrichment
internal/dlna/      SSDP announcements, ContentDirectory, DIDL
internal/api/       JSON, streaming with ranges, posters, SOAP wiring
web/                React + Vite (built into the binary)
deploy/             systemd unit, install.sh, setup-domain.sh
```

## Notes and limits

- DLNA speaks plain HTTP on the LAN by design: anyone on the network can open
  the shelf. That's the model; the tailnet-only part is the browser side via
  Caddy.
- `ffprobe` and `ffmpeg` are optional: metadata and thumbnails appear when
  they're installed, and nothing breaks when they aren't.
- Resume positions live in the browser's localStorage, not on the server: the
  web player resumes; the TV starts from zero.
