# Unraid Installation Guide for ProxyServer

This guide explains how to install and run **ProxyServer Gateway & Cloudflare DDNS** on your Unraid server using the custom template and Alpine-based container.

![ProxyServer Logo](../assets/logo.png)

---

## 🚀 Installation Options on Unraid

### Option 1: Direct XML Template Import (Recommended)

1. Open a terminal on your Unraid server (via Unraid WebGUI Terminal or SSH).
2. Copy [`ProxyServer.xml`](ProxyServer.xml) to your Unraid templates directory:
   ```bash
   curl -sSL https://raw.githubusercontent.com/sebmak/ProxyServer/main/unraid/ProxyServer.xml \
     -o /boot/config/plugins/dockerMan/templates-user/my-ProxyServer.xml
   ```
3. In the Unraid WebGUI, go to the **Docker** tab.
4. Click **Add Container** at the bottom.
5. In the **Template** dropdown, select **ProxyServer**.
6. Review the settings:
   - **Web Management UI Port**: `8080` (or your preferred host port)
   - **HTTP Reverse Proxy Port**: `9080` (or `80` if not used by Unraid WebGUI)
   - **HTTPS Reverse Proxy Port**: `9443` (or `443` if not used by Unraid WebGUI)
   - **AppData Storage Path**: `/mnt/user/appdata/proxyserver`
   - **PUID / PGID**: `99` / `100` (`nobody:users` on Unraid)
7. Click **Apply**. Unraid will download/build and launch the container!
8. Click on the container icon and select **WebUI** to complete initial setup at `http://[tower-ip]:8080`.

---

### Option 2: Build Locally on Unraid

If you prefer building the Alpine container image directly on your Unraid host:

```bash
cd /mnt/user/appdata
git clone https://github.com/sebmak/ProxyServer.git proxyserver-repo
cd proxyserver-repo
docker build -t proxyserver:latest .
```

Then in Unraid Docker GUI:
- **Repository**: `proxyserver:latest`
- Add ports `8080`, `9080`, `9443`
- Add path `/data` -> `/mnt/user/appdata/proxyserver`

---

### Option 3: Docker Compose on Unraid (via Docker Compose Manager Plugin)

If you have the **Docker Compose Manager** plugin installed in Unraid Community Applications:

```yaml
version: '3.8'

services:
  proxyserver:
    image: proxyserver:latest
    build: .
    container_name: proxyserver
    restart: unless-stopped
    ports:
      - "8080:8080" # Web Management UI
      - "9080:9080" # HTTP Reverse Proxy
      - "9443:9443" # HTTPS Reverse Proxy
    environment:
      - PUID=99
      - PGID=100
      - DATA_DIR=/data
    volumes:
      - /mnt/user/appdata/proxyserver:/data
```

---

## 📁 Migrating Legacy `servers.json` on Unraid

If you have an existing `servers.json` from `proxy_server_old`:
1. Copy `servers.json` to `/mnt/user/appdata/proxyserver/servers.json` before starting the container.
2. ProxyServer will automatically detect and import your targets on its first boot!
3. You can also re-import at any time from **Settings & Security -> Re-import targets from legacy servers.json**.
