//go:build !js

package main

import (
	"os"
	"runtime/pprof"
)

// startProfile writes a CPU profile to the file NEXDIRECT_PROFILE names,
// for finding out where a frame's time goes; returns the stop function.
func startProfile() func() {
	path := os.Getenv("NEXDIRECT_PROFILE")
	if path == "" {
		return func() {}
	}
	f, err := os.Create(path)
	if err != nil {
		return func() {}
	}
	if err := pprof.StartCPUProfile(f); err != nil {
		_ = f.Close()
		return func() {}
	}
	return func() {
		pprof.StopCPUProfile()
		_ = f.Close()
	}
}
