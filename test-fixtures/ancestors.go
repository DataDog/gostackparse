//+build ignore

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

package main

import (
	"fmt"
	"runtime"
)

// adapted from https://github.com/golang/go/issues/22289
// run with GODEBUG=tracebackancestors=10 go run ancestors.go
func main() {
	w := make(chan struct{})
	go foo(w, 5)
	<-w
	x := make(chan struct{})
	go foo(x, 3)
	<-x
	fmt.Print(string(stackAll()))
	close(w)
}

func stackAll() []byte {
	buf := make([]byte, 1024)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			return buf[:n]
		}
		buf = make([]byte, 2*len(buf))
	}
}

const maxStackDepth = 5

func foo(w chan struct{}, timesToCallGo int) {
	if timesToCallGo == 0 {
		w <- struct{}{}
		<-w
		return
	}
	if timesToCallGo%2 == 0 {
		d1(func() { foo(w, timesToCallGo-1) }, maxStackDepth)
	} else {
		d1(func() { bar(w, timesToCallGo-1) }, maxStackDepth)
	}
}

func bar(w chan struct{}, timesToCallGo int) {
	if timesToCallGo == 0 {
		w <- struct{}{}
		<-w
		return
	}
	if timesToCallGo%2 == 0 {
		d1(func() { foo(w, timesToCallGo-1) }, maxStackDepth)
	} else {
		d1(func() { bar(w, timesToCallGo-1) }, maxStackDepth)
	}
}

func d1(fn func(), timesToRecurse int) {
	if timesToRecurse == 0 {
		go fn()
		return
	}
	d1(fn, timesToRecurse-1)
}
