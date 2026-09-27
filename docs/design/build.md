# Building and installing

Sakura Print is **always built from source on the computer it runs on**: there are no ready-made programs. The
installer (`install.sh`) installs the tools it needs (Go, CUPS client tools, qpdf, poppler, librsvg, SANE,
avahi, picture converters) with the distribution's package manager, then builds.

## Tuned to the processor

`install.sh: build_env` reads `/proc/cpuinfo` and tells Go which instructions this processor has:

| Processor | Setting | Needs |
|---|---|---|
| Intel/AMD, any 64-bit | `GOAMD64=v1` | |
| … from about 2009 | `GOAMD64=v2` | SSE3, SSSE3, SSE4.1/4.2, POPCNT, CMPXCHG16B |
| … from about 2015 (Haswell, Zen) | `GOAMD64=v3` | AVX, AVX2, BMI1/2, F16C, FMA, LZCNT, MOVBE |
| … with AVX-512 | `GOAMD64=v4` | AVX-512 F, BW, CD, DQ, VL |
| 64-bit ARM (Raspberry Pi 4…) | `GOARM64=v8.0`, plus `,lse` (atomics) and `,crypto` (AES, SHA) when present | |
| 32-bit ARM | `GOARM=6` or `7` | |

The result only runs on computers with the same features, which is fine: everyone builds their own. `./install.sh
--print-build-env` shows the choice without building; `sakuraprint version` shows how a copy was built
(`main.buildInfo`, set by the installer).

## The Go version

`go.mod` names the Go version the code needs. If the distribution's Go is older, Go fetches exactly that version
from the Go project (`GOTOOLCHAIN=auto`), checked against Go's public checksum database; some distributions switch
this off, so the installer switches it on for the build. The build uses no outside Go libraries.

## What the installer does

1. Tools, through the package manager (skipped with `--no-deps`; LibreOffice only with `--office`).
2. Build (`CGO_ENABLED=0`, `-trimpath`, stripped) into `~/.local/bin/sakuraprint`.
3. App menu entry and icon; start at login (a systemd user service, or an autostart entry); restart the server.
4. Check: every tool found, driver installs possible (polkit), Bonjour running, firewall hints.

`./install.sh --uninstall` removes the program, menu entry and autostart; settings and scans stay.

## Tests

`tests/install_test.sh` gives the tuning pretend processors (old PCs, AVX2, AVX-512, Raspberry Pi 4 and 5, 32-bit
ARM) and checks there are no ready-made programs in the source.
