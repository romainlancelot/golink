<h1>Go Links - URL Shortener for Raspberry</h1>

A lightweight URL shortener that runs on your Raspberry Pi Model B alongside Pi-hole v6. Access your favorite sites with simple shortcuts like `go/youtube`, `go/github`, etc.

- [Features](#features)
- [Requirements](#requirements)
- [🏁 Quick Start Guide](#-quick-start-guide)
  - [Step 1: Prepare Pi-hole v6 (On Raspberry Pi)](#step-1-prepare-pi-hole-v6-on-raspberry-pi)
  - [Step 2: Build the Application (On Your PC)](#step-2-build-the-application-on-your-pc)
  - [Step 3: Install on Raspberry Pi (On Raspberry Pi)](#step-3-install-on-raspberry-pi-on-raspberry-pi)
  - [Step 4: Configure DNS (On Raspberry Pi)](#step-4-configure-dns-on-raspberry-pi)
- [🎉 Testing](#-testing)
- [📋 Configuration Options](#-configuration-options)
  - [Environment Variables](#environment-variables)
- [🔧 Management Commands](#-management-commands)
- [💾 Backup and Restore](#-backup-and-restore)
  - [Backup](#backup)
  - [Restore](#restore)
- [🛠️ Troubleshooting](#️-troubleshooting)
  - [Service won't start](#service-wont-start)
  - [Can't access `go/` links](#cant-access-go-links)
  - [Pi-hole admin not accessible](#pi-hole-admin-not-accessible)
  - [Permission denied errors](#permission-denied-errors)
  - [Links not persisting after reboot](#links-not-persisting-after-reboot)
- [🔄 Updating](#-updating)
- [🔒 Security Considerations](#-security-considerations)
- [📚 Advanced Configuration](#-advanced-configuration)
  - [Using a Different Port (e.g., 8000)](#using-a-different-port-eg-8000)
  - [Custom Database Location](#custom-database-location)
  - [Multiple Domain Support](#multiple-domain-support)
- [📖 Usage Examples](#-usage-examples)
  - [Create Common Links](#create-common-links)
  - [Access from Different Devices](#access-from-different-devices)
  - [Non-existent Link Behavior](#non-existent-link-behavior)
- [🐛 Known Issues \& Limitations](#-known-issues--limitations)
- [🤝 Contributing](#-contributing)
- [📄 Project Files](#-project-files)
- [🙏 Acknowledgments](#-acknowledgments)

# Features

- **Custom short links**: Create memorable shortcuts for any URL
- **Pi-hole v6 integration**: Transparent proxy to Pi-hole for DNS management
- **Web interface**: Simple UI to create and manage links
- **Persistent storage**: Links saved to JSON file
- **Graceful shutdown**: No data loss on restart
- **Environment configuration**: Easy setup via `.env` file

# Requirements

- Raspberry Pi Model B (ARMv6)
- Go 1.21+ installed on your PC (for building from source)
- Pi-hole v6 running on your Raspberry Pi
- Root/sudo access on Raspberry Pi (to bind to port 80)

---

# 🏁 Quick Start Guide

This guide separates what you do on your **PC** (compilation) and on your **Raspberry Pi** (configuration).

## Step 1: Prepare Pi-hole v6 (On Raspberry Pi)

Since we'll use port 80 for `go/` links, we need to move Pi-hole's web interface to port 8080.

1. SSH into your Raspberry Pi

2. Edit the configuration file:

   ```bash
   sudo vim /etc/pihole/pihole.toml
   ```

3. Find the `[webserver]` section and modify the port:

   ```toml
   [webserver]
   port = "8080o,443os,[::]:8080o,[::]:443os"
   ```

4. Restart Pi-hole:

   ```bash
   sudo systemctl restart pihole-FTL
   ```

5. **Verify Pi-hole is now on port 8080**:

   ```bash
   # Check which ports Pi-hole is listening on
   sudo netstat -tlnp | grep pihole-FTL

   # Should show port 8080, NOT port 80
   # Example output: tcp 0 0 0.0.0.0:8080 0.0.0.0:* LISTEN 1234/pihole-FTL

   # Test Pi-hole on port 8080
   curl -I http://localhost:8080/admin/

   # Port 80 should be free now
   sudo lsof -i :80
   # Should return nothing or only show golink later
   ```

   **Important**: If Pi-hole is still on port 80, the configuration change didn't work. Check the `pihole.toml` file syntax.

---

## Step 2: Build the Application (On Your PC)

Make sure **Go 1.21+** is installed on your PC.

1. **Download or clone this project** to your PC

2. **Run the build script**:

   ```bash
   chmod +x build.sh
   ./build.sh
   ```

   The script will compile the binary for Raspberry Pi Model B (ARMv6).

3. **Transfer files to your Raspberry Pi**:

   ```bash
   # Transfer the binary and service file
   scp golink golink.service pi@<PI_IP>:/home/pi/
   ```

   **Note**: The `golink.service` file is provided in the project - no need to create it.

4. **(Optional) Create and transfer the .env file** if you want to customize settings:

   ```bash
   cp .env.example .env
   # Edit .env if needed, then:
   scp .env pi@<PI_IP>:/home/pi/
   ```

---

## Step 3: Install on Raspberry Pi (On Raspberry Pi)

1. **SSH into your Raspberry Pi**

2. **Create the application directory and move files**:

   ```bash
   # Create application directory
   sudo mkdir -p /opt/golink

   # Move binary to system location
   sudo mv /home/pi/golink /usr/local/bin/
   sudo chmod +x /usr/local/bin/golink
   sudo chown root:root /usr/local/bin/golink

   # Move .env file if you created one (optional)
   sudo mv /home/pi/.env /opt/golink/ 2>/dev/null || true

   # Set proper ownership for application directory
   sudo chown -R root:root /opt/golink
   sudo chmod 755 /opt/golink
   ```

3. **Install the systemd service**:

   **Option A - Copy the provided file** (recommended):

   ```bash
   sudo cp /home/pi/golink.service /etc/systemd/system/
   ```

   **Option B - Create manually**:

   ```bash
   sudo vim /etc/systemd/system/golink.service
   ```

   Paste this configuration:

   ```ini
   [Unit]
   Description=Go Links URL Shortener
   After=network.target pihole-FTL.service
   Wants=pihole-FTL.service

   [Service]
   Type=simple
   User=root
   WorkingDirectory=/opt/golink
   EnvironmentFile=-/opt/golink/.env
   ExecStart=/usr/local/bin/golink
   Restart=always
   RestartSec=5

   [Install]
   WantedBy=multi-user.target
   ```

4. **Enable and start the service**:

   ```bash
   sudo systemctl daemon-reload
   sudo systemctl enable golink
   sudo systemctl start golink
   ```

5. **Check the service status**:

   ```bash
   sudo systemctl status golink
   ```

   You should see: `Active: active (running)`

6. **Verify everything is working**:

   ```bash
   # Check that golink is listening on port 80
   sudo lsof -i :80
   # Should show: golink with PID
   
   # Check that Pi-hole is on 8080 (not 80)
   sudo netstat -tlnp | grep pihole-FTL
   # Should show: port 8080, NOT 80
   
   # Test golink from the Pi itself
   curl -I http://localhost/
   # Should return HTTP/200 with Go Links HTML
   ```

   **If golink is not running**, check the logs:
   
   ```bash
   sudo journalctl -u golink -n 50 --no-pager
   ```

---

## Step 4: Configure DNS (On Raspberry Pi)

Tell your network that `go` points to your Raspberry Pi.

1. Access Pi-hole admin: `http://<PI_IP>:8080/admin` (note the **:8080**)
2. Navigate to **Local DNS** → **DNS Records**
3. Add a new record:
   - **Domain**: `go`
   - **IP Address**: `<PI_IP>` (your Raspberry Pi's IP address)
4. Click **Add**

**Optional**: Also add `go.local` pointing to the same IP for redundancy.

---

# 🎉 Testing

1. **Open the management interface**:
   - From any device on your network, go to: `http://go/`
   - You should see the Go Links interface

2. **Create a test link**:
   - Shortcut name: `test`
   - URL: `https://google.com`
   - Click "Create Link"

3. **Use your link**:
   - Navigate to: `http://go/test`
   - You should be redirected to Google

4. **Verify Pi-hole proxy works**:
   - Go to: `http://<PI_IP>/admin` (without the :8080)
   - You should be proxied to Pi-hole automatically

---

# 📋 Configuration Options

## Environment Variables

Create a `.env` file in `/opt/golink/` (optional, defaults work for most setups):

```bash
# Pi-hole v6 proxy target (where Pi-hole web interface is running)
PIHOLE_TARGET=http://127.0.0.1:8080

# Listen address and port (80 requires root)
LISTEN_ADDR=:80

# Database file path for storing links
DB_FILE=go_links.json
```

---

# 🔧 Management Commands

```bash
# View live logs
sudo journalctl -u golink -f

# Restart service
sudo systemctl restart golink

# Stop service
sudo systemctl stop golink

# Start service
sudo systemctl start golink

# Check status
sudo systemctl status golink

# Disable autostart
sudo systemctl disable golink

# Re-enable autostart
sudo systemctl enable golink
```

---

# 💾 Backup and Restore

Your links are stored in `/opt/golink/go_links.json` by default.

## Backup

```bash
sudo cp /opt/golink/go_links.json /opt/golink/go_links.json.backup
# Or download to your PC
scp pi@<PI_IP>:/opt/golink/go_links.json ./go_links_backup.json
```

## Restore

```bash
sudo cp /opt/golink/go_links.json.backup /opt/golink/go_links.json
sudo systemctl restart golink
# Or upload from your PC
scp ./go_links_backup.json pi@<PI_IP>:/tmp/
ssh pi@<PI_IP> "sudo mv /tmp/go_links_backup.json /opt/golink/go_links.json && sudo systemctl restart golink"
```

---

# 🛠️ Troubleshooting

## Service won't start

```bash
# Check detailed logs
sudo journalctl -u golink -n 50 --no-pager

# Common issue: CHDIR error (status=200/CHDIR)
# This means the WorkingDirectory doesn't exist or isn't accessible
sudo mkdir -p /opt/golink
sudo chown root:root /opt/golink
sudo chmod 755 /opt/golink
sudo systemctl restart golink

# Check if port 80 is already in use (likely Pi-hole)
sudo lsof -i :80
sudo netstat -tlnp | grep :80

# If port 80 is occupied by pihole-FTL:
# Pi-hole wasn't moved to port 8080 - go back to Step 1
# Verify Pi-hole configuration:
sudo cat /etc/pihole/pihole.toml | grep -A 2 "\[webserver\]"
# Should show: port = "8080o,443os,[::]:8080o,[::]:443os"

# Try manual start to see errors
cd /opt/golink
sudo /usr/local/bin/golink
```

## Can't access `go/` links

1. **Check DNS resolution**:

   ```bash
   nslookup go
   # Should return your Pi's IP
   ```

2. **Verify service is running**:

   ```bash
   sudo systemctl status golink
   ```

3. **Check from the Pi itself**:

   ```bash
   curl http://localhost/
   # Should return HTML
   ```

4. **Verify firewall isn't blocking**:
   ```bash
   sudo iptables -L -n | grep 80
   ```

## Pi-hole admin not accessible

1. **Check Pi-hole is running**:

   ```bash
   pihole status
   sudo systemctl status pihole-FTL
   ```

2. **Verify Pi-hole is on port 8080**:

   ```bash
   sudo netstat -tlnp | grep 8080
   # Should show pihole-FTL listening

   # Also check it's NOT on port 80
   sudo netstat -tlnp | grep pihole-FTL
   ```

3. **If Pi-hole is still on port 80**:

   ```bash
   # Check the configuration
   sudo cat /etc/pihole/pihole.toml | grep -A 5 "\[webserver\]"

   # Expected output:
   # [webserver]
   # port = "8080o,443os,[::]:8080o,[::]:443os"

   # If different, edit again:
   sudo vim /etc/pihole/pihole.toml
   # Find [webserver] section and set:
   # port = "8080o,443os,[::]:8080o,[::]:443os"

   # Restart Pi-hole
   sudo systemctl restart pihole-FTL

   # Wait 5 seconds, then verify
   sleep 5
   sudo netstat -tlnp | grep pihole-FTL
   ```

4. **Check PIHOLE_TARGET in .env**:

   ```bash
   sudo cat /opt/golink/.env
   # Should match where Pi-hole actually runs
   ```

5. **Test direct access**:

   ```bash
   curl http://localhost:8080/admin/
   # Should return HTML
   ```

## Permission denied errors

```bash
# Ensure binary is executable and owned by root
sudo chmod +x /usr/local/bin/golink
sudo chown root:root /usr/local/bin/golink

# Ensure working directory exists and is accessible
sudo mkdir -p /opt/golink
sudo ls -la /opt/golink/
sudo chown root:root /opt/golink
sudo chmod 755 /opt/golink

# Check service user is root (required for port 80)
sudo systemctl cat golink | grep User
# Should show: User=root
```

## Links not persisting after reboot

```bash
# Check if JSON file exists and is writable
sudo ls -la /opt/golink/go_links.json

# Check logs for save errors
sudo journalctl -u golink | grep -i error

# Verify WorkingDirectory in service
sudo systemctl cat golink | grep WorkingDirectory
# Should be: WorkingDirectory=/opt/golink
```

---

# 🔄 Updating

When you make changes to the code:

1. **On your PC**: Rebuild the binary

   ```bash
   ./build.sh
   ```

2. **Transfer to Pi**:

   ```bash
   scp golink pi@<PI_IP>:/home/pi/golink-new
   ```

3. **On Raspberry Pi**: Replace and restart

   ```bash
   sudo systemctl stop golink
   sudo mv /home/pi/golink-new /usr/local/bin/golink
   sudo chmod +x /usr/local/bin/golink
   sudo systemctl start golink
   sudo systemctl status golink
   ```

---

# 🔒 Security Considerations

- **Port 80**: Requires root privileges. The service runs as root to bind to port 80
- **Input validation**: Application validates keys (alphanumeric + `-_`) and URLs (http/https only)
- **XSS Protection**: All user inputs are HTML-escaped before display
- **Network access**: Accessible only from your local network (LAN)
- **HTTPS**: Not included by default. Use nginx or Caddy reverse proxy if external access needed
- **Firewall**: Ensure your Pi's firewall allows port 80 if configured

---

# 📚 Advanced Configuration

## Using a Different Port (e.g., 8000)

If you don't want to run as root, use port 8000 and configure nginx as reverse proxy:

1. **Modify .env**:

   ```bash
   LISTEN_ADDR=:8000
   ```

2. **Run as regular user** (modify service User to `pi`)

3. **Install nginx**:

   ```bash
   sudo apt install nginx
   ```

4. **Configure nginx** (`/etc/nginx/sites-available/golink`):

   ```nginx
   server {
       listen 80;
       server_name go go.local;

       location / {
           proxy_pass http://127.0.0.1:8000;
           proxy_set_header Host $host;
           proxy_set_header X-Real-IP $remote_addr;
       }
   }
   ```

5. **Enable and restart**:

   ```bash
   sudo ln -s /etc/nginx/sites-available/golink /etc/nginx/sites-enabled/
   sudo systemctl restart nginx
   ```

## Custom Database Location

```bash
# In /opt/golink/.env
DB_FILE=/var/lib/golink/links.json

# Create directory
sudo mkdir -p /var/lib/golink
sudo chown root:root /var/lib/golink
```

## Multiple Domain Support

Modify the code to handle additional domains by editing the condition:

```go
if host == "go" || host == "go.local" || host == "links" {
    handleGo(w, r)
}
```

Then add DNS records for each domain in Pi-hole.

---

# 📖 Usage Examples

## Create Common Links

```
go/gh      → https://github.com
go/mail    → https://gmail.com
go/drive   → https://drive.google.com
go/cal     → https://calendar.google.com
go/meet    → https://meet.google.com
go/pihole  → http://<PI_IP>:8080/admin
go/router  → http://192.168.1.1
```

## Access from Different Devices

- **Desktop Browser**: `http://go/gh`
- **Mobile Browser**: `http://go/mail`
- **Terminal**: `curl -L http://go/test`

## Non-existent Link Behavior

If you type `http://go/newlink` and it doesn't exist:

- You'll be redirected to `http://go/?key=newlink`
- The form will be pre-filled with "newlink"
- Enter the URL and create the link instantly

---

# 🐛 Known Issues & Limitations

1. **DNS Caching**: Some devices cache DNS. After adding the `go` record, you may need to:
   - Flush DNS cache on your device
   - Restart Wi-Fi connection
   - Wait a few minutes

2. **HTTPS**: The service serves HTTP only. Browsers may show "Not Secure" warnings.

3. **Link Deletion**: Currently no web UI to delete links. To remove:

   ```bash
   # Edit JSON file manually
   sudo vim /opt/golink/go_links.json
   # Remove the line with the unwanted link, then save (:wq)
   sudo systemctl restart golink
   ```

4. **No Authentication**: Anyone on your network can create links. Consider adding authentication if needed.

---

# 🤝 Contributing

Improvements and suggestions welcome! Feel free to:

- Report issues
- Suggest features
- Submit pull requests
- Share your setup

---

# 📄 Project Files

```bash
golink/
├── main.go              # Main application with Go best practices
├── build.sh             # Build script for Raspberry Pi Model B (ARMv6)
├── golink.service       # Systemd service for Pi-hole v6
├── .env.example         # Environment configuration template
├── .gitignore           # Git ignore rules
└── README.md            # This guide
```

---

# 🙏 Acknowledgments

- Inspired by corporate "go links" systems (Google, Slack, etc.)
- Built for the Raspberry Pi Model B and Pi-hole v6 community
- Thanks to the Go programming language team

---

**Made with ❤️ for Raspberry Pi Model B**

**Questions?** Check the Troubleshooting section or review the systemd logs with `sudo journalctl -u golink -f`
