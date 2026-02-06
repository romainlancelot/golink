# Go Links - URL Shortener for Raspberry Pi

A lightweight URL shortener that runs on your Raspberry Pi alongside Pi-hole v6. Access your favorite sites with simple shortcuts like `go/youtube`, `go/github`, etc.

## Features

- 🔗 Custom short links with a simple web interface
- 🔄 Pi-hole v6 transparent proxy (optional)
- 💾 Persistent JSON storage
- 🚀 Fast and lightweight (single binary)
- ⚙️ Environment variable configuration

## Requirements

- Raspberry Pi (ARMv6/v7/v8)
- Go 1.21+ installed on your build machine
- (Optional) Pi-hole v6 running on your Raspberry Pi
- Root access on Raspberry Pi (to bind to port 80)

---

## Quick Start

### 1. Prepare Pi-hole (Optional)

If you're running Pi-hole, move it to port 8080 to free up port 80:

```bash
# On Raspberry Pi
sudo vim /etc/pihole/pihole.toml

# Find [webserver] section and set:
# port = "8080o,443os,[::]:8080o,[::]:443os"

sudo systemctl restart pihole-FTL

# Verify Pi-hole is on port 8080
curl -I http://localhost:8080/admin/
```

### 2. Build and Transfer

On your PC:

```bash
./build.sh
scp golink golink.service pi@<PI_IP>:/home/pi/
```

### 3. Install on Raspberry Pi

```bash
# SSH into Raspberry Pi
ssh pi@<PI_IP>

# Create directories and move files
sudo mkdir -p /opt/golink
sudo mv /home/pi/golink /usr/local/bin/
sudo chmod +x /usr/local/bin/golink
sudo mv /home/pi/golink.service /etc/systemd/system/

# Start the service
sudo systemctl daemon-reload
sudo systemctl enable --now golink
sudo systemctl status golink
```

### 4. Configure DNS

Add DNS record in Pi-hole or your router:

- Domain: `go`
- IP: `<PI_IP>` (your Raspberry Pi IP)

**That's it!** Visit `http://go/` from any device on your network.

---

## Configuration

Configure via environment variables (defaults work for most setups):

| Variable        | Default                 | Description             |
| --------------- | ----------------------- | ----------------------- |
| `PIHOLE_TARGET` | `http://127.0.0.1:8080` | Pi-hole proxy target    |
| `LISTEN_ADDR`   | `:80`                   | Listen address and port |
| `DB_FILE`       | `go_links.json`         | Database file path      |

### Setting Environment Variables

Edit the systemd service file directly:

```bash
sudo systemctl edit golink
```

Add your configuration:

```ini
[Service]
Environment="PIHOLE_TARGET=http://127.0.0.1:8080"
Environment="LISTEN_ADDR=:80"
Environment="DB_FILE=/opt/golink/links.json"
```

Then reload:

```bash
sudo systemctl daemon-reload
sudo systemctl restart golink
```

---

## Usage

**Create a link:**

1. Visit `http://go/`
2. Enter shortcut name (e.g., `yt`)
3. Enter URL (e.g., `https://youtube.com`)
4. Click "Create"

**Use your link:**

- Navigate to `http://go/yt`

**Auto-create links:**

- Type `http://go/newlink` - if it doesn't exist, you'll be redirected to the creation form with the name pre-filled

---

## Management

```bash
# View logs
sudo journalctl -u golink -f

# Restart service
sudo systemctl restart golink

# Stop service
sudo systemctl stop golink

# Check status
sudo systemctl status golink
```

---

## Backup

Your links are stored in `/opt/golink/go_links.json`:

```bash
# Backup
sudo cp /opt/golink/go_links.json ~/go_links_backup.json

# Restore
sudo cp ~/go_links_backup.json /opt/golink/go_links.json
sudo systemctl restart golink
```

---

## Troubleshooting

**Service won't start:**

```bash
# Check logs
sudo journalctl -u golink -n 50

# Verify port 80 is free
sudo lsof -i :80

# If Pi-hole is on port 80, go back to step 1
```

**Can't access go/ links:**

```bash
# Test DNS
nslookup go

# Test locally on Pi
curl http://localhost/

# Check service
sudo systemctl status golink
```

**Can't access Pi-hole admin:**

```bash
# Verify Pi-hole is on 8080
sudo netstat -tlnp | grep pihole-FTL

# Should show port 8080, not 80
```

---

## Updating

```bash
# On your PC: rebuild
./build.sh
scp golink pi@<PI_IP>:/tmp/

# On Raspberry Pi: replace binary
ssh pi@<PI_IP>
sudo systemctl stop golink
sudo mv /tmp/golink /usr/local/bin/golink
sudo chmod +x /usr/local/bin/golink
sudo systemctl start golink
```
