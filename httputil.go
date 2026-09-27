// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 PulseRoot

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
)

// Small HTTP helpers, and the computer's addresses on the WiFi.

// ---------- http helpers ----------

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func readJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(v)
}

// virtual network cards (containers, virtual machines) that a phone can never reach
var virtualIface = regexp.MustCompile(`^(docker|br-|veth|virbr|vmnet|vboxnet|lxc|lxd|incus|podman|cni|flannel|cali|kube|vnet)`)

// lanAddrs lists this computer's WiFi / network cable addresses
func lanAddrs() []net.IP {
	var out []net.IP
	ifaces, _ := net.Interfaces()
	for _, i := range ifaces {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 || virtualIface.MatchString(i.Name) {
			continue
		}
		addrs, _ := i.Addrs()
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok && ipn.IP.To4() != nil {
				out = append(out, ipn.IP.To4())
			}
		}
	}
	return out
}

func lanURLs(port int) []string {
	var urls []string
	for _, ip := range lanAddrs() {
		urls = append(urls, fmt.Sprintf("http://%s:%d", ip, port))
	}
	return urls
}
