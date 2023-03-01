//go:build ignore

package main

import (
	"fmt"
	"runtime"
	"sync"
)

func main() {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		printStack()
	}()
	wg.Wait()
}

func printStack() {
	buf := make([]byte, 1<<20)
	n := runtime.Stack(buf, true)
	fmt.Printf("%s\n", buf[:n])
}
