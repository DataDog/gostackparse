// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026 Datadog, Inc.

package gostackparse

import (
	"bufio"
	"bytes"
	"io/ioutil"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseWithOptions_KeepIncomplete(t *testing.T) {
	const header = "goroutine 1 [running]:\n"
	const frame = "main.first()\n\t/main.go:1\n"
	const ancestor = "[originating from goroutine 2]:\n"
	first := &Frame{Func: "main.first", File: "/main.go", Line: 1}
	for _, tt := range []struct {
		name       string
		input      string
		incomplete bool
		stack      []*Frame
		ancestor   *Goroutine
		elided     bool
	}{
		{name: "empty stack", input: header, incomplete: true},
		{name: "mid-function", input: header + frame + "main.next(", incomplete: true, stack: []*Frame{first}},
		{name: "missing file", input: header + frame + "main.next()\n", incomplete: true, stack: []*Frame{first}},
		{name: "mid-file", input: header + frame + "main.next()\n\t/main.go:", incomplete: true, stack: []*Frame{first}},
		{name: "malformed file", input: header + frame + "main.next()\n\t/main.go:2x\n", incomplete: true, stack: []*Frame{first}},
		{name: "mid-created-by function", input: header + frame + "created by main.spa", incomplete: true, stack: []*Frame{first}},
		{name: "mid-created-by file", input: header + frame + "created by main.spawn\n\t/main.go:", incomplete: true, stack: []*Frame{first}},
		{name: "invalid ancestor header", input: header + frame + "[originating from goroutine ", incomplete: true, stack: []*Frame{first}},
		{name: "mid-ancestor ID", input: header + frame + "[originating from goroutine 98", incomplete: true, stack: []*Frame{first}},
		{name: "missing ancestor stack", input: header + frame + ancestor, incomplete: true, stack: []*Frame{first}, ancestor: &Goroutine{ID: 2}},
		{name: "mid-ancestor function", input: header + frame + ancestor + "main.parent(", incomplete: true, stack: []*Frame{first}, ancestor: &Goroutine{ID: 2}},
		{name: "mid-ancestor file", input: header + frame + ancestor + frame + "main.parent()\n\t/main.go:", incomplete: true, stack: []*Frame{first}, ancestor: &Goroutine{ID: 2, Stack: []*Frame{first}}},
		{name: "mid-ancestor created-by", input: header + frame + ancestor + frame + "created by main.spawn\n", incomplete: true, stack: []*Frame{first}, ancestor: &Goroutine{ID: 2, Stack: []*Frame{first}}},
		{name: "EOF without newline", input: header + "main.first()\n\t/main.go:1", stack: []*Frame{first}},
		{name: "EOF after complete frame", input: header + frame, stack: []*Frame{first}},
		{name: "legacy elision then frame", input: header + "...additional frames elided...\n" + frame, stack: []*Frame{first}, elided: true},
		{name: "counted elision then frame", input: header + "...123 frames elided...\n" + frame, stack: []*Frame{first}, elided: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			gs, errs := ParseWithOptions(strings.NewReader(tt.input), ParseOptions{KeepIncomplete: true})
			require.Equal(t, []*Goroutine{{
				ID: 1, State: "running", Stack: tt.stack, Ancestor: tt.ancestor,
				Incomplete: tt.incomplete, FramesElided: tt.elided,
			}}, gs)
			if tt.incomplete {
				require.Len(t, errs, 1)
			} else {
				require.Empty(t, errs)
			}
		})
	}
}

func TestParseWithOptions_KeepIncompleteScannerError(t *testing.T) {
	const block = "goroutine 1 [running]:\nmain.first()\n\t/main.go:1\n[originating from goroutine 2]:\nmain.parent()\n\t/main.go:2\n"
	for _, tt := range []struct {
		name       string
		separator  string
		incomplete bool
	}{
		{name: "open goroutine", incomplete: true},
		{name: "closed goroutine", separator: "\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			input := block + tt.separator + strings.Repeat("x", bufio.MaxScanTokenSize) + "\n"
			gs, errs := Parse(strings.NewReader(input))
			require.Len(t, errs, 1)
			require.ErrorIs(t, errs[0], bufio.ErrTooLong)
			require.Len(t, gs, 1)
			require.False(t, gs[0].Incomplete)

			kept, keptErrs := ParseWithOptions(strings.NewReader(input), ParseOptions{KeepIncomplete: true})
			require.Equal(t, errs, keptErrs)
			checkParseSuperset(t, gs, kept, errs)
			require.Len(t, kept, 1)
			require.Equal(t, tt.incomplete, kept[0].Incomplete)
			require.NotNil(t, kept[0].Ancestor)
			require.False(t, kept[0].Ancestor.Incomplete)
		})
	}
}

func TestParseWithOptions_InvalidHeader(t *testing.T) {
	for _, input := range []string{"goroutine ", "goroutine nope [running]:\n", "goroutine 1 []:\n"} {
		gs, errs := ParseWithOptions(strings.NewReader(input), ParseOptions{KeepIncomplete: true})
		require.Empty(t, gs)
		require.Len(t, errs, 1)
	}
}

func TestParseWithOptions_Superset(t *testing.T) {
	check := func(data []byte) {
		gs, errs := Parse(bytes.NewReader(data))
		kept, keptErrs := ParseWithOptions(bytes.NewReader(data), ParseOptions{KeepIncomplete: true})
		require.Equal(t, errs, keptErrs)
		checkParseSuperset(t, gs, kept, errs)
	}
	inputs, err := filepath.Glob(filepath.Join("test-fixtures", "*.txt"))
	require.NoError(t, err)
	for _, input := range inputs {
		data, err := ioutil.ReadFile(input)
		require.NoError(t, err)
		check(data)
	}
	for i := 0; i < fixtures.Permutations(); i++ {
		check([]byte(fixtures.Generate(i).String()))
	}
	const mixed = "goroutine 1 [running]:\nmain.first()\n\t/main.go:1\n\ngoroutine 2 [running]:\nmain.broken(\n\ngoroutine bad\n\ngoroutine 3 [running]:\nmain.last()\n\t/main.go:3\n\n"
	for end := 0; end <= len(mixed); end++ {
		check([]byte(mixed[:end]))
	}
}

func checkParseSuperset(t testing.TB, defaults, kept []*Goroutine, errs []error) {
	t.Helper()
	i := 0
	for j, g := range kept {
		if g.Incomplete {
			// The default parser may keep the last goroutine without marking
			// it incomplete when the scanner stops after a complete frame.
			if j == len(kept)-1 && i < len(defaults) && len(errs) > 0 && errs[len(errs)-1] == bufio.ErrTooLong {
				unmarked := *g
				unmarked.Incomplete = false
				g = &unmarked
			} else {
				continue
			}
		}
		if i >= len(defaults) || !reflect.DeepEqual(defaults[i], g) {
			t.Fatalf("complete goroutine %d differs from default: %+v", i, g)
		}
		i++
	}
	if i != len(defaults) {
		t.Fatalf("kept %d of %d default goroutines", i, len(defaults))
	}
}
