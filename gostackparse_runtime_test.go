// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026 Datadog, Inc.

package gostackparse

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"reflect"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestParse_LiveRuntime(t *testing.T) {
	t.Run("runtime.Stack", func(t *testing.T) {
		stop := make(chan struct{})
		var wg sync.WaitGroup
		for _, fn := range []func(<-chan struct{}){liveRuntimeChan, liveRuntimeSelect, liveRuntimeSleep, liveRuntimeLocked} {
			wg.Add(1)
			go func(fn func(<-chan struct{})) {
				defer wg.Done()
				fn(stop)
			}(fn)
		}
		defer wg.Wait()
		defer close(stop)

		buf := make([]byte, 64<<10)
		deadline := time.Now().Add(5 * time.Second)
		for {
			n := runtime.Stack(buf, true)
			if n == len(buf) {
				buf = make([]byte, len(buf)*2)
				continue
			}
			gs, errs := Parse(bytes.NewReader(buf[:n]))
			if len(errs) != 0 {
				t.Fatalf("parse runtime.Stack: %v", errs)
			}
			found := 0
			for _, g := range gs {
				for _, frame := range g.Stack {
					switch {
					case strings.HasSuffix(frame.Func, ".liveRuntimeChan") && g.State == "chan receive":
						found++
					case strings.HasSuffix(frame.Func, ".liveRuntimeSelect") && g.State == "select":
						found++
					case strings.HasSuffix(frame.Func, ".liveRuntimeSleep") && g.State == "sleep":
						found++
					case strings.HasSuffix(frame.Func, ".liveRuntimeLocked") && g.State == "chan receive" && g.LockedToThread:
						found++
					}
				}
			}
			if found == 4 && len(gs) >= 5 {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("found %d of 4 blocked goroutines in %d parsed goroutines:\n%s", found, len(gs), buf[:n])
			}
			time.Sleep(time.Millisecond)
		}
	})
}

func TestParse_LiveRuntimePanic(t *testing.T) {
	for _, ancestors := range []bool{false, true} {
		name := "panic"
		if ancestors {
			name += "-ancestors"
		}
		t.Run(name, func(t *testing.T) {
			data := liveRuntimeTraceback(t, "panic", "all", ancestors)
			gs, errs := Parse(bytes.NewReader(data))
			if len(errs) != 0 {
				t.Fatalf("parse panic: %v\n%s", errs, data)
			}
			if len(gs) < 2 {
				t.Fatalf("parsed %d goroutines, want at least 2", len(gs))
			}
			g := gs[0]
			if g.State != "running" || !hasRuntimeFrame(g, ".liveRuntimePanic") {
				t.Fatalf("missing crashing goroutine: %+v", g)
			}
			if ancestors && g.Ancestor == nil {
				t.Fatal("missing ancestor")
			}
		})
	}
}

func liveRuntimeChan(stop <-chan struct{}) {
	<-stop
}

func liveRuntimeSelect(stop <-chan struct{}) {
	select {
	case <-stop:
	case <-make(chan struct{}):
	}
}

func liveRuntimeSleep(stop <-chan struct{}) {
	for {
		select {
		case <-stop:
			return
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func liveRuntimeLocked(stop <-chan struct{}) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	<-stop
}

func liveRuntimePanic() {
	panic("gostackparse live runtime test")
}

func TestHelperProcess(t *testing.T) {
	switch os.Getenv("GOSTACKPARSE_HELPER") {
	case "panic":
		go liveRuntimePanic()
	case "labels":
		go func() {
			ctx := pprof.WithLabels(context.Background(), pprof.Labels("handler", "checkout", "tenant", `acme corp }: ], "quoted"`))
			pprof.SetGoroutineLabels(ctx)
			liveRuntimePanic()
		}()
	case "overflow":
		debug.SetMaxStack(64 << 10)
		go liveRuntimeOverflow(1)
	default:
		return
	}
	select {}
}

func liveRuntimeOverflow(n int) int {
	return liveRuntimeOverflow(n+1) + n
}

func TestParse_LiveSystemTracebacks(t *testing.T) {
	for _, tt := range []struct {
		mode      string
		traceback string
	}{
		{"overflow", "all"},
		{"panic", "system"},
	} {
		t.Run(tt.mode+"-"+tt.traceback, func(t *testing.T) {
			data := liveRuntimeTraceback(t, tt.mode, tt.traceback, false)
			gs, errs := Parse(bytes.NewReader(data))
			if len(errs) != 0 {
				t.Fatalf("parse %s traceback: %v", tt.traceback, errs)
			}
			if len(gs) < 2 {
				t.Fatalf("parsed %d goroutines, want at least 2", len(gs))
			}
			if tt.mode == "overflow" {
				if !gs[0].FramesElided || !hasRuntimeFrame(gs[0], ".liveRuntimeOverflow") {
					t.Fatal("missing elided overflow frames")
				}
			} else if !hasRuntimeFrame(gs[0], ".liveRuntimePanic") {
				t.Fatal("missing crashing goroutine")
			} else if !hasRuntimeFrame(gs[0], "runtime.goexit") {
				// Only GOTRACEBACK=system shows runtime frames such as goexit.
				t.Fatal("missing runtime frames of a system traceback")
			}
		})
	}
}

func liveRuntimeTraceback(t *testing.T, mode, traceback string, ancestors bool) []byte {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("locate helper executable: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "-test.run=^TestHelperProcess$")
	for _, env := range os.Environ() {
		if !strings.HasPrefix(env, "GOSTACKPARSE_HELPER=") && !strings.HasPrefix(env, "GOTRACEBACK=") && !strings.HasPrefix(env, "GODEBUG=") {
			cmd.Env = append(cmd.Env, env)
		}
	}
	cmd.Env = append(cmd.Env, "GOSTACKPARSE_HELPER="+mode, "GOTRACEBACK="+traceback)
	if ancestors {
		cmd.Env = append(cmd.Env, "GODEBUG=tracebackancestors=3")
	} else if mode == "labels" {
		cmd.Env = append(cmd.Env, "GODEBUG=tracebacklabels=1")
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	if err := cmd.Wait(); err == nil {
		t.Fatal("helper did not crash")
	}
	if ctx.Err() != nil {
		t.Fatal(ctx.Err())
	}
	_, data, ok := bytes.Cut(stderr.Bytes(), []byte("\ngoroutine "))
	if !ok {
		t.Fatalf("missing traceback:\n%s", stderr.Bytes())
	}
	return append([]byte("goroutine "), data...)
}

func TestParseWithOptions_LiveTruncatedCrash(t *testing.T) {
	data := liveRuntimeTraceback(t, "panic", "all", false)
	complete, errs := Parse(bytes.NewReader(data))
	if len(errs) != 0 || len(complete) == 0 || !hasRuntimeFrame(complete[0], ".liveRuntimePanic") {
		t.Fatalf("parse complete crash: %v", errs)
	}
	offset := 0
	for _, line := range bytes.SplitAfter(data, []byte("\n")) {
		offset += len(line)
		if bytes.Contains(line, []byte(".liveRuntimePanic(")) {
			data = data[:offset+2]
			break
		}
	}
	defaults, errs := Parse(bytes.NewReader(data))
	if len(defaults) != 0 || len(errs) != 1 {
		t.Fatalf("default parse of truncated crash: %d goroutines, %v", len(defaults), errs)
	}
	kept, errs := ParseWithOptions(bytes.NewReader(data), ParseOptions{KeepIncomplete: true})
	if len(kept) != 1 || len(errs) != 1 {
		t.Fatalf("keep incomplete crash: %d goroutines, %v", len(kept), errs)
	}
	want := complete[0]
	want.Incomplete = true
	want.Stack = append([]*Frame(nil), want.Stack[:len(want.Stack)-1]...)
	want.CreatedBy = nil
	if !reflect.DeepEqual(want, kept[0]) {
		t.Fatalf("truncated crash differs: got %+v, want %+v", kept[0], want)
	}
}

func hasRuntimeFrame(g *Goroutine, suffix string) bool {
	for _, frame := range g.Stack {
		if strings.HasSuffix(frame.Func, suffix) {
			return true
		}
	}
	return false
}
