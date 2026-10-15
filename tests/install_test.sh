#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (C) 2026 PulseRoot
# The installer builds Sakura Print from source, tuned to the computer's processor. This checks it picks the right
# tuning for different processors (from pretend /proc/cpuinfo files), without building or installing anything.
cd "$(dirname "$(readlink -f "$0")")/.." || exit 1
fail=0 pass=0
tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT

check() { # name arch cpuinfo-flags-or-features expected
  printf 'processor\t: 0\n%s\n' "$3" > "$tmp/cpuinfo"
  got=$(SAKURA_CPUINFO="$tmp/cpuinfo" SAKURA_ARCH="$2" ./install.sh --print-build-env 2>&1 | grep -E '^(GOAMD64|GOARM64|GOARCH)=' | tr '\n' ' ' | sed 's/ $//')
  if [ "$got" = "$4" ]; then pass=$((pass + 1)); echo "  PASS $1: $got"; else fail=$((fail + 1)); echo "  FAIL $1: want '$4', got '$got'"; fi
}

v1='flags		: fpu vme de pse tsc msr pae mce cx8 apic sep mtrr pge mca cmov pat pse36 clflush mmx fxsr sse sse2'
v2="$v1 ht syscall nx lm pni ssse3 cx16 sse4_1 sse4_2 popcnt lahf_lm"
v3="$v2 movbe xsave avx f16c fma abm bmi1 avx2 bmi2"
v4="$v3 avx512f avx512dq avx512cd avx512bw avx512vl"

echo "installer: build tuning"
check "an old PC (2008)"              x86_64 "$v1" "GOARCH=amd64 GOAMD64=v1"
check "a 2010s PC"                    x86_64 "$v2" "GOARCH=amd64 GOAMD64=v2"
check "a modern PC (AVX2)"            x86_64 "$v3" "GOARCH=amd64 GOAMD64=v3"
check "a PC with AVX-512"             x86_64 "$v4" "GOARCH=amd64 GOAMD64=v4"
check "AVX2 but no BMI2 (odd CPU)"    x86_64 "${v3/ bmi2/}" "GOARCH=amd64 GOAMD64=v2"
check "a Raspberry Pi 4"              aarch64 'Features	: fp asimd evtstrm crc32 cpuid' "GOARCH=arm64 GOARM64=v8.0"
check "a Raspberry Pi 5"              aarch64 'Features	: fp asimd evtstrm aes pmull sha1 sha2 crc32 atomics fphp asimdhp cpuid asimdrdm lrcpc dcpop asimddp' "GOARCH=arm64 GOARM64=v8.0,lse,crypto"
check "32-bit ARM"                    armv7l  'Features	: half thumb fastmult vfp edsp neon vfpv3 tls vfpv4 idiva idivt' "GOARCH=arm"
real=$(./install.sh --print-build-env 2>&1 | grep -E '^(GOAMD64|GOARM64)=')
if [ -n "$real" ]; then pass=$((pass + 1)); echo "  PASS this computer's real processor: $real"; else fail=$((fail + 1)); echo "  FAIL this computer's real processor: no tuning found"; fi

echo "installer: nothing ready-made is shipped"
if [ -d bin ] || grep -q 'bin/sakuraprint-' install.sh; then fail=$((fail + 1)); echo "  FAIL there are ready-made programs (bin/): everyone builds their own"; else pass=$((pass + 1)); echo "  PASS everyone builds their own"; fi

echo "installer: installing the tools, on pretend computers"
# Found by a real install: with every tool already there, the installer still said "installing the printing tools
# failed", on every distribution, unless --office was given. Here the real installer runs with stand-in package
# managers (they say yes to everything and write down what they were asked), a stand-in sudo, and a stand-in Go
# that stops it before anything is built or installed.
deps() { # name package-manager flags expected-in-log not-expected-in-log
  local pc="$tmp/pc-$2$3" got
  rm -rf "$pc"; mkdir -p "$pc/bin" "$pc/home"
  for t in bash env sh grep cut uname sed date awk mkdir readlink dirname cat tr head mv rm install; do
    ln -s "$(command -v "$t")" "$pc/bin/$t"
  done
  for t in "$2" sudo go; do
    printf '#!/bin/sh\necho "%s $*" >> "%s/log"\n' "$t" "$pc" > "$pc/bin/$t"
  done
  [ -n "$BROKEN" ] && echo "case \"\$*\" in *$BROKEN*) exit 1;; esac" >> "$pc/bin/$2"   # this package won't install
  echo 'exec "$@"' >> "$pc/bin/sudo"            # sudo runs what it's given
  echo 'case $1 in build) exit 1;; esac' >> "$pc/bin/go"   # stop before building anything
  chmod +x "$pc/bin/$2" "$pc/bin/sudo" "$pc/bin/go"
  got=$(HOME="$pc/home" XDG_DATA_HOME="$pc/home/.local/share" XDG_CONFIG_HOME="$pc/home/.config" PATH="$pc/bin" ./install.sh $3 2>&1)
  if [[ $got == *"installing the printing tools failed"* ]] || [[ $got != *"building Sakura Print"* ]]; then
    fail=$((fail + 1)); echo "  FAIL $1: the tools step didn't succeed"; printf '%s\n' "$got" | sed 's/^/      /' | tail -8; return
  fi
  if ! grep -q -- "$4" "$pc/log" || { [ -n "$5" ] && grep -q -- "$5" "$pc/log"; }; then
    fail=$((fail + 1)); echo "  FAIL $1: asked the package manager for the wrong things:"; sed 's/^/      /' "$pc/log"; return
  fi
  pass=$((pass + 1)); echo "  PASS $1"
}
for pm in pacman apt-get dnf zypper; do
  deps "$pm, everything there, without --office" "$pm" "" "qpdf" "libreoffice"
  deps "$pm, with --office" "$pm" "--office" "libreoffice" ""
done
# and a required tool that won't install still stops it, with the message (the safety net doesn't hide real failures)
for pm in pacman apt-get dnf zypper; do
  out=$(BROKEN=qpdf deps "x" "$pm" "" "qpdf" "" 2>&1)
  if [[ $out == *"FAIL x: the tools step didn't succeed"* && $out == *"installing the printing tools failed"* ]]; then
    pass=$((pass + 1)); echo "  PASS $pm, a required tool won't install: it stops and says so"
  else fail=$((fail + 1)); echo "  FAIL $pm, a required tool won't install: it carried on"; fi
done

echo "$pass passed, $fail failed"
[ $fail -eq 0 ]
