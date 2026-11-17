// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026 Datadog, Inc.

//go:build go1.26

package gostackparse

import (
	"bytes"
	"testing"
)

func TestParse_LiveRuntimeLabels(t *testing.T) {
	data := liveRuntimeTraceback(t, "labels", "all", false)
	header, _, _ := bytes.Cut(data, []byte("\n"))
	if !bytes.Contains(header, []byte(`"handler": "checkout"`)) && !bytes.Contains(header, []byte(`handler: checkout`)) {
		t.Fatalf("missing handler label in goroutine header: %q", header)
	}
	gs, errs := Parse(bytes.NewReader(data))
	if len(errs) != 0 {
		t.Fatalf("parse labeled traceback: %v", errs)
	}
	if len(gs) == 0 || gs[0].State != "running" || !hasRuntimeFrame(gs[0], ".liveRuntimePanic") {
		t.Fatalf("missing labeled goroutine or its frames: %+v", gs)
	}
}
