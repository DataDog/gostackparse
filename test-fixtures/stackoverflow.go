//go:build ignore
// +build ignore

package main

import (
	"io"
	"os"
	"os/exec"
	"runtime/debug"
	"sync"
)

func main() {
	crashOutput := os.Getenv("CRASH_OUTPUT")
	if crashOutput != "" {
		f, err := os.OpenFile(crashOutput, os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			panic(err)
		}
		debug.SetCrashOutput(f, debug.CrashOptions{})
		f.Close()
		debug.SetMaxStack(2 << 13)
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			overflow()
		}()
		wg.Wait()
	} else {
		exe, err := os.Executable()
		if err != nil {
			panic(err)
		}
		f, err := os.CreateTemp("", "*.crash")
		if err != nil {
			panic(err)
		}
		defer f.Close()
		cmd := exec.Command(exe)
		cmd.Env = append(os.Environ(), "CRASH_OUTPUT="+f.Name())
		_, err = cmd.Output()
		if err == nil {
			panic("expected a crash")
		}
		_, err = io.Copy(os.Stdout, f)
		if err != nil {
			panic(err)
		}
	}
}

func overflow() {
	overflow()
}
