# golink

A lightweight URL shortener for your local network. Create short links like `go/yt` that redirect to `https://youtube.com`. Runs on a Raspberry Pi alongside Pi-hole.

## Features

- Simple web interface to create, edit and delete links
- Instant redirects: `go/gh` → `https://github.com`
- Auto-prefill: visiting `go/newlink` suggests creating it
- Transparent Pi-hole reverse proxy
- Single binary, JSON file storage

## Quick Start

### Build

```bash
# Cross-compile for Raspberry Pi (ARMv6)
make build

# Or build for your local machine
make build-local
```

### Install on Raspberry Pi

```bash
# Copy files
make deploy PI_HOST=pi@192.168.1.10

# On the Pi
ssh pi@192.168.1.10
sudo mkdir -p /opt/golink
sudo mv /tmp/golink /usr/local/bin/
sudo mv /tmp/golink.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now golink
```

### DNS Setup

Add a DNS record pointing `go` to your Pi's IP (via Pi-hole or your router).

Then visit **http://go/** from any device on your network.

## Configuration

Environment variables (all optional):

| Variable        | Default                 | Description          |
| --------------- | ----------------------- | -------------------- |
| `PIHOLE_TARGET` | `http://127.0.0.1:8080` | Pi-hole proxy target |
| `LISTEN_ADDR`   | `:80`                   | Listen address       |
| `DB_FILE`       | `go_links.json`         | Database file path   |

Override via systemd:

```bash
sudo systemctl edit golink
```

```ini
[Service]
Environment="DB_FILE=/opt/golink/links.json"
```

## Pi-hole Setup

If Pi-hole uses port 80, move it to 8080:

```bash
sudo vim /etc/pihole/pihole.toml
# Set: port = "8080o,443os,[::]:8080o,[::]:443os"
sudo systemctl restart pihole-FTL
```

## Project Structure

```
cmd/golink/         Entrypoint
internal/
  config/           Configuration
  server/           HTTP server lifecycle
  store/            Thread-safe link storage
  web/              Handlers and HTML template
deploy/             Systemd service file
```

## Development

```bash
make run            # Build and run locally
make fmt            # Format code
make vet            # Run go vet
make lint           # Run staticcheck
make test           # Run tests
make help           # Show all targets
```

## License

[MIT](LICENSE)
