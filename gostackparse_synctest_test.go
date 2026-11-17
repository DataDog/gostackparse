// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026 Datadog, Inc.

//go:build go1.25

package gostackparse

import (
	"bytes"
	"os"
	"testing"
	"testing/synctest"
)

func TestParse_LiveSynctest(t *testing.T) {
	data := liveRuntimeTraceback(t, "synctest", "all", false)
	if !bytes.Contains(data, []byte(", synctest bubble ")) {
		t.Fatalf("missing synctest bubble header:\n%s", data)
	}
	gs, errs := Parse(bytes.NewReader(data))
	if len(errs) != 0 {
		t.Fatalf("parse synctest traceback: %v", errs)
	}
	var foundPanic, foundBlocked bool
	for _, g := range gs {
		// Go 1.25 runtime/traceback.go adds (durable) without checking _Gwaiting.
		if (g.State == "running" || g.State == "running (durable)") && hasRuntimeFrame(g, ".liveRuntimePanic") {
			foundPanic = true
		}
		if g.State == "chan receive (durable)" && hasRuntimeFrame(g, ".liveRuntimeChan") {
			foundBlocked = true
		}
	}
	if !foundPanic || !foundBlocked {
		t.Fatalf("missing bubble goroutines: panic=%t blocked=%t\n%s", foundPanic, foundBlocked, data)
	}
}

func TestHelperProcessSynctest(t *testing.T) {
	if os.Getenv("GOSTACKPARSE_HELPER") != "synctest" {
		return
	}
	synctest.Test(t, func(t *testing.T) {
		go liveRuntimeChan(make(chan struct{}))
		synctest.Wait()
		liveRuntimePanic()
	})
}
