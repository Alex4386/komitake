# Setup on WSL2

> [!WARNING]  
> **Here be dragons!**  
> Due to Microsoft quirks on Linux on WSL2, this configuration is not really supported.  
> But you can at least try! Don't expect support for this though.

Komitake needs a real Linux Wi-Fi stack (`cfg80211` / `mac80211`, `nl80211`, a driver that can do **AP** mode). The stock WSL2 kernel does not give you that for a USB dongle. The usual escape hatch is a **custom WSL kernel 6.18**, **USB passthrough**, and a USB Wi-Fi adapter with a driver you can build as a module.

If any step sounds painful, it probably is. A bare-metal Linux PC or a Raspberry Pi is the path of least suffering. See [QUICKSTART.md](../QUICKSTART.md).

## Prerequisites

* Windows 10+ with WSL2 (WSL2, not WSL1)
* A **USB** Wi-Fi adapter (PCIe Wi-Fi cards do not work in WSL)
* Enough disk space and patience to build a kernel (~15–20 GB comfortable)
* Fast internet (kernel source + Komitake submodules)

Figure out which chipset you have (`lsusb`, the box, vendor docs) and which in-tree driver + firmware blob it needs. This guide uses **RTL8192EU** as the worked example because it is common. Your adapter may differ.

**Already built the kernel and just need to bring Wi-Fi up?** See [On startup](#on-startup).

## Overview

1. Install **usbipd-win** on Windows and pass the USB dongle into WSL.
2. Build the **Microsoft WSL2 kernel** (6.18) with `cfg80211` / `mac80211` and **your** Wi-Fi driver as **modules**. Embed firmware in the kernel image ([WSL firmware loading is broken](https://github.com/dorssel/usbipd-win/issues/390#issuecomment-1223615102)).
3. Point WSL at your `bzImage`, install the module tree, reboot WSL.
4. **On startup** (every session): attach USB from Windows, `modprobe` the wireless stack.
5. Install Komitake ([QUICKSTART.md](../QUICKSTART.md)), then set `wireless.interface` to whatever `ip link` shows.

## 1. Windows: USB passthrough

WSL2 does not see USB devices by default. Use [usbipd-win](https://github.com/dorssel/usbipd-win):

```powershell
winget install dorssel.usbipd-win
```

Plug in the dongle, then:

```powershell
usbipd list
usbipd bind --busid <BUSID>
usbipd attach --wsl --busid <BUSID>
```

You need to **attach again after every WSL restart** (and usually after unplugging the dongle). `usbipd list` shows whether the device is shared / attached.

Inside WSL, install the client tools once:

```sh
sudo apt update
sudo apt install linux-tools-virtual hwdata
sudo update-alternatives --install /usr/local/bin/usbip usbip "$(command -v usbip)" 20
```

If `usbip` is missing, your distro package name may differ. Search for `usbip` in `apt`.

## 2. Build a WSL2 kernel (6.18 + mac80211)

Microsoft ships a WSL-patched tree at [microsoft/WSL2-Linux-Kernel](https://github.com/microsoft/WSL2-Linux-Kernel). Use the branch that tracks **6.18** (name will look like `linux-msft-wsl-6.18.y` when it exists; pick the newest `6.18` tag/branch available).

On **WSL2** (or another Linux box with a cross-toolchain). Building inside WSL is fine on x86_64:

```sh
sudo apt update
sudo apt install build-essential flex bison libssl-dev libelf-dev bc dwarves pahole python3 rsync linux-firmware

git clone https://github.com/microsoft/WSL2-Linux-Kernel.git
cd WSL2-Linux-Kernel
git checkout linux-msft-wsl-6.18.y   # or the latest 6.18 WSL branch/tag

cp Microsoft/config-wsl .config
./scripts/config --module CFG80211
./scripts/config --module MAC80211
./scripts/config --disable FW_LOADER_STANDALONE
make olddefconfig
make menuconfig   # enable your Wi-Fi driver; double-check firmware options below
```

### Wireless stack (everyone)

Build these as **modules** (`M`), not built-in (`Y`). You load them with `modprobe` after each WSL start (see [On startup](#on-startup)).

| Location | Option | Build as |
|----------|--------|----------|
| `Networking support` → `Wireless` | **cfg80211** | **`M`** |
| same | **Generic IEEE 802.11 Networking Stack (mac80211)** | **`M`** |
| `Device Drivers` → `USB support` | USB host support | usually already on in `config-wsl` |

Then enable **your** driver's `CONFIG_*` option in `menuconfig` under `Device Drivers` → `Network device support` → `Wireless LAN`. Also turn on `RFKILL` if the driver wants it.

### Embed firmware (required on WSL)

[WSL's firmware resolution is broken](https://github.com/dorssel/usbipd-win/issues/390#issuecomment-1223615102). Putting files in `/lib/firmware` alone often is not enough. Bake the blob into the kernel binary instead.

1. Find the firmware path your driver requests (`dmesg` after a failed probe, the driver source, or files under `/lib/firmware` after `apt install linux-firmware`).
2. In `menuconfig` → `Device Drivers` → `Generic Driver Options`:

| Option | Setting |
|--------|---------|
| **Select only drivers that don't need compile-time external firmware** | **off** (`CONFIG_FW_LOADER_STANDALONE` disabled) |
| **Firmware loader** → **Build named firmware blobs into the kernel binary** | path to your `.bin` / `.fw` (see example below) |
| **Firmware blobs root directory** | `/lib/firmware`, or **`.`** if you copied the firmware tree into the kernel source |

Or use `scripts/config` before `make olddefconfig`:

```sh
./scripts/config --disable FW_LOADER_STANDALONE
./scripts/config --set-str EXTRA_FIRMWARE "<firmware-path-under-root>"
./scripts/config --set-str EXTRA_FIRMWARE_DIR "/lib/firmware"
```

### Example: RTL8192EU

Chipset **RTL8192EU** → in-tree driver **`rtl8xxxu`** (`CONFIG_RTL8XXXU`). Firmware: **`rtlwifi/rtl8192eu_nic.bin`**.

```sh
./scripts/config --module RTL8XXXU
./scripts/config --enable RTL8XXXU_UNTESTED
./scripts/config --set-str EXTRA_FIRMWARE "rtlwifi/rtl8192eu_nic.bin"
./scripts/config --set-str EXTRA_FIRMWARE_DIR "/lib/firmware"
```

Optional: copy the firmware tree into the kernel source and point `EXTRA_FIRMWARE_DIR` at `.`:

```sh
cp -r /lib/firmware/rtlwifi .
./scripts/config --set-str EXTRA_FIRMWARE_DIR "."
```

Kconfig reference: [`drivers/net/wireless/realtek/rtl8xxxu/Kconfig`](https://github.com/torvalds/linux/blob/master/drivers/net/wireless/realtek/rtl8xxxu/Kconfig).

### Build and install

```sh
make -j"$(nproc)"
sudo make modules_install
```

Copy the kernel image somewhere Windows can read:

```sh
mkdir -p /mnt/c/Users/<you>/wsl-kernel
cp arch/x86/boot/bzImage /mnt/c/Users/<you>/wsl-kernel/bzImage
```

`make modules_install` puts your `.ko` files under `/lib/modules/$(uname -r)/`. Re-run it whenever you rebuild the kernel.

## 3. Tell WSL to use your kernel

Create or edit `%USERPROFILE%\.wslconfig` on Windows:

```ini
[wsl2]
kernel=C:\\Users\\<you>\\wsl-kernel\\bzImage
```

Shut WSL down completely (from PowerShell or CMD):

```powershell
wsl --shutdown
```

Start your distro again. Confirm the running version:

```sh
uname -r
```

You should see a `6.18.x` WSL flavour string, not the old stock `-microsoft-standard-WSL2` build.

### systemd (recommended for Komitake)

Komitake's install path expects `systemctl`. Enable systemd in WSL in `/etc/wsl.conf`:

```ini
[boot]
systemd=true
```

Then `wsl --shutdown` and reopen the distro.

## 4. On startup

Do this **every time** you start WSL (and after `wsl --shutdown` or a Windows reboot). Microsoft will not remember your USB Wi-Fi setup for you.

**Windows (PowerShell):**

```powershell
usbipd attach --wsl --busid <BUSID>
```

**WSL** (load the wireless stack, then your driver module):

```sh
sudo modprobe cfg80211
sudo modprobe mac80211
sudo modprobe <your-driver>
dmesg | tail -20
lsmod
ip link
```

You want a `wlan0` (or `wlx…`) interface. If `modprobe` fails, the modules are missing or the wrong kernel is running. Recheck `make modules_install`, your driver `CONFIG_*` options, firmware embed, and `.wslconfig`.

**RTL8192EU example:**

```sh
sudo modprobe rtl8xxxu
lsmod | grep -E 'cfg80211|mac80211|rtl8xxxu'
```

### Check AP mode (SoftAP)

Komitake needs **AP** mode, not just Wi-Fi client:

```sh
iw list | grep -A10 "Supported interface modes"
```

Look for `* AP` under the phy that owns your dongle. If it is not there, you are gonna have a bad time. Welp! Time for finding drivers and trial and error!

## 5. Install Komitake

From here, follow [QUICKSTART.md](../QUICKSTART.md) inside WSL. Go, NodeSource, `./scripts/deps.sh`, `./INSTALL`, `systemctl`, `komitake set`, all of that.

WSL-specific reminders:

* Set the interface name to whatever showed up in `ip link`:
  ```sh
  sudo komitake set --wireless-interface=wlan0
  sudo systemctl restart komitake.service
  ```
* If **NetworkManager** touches the dongle, tell it to back off:
  ```sh
  sudo nmcli device set wlan0 managed no
  ```
* Re-attach the USB device and run the [On startup](#on-startup) `modprobe` steps after each `wsl --shutdown` or Windows reboot.

## Troubleshooting

| Symptom | Things to try |
|---------|----------------|
| `usbipd attach` fails | Run PowerShell as Administrator; `usbipd bind` first; only one WSL instance should own the device |
| No wireless adapter after attach | [On startup](#on-startup) `modprobe` chain; `dmesg`, `lsusb`, `lsmod`; confirm driver built as **`M`** and `make modules_install` ran; firmware embedded (`EXTRA_FIRMWARE`, `FW_LOADER_STANDALONE` off) |
| `iw` missing | `sudo apt install iw` |
| `hostapd` / AP won't start | `iw list` AP support; try another channel (`komitake set --wireless-channel=6`); check `journalctl -u komitake.service -e` |
| Still on stock kernel | `.wslconfig` path must use `\\`, `wsl --shutdown` after edits, `uname -r` to verify |
| Everything is cursed | Use native Linux instead. Seriously. |

Good luck. 🐉
