package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"runtime/trace"
)

// startProfiling enables opt-in runtime profiling for a whole client run when
// RUNE_PROFILE_DIR names a directory. It writes cpu.pprof and trace.out while
// running, and heap.pprof, allocs.pprof, block.pprof, mutex.pprof, and
// goroutine.txt when the returned stop function runs. Unset, it does nothing.
//
//	RUNE_PROFILE_DIR=/tmp/rune-prof rune mud.example.com 4000
//	go tool pprof -http=:8080 /tmp/rune-prof/cpu.pprof
//	go tool trace /tmp/rune-prof/trace.out
func startProfiling() (stop func(), err error) {
	dir := os.Getenv("RUNE_PROFILE_DIR")
	if dir == "" {
		return func() {}, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	runtime.SetBlockProfileRate(10_000) // about one sampled event per 10µs spent blocked
	runtime.SetMutexProfileFraction(10)

	cpuFile, err := os.Create(filepath.Join(dir, "cpu.pprof"))
	if err != nil {
		return nil, err
	}
	if err := pprof.StartCPUProfile(cpuFile); err != nil {
		cpuFile.Close()
		return nil, err
	}
	traceFile, err := os.Create(filepath.Join(dir, "trace.out"))
	if err != nil {
		pprof.StopCPUProfile()
		cpuFile.Close()
		return nil, err
	}
	if err := trace.Start(traceFile); err != nil {
		pprof.StopCPUProfile()
		cpuFile.Close()
		traceFile.Close()
		return nil, err
	}

	return func() {
		trace.Stop()
		traceFile.Close()
		pprof.StopCPUProfile()
		cpuFile.Close()
		runtime.GC()
		for name, file := range map[string]string{
			"heap": "heap.pprof", "allocs": "allocs.pprof", "block": "block.pprof", "mutex": "mutex.pprof",
		} {
			if err := writeProfile(name, filepath.Join(dir, file), 0); err != nil {
				fmt.Fprintln(os.Stderr, "profile:", err)
			}
		}
		if err := writeProfile("goroutine", filepath.Join(dir, "goroutine.txt"), 2); err != nil {
			fmt.Fprintln(os.Stderr, "profile:", err)
		}
	}, nil
}

func writeProfile(name, path string, debug int) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return pprof.Lookup(name).WriteTo(f, debug)
}
