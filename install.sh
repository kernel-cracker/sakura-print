#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (C) 2026 PulseRoot
# Sakura Print installer
#   ./install.sh              build and install for your user (asks for your password only to install tools)
#   ./install.sh --no-deps    skip the package manager part (Go and the printing tools must be there already)
#   ./install.sh --office     also install LibreOffice, to print Word, Excel, PowerPoint and text files (big)
#   ./install.sh --uninstall  remove it
#   ./install.sh --print-build-env   show how it would be built for this processor, and stop
# Sakura Print is always built from source on your own computer, tuned to your processor: nothing ready-made.
set -o pipefail
cd "$(dirname "$(readlink -f "$0")")" || exit 1

G=$'\e[32m' Y=$'\e[33m' R=$'\e[31m' B=$'\e[1m' N=$'\e[0m'
step() { printf '%s\n' "${B}==>${N} $*"; }
ok()   { printf '%s\n' "  ${G}✔${N} $*"; }
warn() { printf '%s\n' "  ${Y}!${N} $*"; }
die()  { printf '%s\n' "  ${R}✘${N} $*" >&2; exit 1; }

DEPS=1 UNINSTALL=0 OFFICE=0 PRINTENV=0
for a in "$@"; do
  case $a in
    --no-deps) DEPS=0;; --uninstall) UNINSTALL=1;; --office) OFFICE=1;; --print-build-env) PRINTENV=1;;
    --build) ;; # always builds now; kept so old instructions still work
    -h|--help) sed -n '2,9p' "$0"; exit 0;;
    *) die "unknown option: $a";;
  esac
done

# build_env: how to build for this processor. Go can use newer instructions when told the computer has them
# (GOAMD64 levels on Intel/AMD, GOARM64 features on ARM); the program then only runs on computers like this one,
# which is fine: everyone builds their own. SAKURA_CPUINFO / SAKURA_ARCH pretend to be another computer (tests).
build_env() {
  local arch=${SAKURA_ARCH:-$(uname -m)} flags
  flags=" $(grep -m1 -E '^(flags|Features)[[:space:]]*:' "${SAKURA_CPUINFO:-/proc/cpuinfo}" 2>/dev/null | cut -d: -f2-) "
  has() { local f; for f in "$@"; do [[ $flags == *" $f "* ]] || return 1; done; }
  case $arch in
    x86_64|amd64)
      local level=v1
      if has cx16 lahf_lm popcnt pni sse4_1 sse4_2 ssse3; then
        level=v2
        if has avx avx2 bmi1 bmi2 f16c fma abm movbe xsave; then
          level=v3
          has avx512f avx512bw avx512cd avx512dq avx512vl && level=v4
        fi
      fi
      echo "GOARCH=amd64"; echo "GOAMD64=$level";;
    aarch64|arm64)
      local v=v8.0
      has atomics && v+=",lse"
      has aes pmull sha1 sha2 && v+=",crypto"
      echo "GOARCH=arm64"; echo "GOARM64=$v";;
    armv7*|armv8l) echo "GOARCH=arm"; echo "GOARM=7";;
    armv6*) echo "GOARCH=arm"; echo "GOARM=6";;
    i?86) echo "GOARCH=386";;
    *) echo "GOARCH=$(go env GOARCH 2>/dev/null)";;
  esac
}
if (( PRINTENV )); then build_env; exit 0; fi

BIN=$HOME/.local/bin
APPS=${XDG_DATA_HOME:-$HOME/.local/share}/applications
ICONS=${XDG_DATA_HOME:-$HOME/.local/share}/icons/hicolor/scalable/apps
UNIT=${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user/sakuraprint.service
AUTOSTART=${XDG_CONFIG_HOME:-$HOME/.config}/autostart/sakuraprint-server.desktop
PORT=8632

has_systemd_user() { command -v systemctl >/dev/null && systemctl --user show-environment >/dev/null 2>&1; }

if (( UNINSTALL )); then
  step "removing Sakura Print"
  if has_systemd_user; then systemctl --user disable --now sakuraprint.service >/dev/null 2>&1; fi
  pkill -f "$BIN/sakuraprint serve" 2>/dev/null
  rm -f "$BIN/sakuraprint" "$APPS/sakuraprint.desktop" "$ICONS/sakuraprint.svg" "$UNIT" "$AUTOSTART"
  ok "removed. Your settings are in ~/.config/sakuraprint and scans in ~/Documents/Scans (delete them if you want)"
  exit 0
fi

SUDO=""
if (( EUID != 0 )); then
  if command -v sudo >/dev/null; then SUDO=sudo; elif command -v doas >/dev/null; then SUDO=doas; fi
fi

# fmt: iPhone/Android photos (HEIC, AVIF) and other pictures (WebP, GIF, TIFF…); office: Word, Excel, PowerPoint, text
install_deps() {
  local req=() opt=() fmt=() office=()
  if command -v pacman >/dev/null; then
    step "Arch-based distro detected"
    req=(go qpdf librsvg cups poppler) opt=(sane sane-airscan xdg-user-dirs) fmt=(libheif imagemagick) office=(libreoffice-fresh)
    $SUDO pacman -S --needed "${req[@]}" || return 1
    $SUDO pacman -S --needed "${opt[@]}" || warn "scanner tools didn't install, printing still works"
    $SUDO pacman -S --needed avahi || warn "avahi didn't install: phones won't find the printer by themselves for printing from other apps"
    $SUDO pacman -S --needed "${fmt[@]}" || warn "photo converters didn't install: iPhone (HEIC) and WebP pictures won't open"
    (( OFFICE )) && { $SUDO pacman -S --needed "${office[@]}" || warn "LibreOffice didn't install: Word/Excel files won't open"; }
  elif command -v apt-get >/dev/null; then
    step "Debian/Ubuntu-based distro detected"
    req=(golang-go qpdf librsvg2-bin cups-client poppler-utils) opt=(sane-utils sane-airscan xdg-user-dirs) fmt=(libheif-examples imagemagick)
    office=(libreoffice-writer libreoffice-calc libreoffice-impress)
    $SUDO apt-get update || warn "apt-get update had problems, trying anyway"
    $SUDO apt-get install -y "${req[@]}" || return 1
    $SUDO apt-get install -y "${opt[@]}" || warn "scanner tools didn't install, printing still works"
    $SUDO apt-get install -y avahi-utils || warn "avahi didn't install: phones won't find the printer by themselves for printing from other apps"
    $SUDO apt-get install -y "${fmt[@]}" || warn "photo converters didn't install: iPhone (HEIC) and WebP pictures won't open"
    (( OFFICE )) && { $SUDO apt-get install -y --no-install-recommends "${office[@]}" || warn "LibreOffice didn't install: Word/Excel files won't open"; }
  elif command -v dnf >/dev/null; then
    step "Fedora-based distro detected"
    req=(golang qpdf librsvg2-tools cups-client poppler-utils) opt=(sane-backends sane-airscan xdg-user-dirs) fmt=(libheif-tools ImageMagick)
    office=(libreoffice-writer libreoffice-calc libreoffice-impress)
    $SUDO dnf install -y "${req[@]}" || return 1
    $SUDO dnf install -y "${opt[@]}" || warn "scanner tools didn't install, printing still works"
    $SUDO dnf install -y avahi-tools || warn "avahi didn't install: phones won't find the printer by themselves for printing from other apps"
    $SUDO dnf install -y "${fmt[@]}" || warn "photo converters didn't install: iPhone (HEIC) and WebP pictures won't open"
    (( OFFICE )) && { $SUDO dnf install -y "${office[@]}" || warn "LibreOffice didn't install: Word/Excel files won't open"; }
  elif command -v zypper >/dev/null; then
    step "openSUSE detected"
    req=(go qpdf rsvg-convert cups-client poppler-tools) opt=(sane-backends sane-airscan xdg-user-dirs)
    office=(libreoffice-writer libreoffice-calc libreoffice-impress)
    $SUDO zypper --non-interactive install "${req[@]}" || return 1
    $SUDO zypper --non-interactive install "${opt[@]}" || warn "scanner tools didn't install, printing still works"
    $SUDO zypper --non-interactive install avahi-utils || warn "avahi didn't install: phones won't find the printer by themselves for printing from other apps"
    $SUDO zypper --non-interactive install ImageMagick || warn "ImageMagick didn't install: WebP and other pictures won't open"
    # openSUSE's own libheif can't decode iPhone HEIC (the HEVC codec came from Packman, which is shutting down at
    # the end of 2026), so this only works where Packman is already set up
    $SUDO zypper --non-interactive install heif-examples 2>/dev/null ||
      warn "iPhone HEIC files on this computer may not open (openSUSE lacks the HEVC codec). Photos sent from the iPhone's photo picker still work: it converts them".
    (( OFFICE )) && { $SUDO zypper --non-interactive install "${office[@]}" || warn "LibreOffice didn't install: Word/Excel files won't open"; }
  else
    warn "couldn't recognise your package manager"
    warn "please install: Go, qpdf, rsvg-convert (librsvg), CUPS client tools, pdftoppm (poppler)"
    warn "for scanning also: sane + sane-airscan; for printing from other apps: avahi (avahi-publish-service)"
    warn "for iPhone photos and other pictures: libheif tools + ImageMagick; for Word/Excel files: LibreOffice"
  fi
}

if (( DEPS )); then install_deps || die "installing the printing tools failed, see above"; fi

step "installing Sakura Print"
mkdir -p "$BIN" "$APPS" "$ICONS" || die "can't create folders in your home"
# build it here, for this computer
command -v go >/dev/null || die "building needs Go: install it with your package manager (go / golang / golang-go), or run ./install.sh without --no-deps"
step "building Sakura Print for this computer"
mapfile -t tune < <(build_env)
# go.mod names the Go version the code needs; an older Go fetches exactly that version from the Go project
# (checked against Go's public checksum database). Some distributions switch this off, so it's switched on here.
export GOTOOLCHAIN=auto GOFLAGS=-mod=mod CGO_ENABLED=0
[ "$(go env GOPROXY)" = "off" ] && export GOPROXY=https://proxy.golang.org,direct
[ "$(go env GOSUMDB)" = "off" ] && export GOSUMDB=sum.golang.org
info="built $(date +%Y-%m-%d) with $(go version | awk '{print $3}') for ${tune[*]}"
env "${tune[@]}" go build -trimpath -ldflags "-s -w -X 'main.buildInfo=$info'" -o "$BIN/sakuraprint.new" . || die "the build failed, see above"
mv -f "$BIN/sakuraprint.new" "$BIN/sakuraprint"
ok "built for this processor (${tune[*]})"
ok "$BIN/sakuraprint"
install -m644 assets/icon.svg "$ICONS/sakuraprint.svg"
cat > "$APPS/sakuraprint.desktop" <<EOF
[Desktop Entry]
Type=Application
Name=Sakura Print
GenericName=Print & Scan
Comment=Print documents and photos, scan, copy
Exec=$BIN/sakuraprint
Icon=sakuraprint
Terminal=false
Categories=Office;Graphics;Printing;
Keywords=print;printer;scan;copy;photo;pdf;
StartupWMClass=SakuraPrint
EOF
command -v update-desktop-database >/dev/null && update-desktop-database "$APPS" >/dev/null 2>&1
ok "added \"Sakura Print\" to your app menu"

# keep the server running so phones can use it any time
pkill -f "$BIN/sakuraprint serve" 2>/dev/null
if has_systemd_user; then
  mkdir -p "$(dirname "$UNIT")"
  cat > "$UNIT" <<EOF
[Unit]
Description=Sakura Print (print & scan web app)
After=network-online.target

[Service]
ExecStart=$BIN/sakuraprint serve -port $PORT
Restart=on-failure

[Install]
WantedBy=default.target
EOF
  systemctl --user daemon-reload && systemctl --user enable --now sakuraprint.service >/dev/null 2>&1 \
    && ok "starts automatically when you log in" || warn "couldn't enable auto-start, run it from the app menu instead"
  systemctl --user restart sakuraprint.service >/dev/null 2>&1
else
  mkdir -p "$(dirname "$AUTOSTART")"
  cat > "$AUTOSTART" <<EOF
[Desktop Entry]
Type=Application
Name=Sakura Print server
Exec=$BIN/sakuraprint serve -port $PORT
NoDisplay=true
X-GNOME-Autostart-enabled=true
EOF
  (setsid "$BIN/sakuraprint" serve -port $PORT >/dev/null 2>&1 &)
  ok "starts automatically when you log in"
fi

step "checking"
for c in qpdf rsvg-convert pdftoppm lp lpstat lpoptions; do
  if command -v "$c" >/dev/null; then ok "$c"; else warn "$c is missing, printing won't work until it's installed"; fi
done
if command -v scanimage >/dev/null; then ok "scanimage (scanning)"; else warn "scanimage missing: scanning with the printer won't work (the phone camera still does)"; fi
if command -v heif-dec >/dev/null || command -v heif-convert >/dev/null || command -v magick >/dev/null; then ok "iPhone photos (HEIC)"; else warn "no HEIC converter: iPhone photos won't open (install libheif tools)"; fi
if command -v magick >/dev/null || command -v convert >/dev/null || command -v vips >/dev/null; then ok "WebP, GIF, TIFF pictures"; else warn "no ImageMagick: WebP/GIF/TIFF pictures won't open"; fi
if ! command -v avahi-publish-service >/dev/null; then
  warn "no avahi: with \"Print from any app\" on, phones won't find the printer by themselves"
elif command -v systemctl >/dev/null && ! systemctl is-active --quiet avahi-daemon; then
  warn "avahi isn't running, so phones won't find the printer for \"Print from any app\". To start it:  sudo systemctl enable --now avahi-daemon"
else
  ok "print from any app (AirPrint / Android): phones find it by themselves"
fi
if command -v pkexec >/dev/null; then ok "printer drivers can be installed from the app (it asks for your password)"; else warn "no pkexec (polkit): Printers & drivers can't ask for the password, so it shows the commands to run instead"; fi
if command -v soffice >/dev/null || command -v libreoffice >/dev/null; then ok "Word, Excel, PowerPoint, text (LibreOffice)"; else warn "Word/Excel/PowerPoint files need LibreOffice: run ./install.sh --office"; fi

if command -v ufw >/dev/null && $SUDO ufw status 2>/dev/null | grep -q "Status: active"; then
  warn "your firewall (ufw) is on. So phones can connect, run:  sudo ufw allow $PORT/tcp  (and for printing from other apps: sudo ufw allow 5353/udp)"
elif command -v firewall-cmd >/dev/null && firewall-cmd --state >/dev/null 2>&1; then
  warn "your firewall is on. So phones can connect, run:  sudo firewall-cmd --permanent --add-port=$PORT/tcp --add-service=mdns && sudo firewall-cmd --reload"
fi

printf '\n'
"$BIN/sakuraprint" phone -port $PORT
printf '\n'
step "done! Open \"Sakura Print\" from your app menu"
