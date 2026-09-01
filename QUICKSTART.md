# Quickstart

Get your Fuji kart on your PC and driving in a few minutes. 

> [!IMPORTANT]  
> This guide skips the deep technical stuff, that might help you to troubleshoot or stuff that are good to know.  
> If you need more info see [USAGE.md](./USAGE.md) if you need that.

## Requirements

* a Linux PC to host Komitake
* a Wi-Fi adapter that supports `SoftAP` mode (creates its own kart network)
* a kart for "Build your Circuit at Home" game (the "Fuji" one, obviously)
* a web browser on the same PC
* an Xbox / PlayStation / generic gamepad (optional, but this guide assumes you want controls that uses one)

> [!NOTE]  
> **But I don't have Linux PC! I only have Windows!**   
> You can at least _"try"_ with **WSL2** with USB Wi-Fi adapter. but this is not supported. [Setup with WSL2](./docs/wsl2-setup.md)

## One-time install

> [!TIP]  
> If Komitake is already installed on your PC, skip to [Every time you race](#every-time-you-race).

### Install Go

`./INSTALL` builds Komitake from source and needs **Go 1.26 or newer**. Your Linux package manager may ship an older Go, so install the official toolchain:

1. Open [https://go.dev/dl/](https://go.dev/dl/) and download the **Linux** tarball for your PC (`amd64` for most desktops, `arm64` for many Raspberry Pi boards).
2. Extract it and put it on your `PATH`:

   ```sh
   sudo rm -rf /usr/local/go
   sudo tar -C /usr/local -xzf go1.26.*.linux-*.tar.gz
   echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.profile
   source ~/.profile
   go version
   ```

   The last command should print `go1.26` or newer.

### Install Node.js

`./INSTALL` embeds the web UI and needs **Node.js** with `npm`. Distro packages are often too old, so use [NodeSource](https://github.com/nodesource/distributions) instead. follow the directions on that repo.

### Get the source

Clone Komitake with submodules (needed for the patched `hostapd`):

```sh
git clone --recurse-submodules https://github.com/Alex4386/komitake
cd komitake
```

If you already cloned without submodules, run:

```sh
git submodule update --init --recursive
```

### Install everything else

From the Komitake folder, run:

```sh
./scripts/deps.sh
./INSTALL
sudo systemctl enable --now komitake.service
```

`./scripts/deps.sh` installs the other build and runtime tools (compiler, Wi-Fi build libs, `git`, `ffmpeg`, and VA-API drivers). It will ask for your password. Node.js is not included — install that from NodeSource first (see above).

`./INSTALL` builds and installs Komitake. The `systemctl` line starts it automatically whenever the PC boots.

### Configure Komitake

Use `komitake set` to change settings. Only the flags you pass are updated and gets updated. just like Tailscale!

1. **Secret (required after first install).** `./INSTALL` leaves a placeholder secret. Generate a random one before pairing:
   ```sh
   sudo komitake set --generate-secret
   sudo systemctl restart komitake.service
   ```
   To choose your own string instead, use `komitake set --secret='…'`.

2. **Admin socket (recommended)**   
   The daemon runs as root and creates a control socket at `/run/komitake.sock`. By default only root can use it, so `komitake web` and the web UI would need `sudo`. Open it to your user account:
   
   ```sh
   sudo komitake set --socket-chmod=0777
   sudo systemctl restart komitake.service
   ```

   After that, run `komitake web`, `komitake status`, and pairing from your normal user.  
   Free yourself from `sudo` on every command.

3. **Wi-Fi adapter.** 
   If your kart network interface is not `wlan0`, set the correct name (check with `ip link`):
   ```sh
   sudo komitake set --wireless-interface=wlan1
   sudo systemctl restart komitake.service
   ```

   If **NetworkManager** is running, tell it to leave that adapter alone (use the same interface name):
   ```sh
   sudo nmcli device set wlan0 managed no
   ```

4. **Web UI from another device on your home network** (optional). 
   By default the page listens on this PC only. To open it from a phone or tablet on the same LAN:
   
   ```sh
   sudo komitake set --web-bind=0.0.0.0:8080
   ```

   Then restart the web UI (`komitake web`). Live video on other devices may need HTTPS:
   
   ```sh
   sudo komitake set --web-tls-enabled=true
   ```

5. **Live camera hardware acceleration (optional).**  
   Komitake uses `ffmpeg` to transcode the kart camera.   
   The default (`auto`) picks a hardware encoder when it can. You only need to change this if video is missing, stutters, or the daemon logs a transcode error:

   ```sh
   # Intel / AMD integrated graphics (most Linux PCs)
   sudo komitake set --video-hwaccel=vaapi
   
   # NVIDIA GPU
   sudo komitake set --video-hwaccel=nvenc
   
   # Intel Quick Sync
   sudo komitake set --video-hwaccel=qsv
   
   sudo systemctl restart komitake.service
   ```

Run `komitake set --help` for the full list (`wireless-address`, `wireless-channel`, and more).

## Every time you race

1. **Turn on the kart** and make sure the PC is on the same machine running Komitake.
2. **Start the web UI** (in a terminal):

   ```sh
   komitake web
   ```

3. **Open the page** in your browser: [http://127.0.0.1:8080](http://127.0.0.1:8080)
4. **Pair the kart** (first time only):
   * click **Pair kart**
   * when the QR code appears, scan it with the kart
   * wait until pairing finishes — the dialog closes on its own
5. **Pick your kart** from the home screen.
6. **Turn on Drive mode** in the telemetry panel (the switch labeled **Drive mode**).
7. **Plug in your gamepad** and press any button so the browser notices it.
8. **Tune controls** (optional): **Settings menu → Controls → Gamepad**
   * **Trigger (Analog)** — like a racing sim: left stick steers, RT accelerates, LT reverses
   * **Button (Digital)** — like "the game": left stick steers, **A** accelerates, **B** reverses
9. **Drive.** Live camera video appears on the left; telemetry and inputs are on the right.

Keyboard works too: **W** / **S** or arrow keys to move, **Space** to center the sticks. Open **Settings menu → Help** for the full list.

## Quick tips

* **Permission denied on `komitake web` or `komitake status`?**  
  Run `sudo komitake set --socket-chmod=0777` and restart the daemon (`sudo systemctl restart komitake.service`).
* **Missing dependencies during `vite` building?**  
  It seems `WebUI` has been updated!  
  Try heading to `internal/web/frontend` and run `npm install`.
* **No video?**  
  Wait a few seconds after connecting — the stream starts once the kart camera is ready.  
  If it never appears, check daemon logs (`journalctl -u komitake.service -e`) and try an explicit encoder (`komitake set --video-hwaccel=vaapi` or `nvenc`, then restart the daemon).  
  From another device on the LAN, use HTTPS (`komitake set --web-tls-enabled=true`); on this PC, `http://127.0.0.1:8080` is simplest.
* **Video froze?**  
  If you have put your kart for idle without input for long time, the kart might disable `Drive Mode` on their own.  
  If then, turn the `Drive Mode` on (if already on, try turning it off and on) to get it working again.
* **Gamepad not detected?**  
  Press a button on the pad while the browser tab is focused. Drive mode must be on before inputs reach the kart.
* **Kart won't join?**  
  Confirm the daemon is running (`komitake status` or `systemctl status komitake.service`).  
  Check the Wi-Fi interface (`komitake set --wireless-interface=…` if needed). If **NetworkManager** manages that adapter, run `sudo nmcli device set <iface> managed no`. Then try pairing again from the web UI.
* **More karts?**  
  Pair each one once; switch between them from the kart picker in the top bar.  
  Due to SoftAP limitation and "fuji" pairing session, when in pairing mode, other karts disconnects until pairing mode ends.
* **Unknown disconnects?**  
  If you are using NetworkManager, that might be the reason that disconnects may occur, remove your interface from managed interfaces.  
  ```sh
  sudo nmcli device set <iface> managed no
  ```

For pairing from the terminal, live video in a separate window, and config details, see [USAGE.md](./USAGE.md).
