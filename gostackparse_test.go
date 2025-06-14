// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021 Datadog, Inc.

package gostackparse

import (
	"bytes"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var update = flag.Bool("update", false, "update golden files")

// TestParse_GoldenFiles verifies the output of the parser for a set of input
// files in the test-fixtures. If you want to test a new behavior or bug fix,
// this is the best place to do it.
func TestParse_GoldenFiles(t *testing.T) {
	inputs, err := filepath.Glob(filepath.Join("test-fixtures", "*.txt"))
	require.NoError(t, err)
	for _, input := range inputs {
		inputData, err := ioutil.ReadFile(input)
		require.NoError(t, err)

		golden := strings.TrimSuffix(input, filepath.Ext(input)) + ".golden.json"
		goroutines, errs := Parse(bytes.NewReader(inputData))
		var errS []string
		for _, err := range errs {
			errS = append(errS, err.Error())
		}
		actual, err := json.MarshalIndent(struct {
			Errors     []string
			Goroutines []*Goroutine
		}{errS, goroutines}, "", "  ")
		actual = append(actual, '\n')
		require.NoError(t, err)

		if *update {
			ioutil.WriteFile(golden, actual, 0644)
		}
		expected, _ := ioutil.ReadFile(golden)
		require.JSONEq(t, string(expected), string(actual))
	}
}

// TestParse_PropertyBased does an exhaustive property based test against all
// possible permutations of a few interesting line fragements defined below,
// making sure the parser always does the right thing and never panics. This
// test is complex and shouldn't be extended unless there is a good reason for
// doing so. It's essentially just a very agressive smoke test.
func TestParse_PropertyBased(t *testing.T) {
	seen := map[string]bool{}
	tests := fixtures.Permutations()
	for i := 0; i < tests; i++ {
		dump := fixtures.Generate(i)
		dumpS := dump.String()
		msg := fmt.Sprintf("permutation %d:\n%s", i, dumpS)

		require.False(t, seen[dumpS], msg)
		seen[dumpS] = true

		goroutines, errs := Parse(strings.NewReader(dumpS))

		wantErr := dump.header.WantErr
		for _, f := range dump.stack {
			if wantErr != "" {
				break
			} else if f.fn.WantErr != "" {
				wantErr = f.fn.WantErr
			} else if f.file.WantErr != "" {
				wantErr = f.file.WantErr
			}
		}
		if wantErr == "" {
			wantErr = dump.createdBy.fn.WantErr
		}
		if wantErr == "" && dump.createdBy.fn.WantFrame != nil {
			wantErr = dump.createdBy.file.WantErr
		}

		if wantErr != "" {
			require.NotNil(t, errs, msg)
			require.Contains(t, errs[0].Error(), wantErr, msg)
			require.Equal(t, 0, len(goroutines), msg)
			continue
		}

		require.Nil(t, errs, msg)

		require.Equal(t, 1, len(goroutines), msg)
		g := goroutines[0]
		require.Equal(t, dump.header.WantG.ID, g.ID, msg)
		require.Equal(t, dump.header.WantG.State, g.State, msg)
		require.Equal(t, dump.header.WantG.Wait, g.Wait, msg)
		require.Equal(t, dump.header.WantG.LockedToThread, g.LockedToThread, msg)

		require.Equal(t, len(dump.stack), len(g.Stack), msg)
		for i, dumpFrame := range dump.stack {
			gFrame := g.Stack[i]
			require.Equal(t, dumpFrame.fn.WantFrame.Func, gFrame.Func, msg)
			require.Equal(t, dumpFrame.file.WantFrame.File, gFrame.File, msg)
			require.Equal(t, dumpFrame.file.WantFrame.Line, gFrame.Line, msg)
		}

		if dump.createdBy.fn.WantFrame == nil {
			require.Nil(t, g.CreatedBy, msg)
		} else {
			require.Equal(t, dump.createdBy.fn.WantFrame.Func, g.CreatedBy.Func, msg)
			require.Equal(t, dump.createdBy.file.WantFrame.File, g.CreatedBy.File, msg)
			require.Equal(t, dump.createdBy.file.WantFrame.Line, g.CreatedBy.Line, msg)
		}
	}
	t.Logf("executed %d tests", tests)
}

type dump struct {
	header    headerLine
	stack     []frameLines
	createdBy frameLines
}

func (d *dump) String() string {
	s := d.header.String()
	for _, f := range d.stack {
		s += f.String()
	}
	s += d.createdBy.String()
	return s
}

type headerLine struct {
	Line    string
	WantG   *Goroutine
	WantErr string
}

func (h *headerLine) String() string {
	return h.Line + "\n"
}

type frameLines struct {
	fn   frameLine
	file frameLine
}

func (f *frameLines) String() string {
	return f.fn.Line + "\n" + f.file.Line + "\n"
}

type frameLine struct {
	Line      string
	WantFrame *Frame
	WantErr   string
}

// generator generates all possible goroutine stack trace permutations based on
// the given stack depths and line fragements.
type generator struct {
	minStack  int
	maxStack  int
	headers   []headerLine
	funcs     []frameLine
	files     []frameLine
	createdBy []frameLine
}

func (g *generator) Generate(n int) *dump {
	// keep going around in circles
	n = n % g.Permutations()

	header := n % len(g.headers)
	n = n / len(g.headers)

	cFn := n % len(g.createdBy)
	n = n / len(g.createdBy)
	cFile := n % len(g.files)
	n = n / len(g.files)

	var stack []frameLines
	for d := 0; d < g.maxStack && (n > 0 || d < g.minStack); d++ {
		fn := n % len(g.funcs)
		n = n / len(g.funcs)
		file := n % len(g.files)
		n = n / len(g.files)
		frame := frameLines{
			fn:   g.funcs[fn],
			file: g.files[file],
		}
		stack = append(stack, frame)
	}

	d := &dump{
		header: g.headers[header],
		stack:  stack,
		createdBy: frameLines{
			fn:   g.createdBy[cFn],
			file: g.files[cFile],
		},
	}
	return d
}

func (g *generator) Permutations() int {
	p := 0
	for d := g.minStack; d <= g.maxStack; d++ {
		pp := 1
		for frame := 0; frame < d; frame++ {
			pp = pp * len(g.files) * len(g.funcs)
		}
		p += len(g.headers) * pp * len(g.createdBy) * len(g.files)
	}
	return p
}

var fixtures = generator{
	// Testing larger stack depths greatly increases the number of permutations
	// but is unlikely to shake out more bugs, so a depth of 1 to 2 seems like
	// the sweet spot.
	minStack: 1,
	maxStack: 2,

	headers: []headerLine{
		{
			Line:  "goroutine 1 [chan receive]:",
			WantG: &Goroutine{ID: 1, State: "chan receive", Wait: 0},
		},
		{
			Line:  "goroutine 2 [IO Wait, locked to thread]:",
			WantG: &Goroutine{ID: 2, State: "IO Wait", Wait: 0, LockedToThread: true},
		},
		{
			Line:  "goroutine 23 [select, 5 minutes]:",
			WantG: &Goroutine{ID: 23, State: "select", Wait: 5 * time.Minute},
		},
		{
			Line:  "goroutine 42 [select, 5 minutes, locked to thread]:",
			WantG: &Goroutine{ID: 42, State: "select", Wait: 5 * time.Minute, LockedToThread: true},
		},
		{
			Line:    "goroutine 23 []:",
			WantErr: "invalid goroutine header",
		},
		{
			Line:    "goroutine ",
			WantErr: "invalid goroutine header",
		},
		{
			Line:    "goroutine 1 [chan receive]:\n",
			WantErr: "invalid function call",
		},
	},

	funcs: []frameLine{
		{
			Line:      "main.main()",
			WantFrame: &Frame{Func: "main.main"},
		},
		{
			Line:      "runtime.goparkunlock(...)",
			WantFrame: &Frame{Func: "runtime.goparkunlock"},
		},
		{
			Line:      "net/http.(*persistConn).writeLoop(0xc0001a5c20)",
			WantFrame: &Frame{Func: "net/http.(*persistConn).writeLoop"},
		},
		{
			Line:    "foo.bar",
			WantErr: "invalid function call",
		},
		{
			Line:    "foo.bar(",
			WantErr: "invalid function call",
		},
		{
			Line:    "net/http.(*persistConn).writeLoop(0xc0",
			WantErr: "invalid function call",
		},
		{
			Line:    "net/http.(*persist",
			WantErr: "invalid function call",
		},
		{
			Line:    "net/http.*persist)(",
			WantErr: "invalid function call",
		},
		{
			Line:    "net/http.(*persist))",
			WantErr: "invalid function call",
		},
		{
			Line:    "net/http.((*persist)",
			WantErr: "invalid function call",
		},
		{
			Line:    "()",
			WantErr: "invalid function call",
		},
	},

	files: []frameLine{
		{
			Line:      "\t/go/src/example.org/example/main.go:231 +0x1187",
			WantFrame: &Frame{File: "/go/src/example.org/example/main.go", Line: 231},
		},
		{
			Line:      "\t/root/go1.15.6.linux.amd64/src/runtime/proc.go:312",
			WantFrame: &Frame{File: "/root/go1.15.6.linux.amd64/src/runtime/proc.go", Line: 312},
		},
		{
			Line:    "/root/go1.15.6.linux.amd64/src/runtime/proc.go:312",
			WantErr: "invalid file:line ref",
		},
		{
			Line:    "",
			WantErr: "invalid file:line ref",
		},
	},

	createdBy: []frameLine{
		{
			Line:      "created by net/http.(*Server).Serve",
			WantFrame: &Frame{Func: "net/http.(*Server).Serve"},
		},
		{
			Line:      "created by github.com/example.org/example/k8s.io/klog.init.0",
			WantFrame: &Frame{Func: "github.com/example.org/example/k8s.io/klog.init.0"},
		},
	},
}

func BenchmarkGostackparse(b *testing.B) {
	data, err := ioutil.ReadFile(filepath.Join("test-fixtures", "waitsince.txt"))
	require.NoError(b, err)

	b.ResetTimer()
	b.ReportAllocs()

	start := time.Now()
	parsedBytes := 0
	for i := 0; i < b.N; i++ {
		parsedBytes += len(data)
		gs, err := Parse(bytes.NewReader(data))
		if err != nil {
			b.Fatal(err)
		} else if l := len(gs); l != 9 {
			b.Fatal(l)
		}
	}

	mbPerSec := float64(parsedBytes) / time.Since(start).Seconds() / 1024 / 1024
	b.ReportMetric(mbPerSec, "MiB/s")
}

// Using go:embed to load test fixtures into memory, so the internal fuzzing infra doesn't need to handle individual files
// along with the fuzzer binary.
//
//go:embed test-fixtures/*.txt
var testFixtures embed.FS

func FuzzParse(f *testing.F) {
	files, err := testFixtures.ReadDir("test-fixtures")
	require.NoError(f, err)

	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".txt") {
			body, err := testFixtures.ReadFile(filepath.Join("test-fixtures", file.Name()))
			require.NoError(f, err)
			f.Add(body)
		}
	}
	// Regression tests
	// panic: runtime error: slice bounds out of range [28:26]
	f.Add([]byte("goroutine 0 [0]:\n0()\n\t:0\n[originating from goroutine "))

	f.Fuzz(func(t *testing.T, data []byte) {
		gs, err := Parse(bytes.NewReader(data))
		if err != nil {
			t.Skip()
		}
		// Invariant checks
		for _, g := range gs {
			for _, f := range g.Stack {
				if f.Func == "" {
					t.Errorf("func is empty: %+v", f)
				}
			}
		}
	})
}

// This is a regression test for a panic on a truncated line with an [originating from goroutine prefix.
func TestCrashRegression(t *testing.T) {
	crashPayload := []byte("goroutine 0 [0]:\n0()\n\t:0\n[originating from goroutine ")
	_, _ = Parse(bytes.NewReader(crashPayload))
}

func TestFuzzCorupus(t *testing.T) {
	if os.Getenv("FUZZ_CORPUS") == "" {
		t.Skip("set FUZZ_CORPUS=true to generate fuzz corpus")
	}
	dir := "corpus"
	tests := fixtures.Permutations()
	require.NoError(t, os.MkdirAll(dir, 0755))
	for i := 0; i < tests; i++ {
		dump := fixtures.Generate(i)
		name := filepath.Join(dir, fmt.Sprintf("%d.txt", i))
		require.NoError(t, ioutil.WriteFile(name, []byte(dump.String()), 0666))
	}
}

func Test_parseFile(t *testing.T) {
	tests := []struct {
		name       string
		line       string
		wantFrame  Frame
		wantReturn bool
	}{
		{
			name:       "empty",
			line:       "",
			wantFrame:  Frame{},
			wantReturn: false,
		},
		{
			name: "simple",
			line: "\t/root/go1.15.6.linux.amd64/src/net/http/server.go:2969 +0x36c",
			wantFrame: Frame{
				File: "/root/go1.15.6.linux.amd64/src/net/http/server.go",
				Line: 2969,
			},
			wantReturn: true,
		},
		{
			name: "simple+space",
			line: "\t/root/go1.15.6.linux.amd64/src/net/http/cool server.go:2969 +0x36c",
			wantFrame: Frame{
				File: "/root/go1.15.6.linux.amd64/src/net/http/cool server.go",
				Line: 2969,
			},
			wantReturn: true,
		},
		{
			name: "no-relative-pc",
			line: "\t/root/go1.15.6.linux.amd64/src/net/http/server.go:2969",
			wantFrame: Frame{
				File: "/root/go1.15.6.linux.amd64/src/net/http/server.go",
				Line: 2969,
			},
			wantReturn: true,
		},
		{
			name: "no-relative-pc+space",
			line: "\t/root/go1.15.6.linux.amd64/src/net/http/cool server.go:2969",
			wantFrame: Frame{
				File: "/root/go1.15.6.linux.amd64/src/net/http/cool server.go",
				Line: 2969,
			},
			wantReturn: true,
		},
	}
	for _, tt := range tests {
		if len(tt.line) > 1 {
			tt.name = "windows+" + tt.name
			tt.line = "\tC:" + tt.line[1:]
			tt.wantFrame.File = "C:" + tt.wantFrame.File
			tests = append(tests, tt)
		}
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var f Frame
			got := parseFile([]byte(tt.line), &f)
			if got != tt.wantReturn {
				t.Fatalf("got=%v want=%v", got, tt.wantReturn)
			} else if f != tt.wantFrame {
				t.Fatalf("got=%+v want=%+v", f, tt.wantFrame)
			}
		})
	}
}
