// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026 Datadog, Inc.

package gostackparse

import (
	"bufio"
	"bytes"
	"errors"
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

func TestParseWithOptions_MaxLineSize(t *testing.T) {
	const size = 70 << 10
	const header = "goroutine 1 [running]:\n"
	const labelStart = "goroutine 1 [running] {handler: "
	const labelEnd = "}:"
	const second = "\ngoroutine 2 [running]:\nmain.second()\n\t/main.go:2\n"
	function := "main." + strings.Repeat("f", size-len("main.()"))
	file := "/" + strings.Repeat("f", size-len("\t/:1"))
	labelHeader := labelStart + strings.Repeat("x", size-len(labelStart)-len(labelEnd)) + labelEnd + "\n"
	for _, tt := range []struct {
		name       string
		first      string
		frame      Frame
		defaultErr string
	}{
		{"function line", header + function + "()\n\t/main.go:1\n", Frame{Func: function, File: "/main.go", Line: 1}, "invalid function call"},
		{"file line", header + "main.first()\n\t" + file + ":1\n", Frame{Func: "main.first", File: file, Line: 1}, "invalid file:line ref"},
		{"label header", labelHeader + "main.first()\n\t/main.go:1\n", Frame{Func: "main.first", File: "/main.go", Line: 1}, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			input := tt.first + second
			gs, errs := ParseWithOptions(strings.NewReader(input), ParseOptions{})
			require.Empty(t, gs)
			if tt.defaultErr == "" {
				require.Len(t, errs, 1)
			} else {
				require.Len(t, errs, 2)
				require.ErrorContains(t, errs[0], tt.defaultErr)
			}
			require.ErrorIs(t, errs[len(errs)-1], bufio.ErrTooLong)

			gs, errs = ParseWithOptions(strings.NewReader(input), ParseOptions{MaxLineSize: 1 << 20})
			require.Empty(t, errs)
			require.Equal(t, []*Goroutine{
				{ID: 1, State: "running", Stack: []*Frame{&tt.frame}},
				{ID: 2, State: "running", Stack: []*Frame{{Func: "main.second", File: "/main.go", Line: 2}}},
			}, gs)
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

func TestParseWithOptions_MaxLineSizeBoundary(t *testing.T) {
	const header = "goroutine 1 [running]:\n"
	funcLine := func(n int, eol string) string { // n bytes including eol
		return "main.f()" + strings.Repeat("x", n-8-len(eol)) + eol
	}
	fileLine := func(n int) string { // n bytes, no line ending
		return "\t/" + strings.Repeat("a", n-7) + ".go:1"
	}
	for _, tt := range []struct {
		name  string
		input string
		limit int
		ok    bool
	}{
		{"LF fits", header + funcLine(64, "\n") + "\t/main.go:1\n", 64, true},
		{"LF one over", header + funcLine(65, "\n") + "\t/main.go:1\n", 64, false},
		{"CRLF fits", header + funcLine(64, "\r\n") + "\t/main.go:1\n", 64, true},
		{"CRLF one over", header + funcLine(65, "\r\n") + "\t/main.go:1\n", 64, false},
		{"last line without ending fits", header + "main.f()\n" + fileLine(63), 64, true},
		{"last line without ending at limit", header + "main.f()\n" + fileLine(64), 64, false},
		{"negative limit keeps the default", header + funcLine(70*1024, "\n") + "\t/main.go:1\n", -1, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			gs, errs := ParseWithOptions(strings.NewReader(tt.input), ParseOptions{MaxLineSize: tt.limit})
			if tt.ok {
				if len(errs) != 0 || len(gs) != 1 || len(gs[0].Stack) != 1 {
					t.Fatalf("got %d goroutines, errs=%v", len(gs), errs)
				}
				return
			}
			for _, err := range errs {
				if errors.Is(err, bufio.ErrTooLong) {
					return
				}
			}
			t.Fatalf("want bufio.ErrTooLong, got errs=%v", errs)
		})
	}
}
