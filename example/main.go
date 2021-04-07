// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021 Datadog, Inc.

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"runtime/debug"

	"github.com/DataDog/gostackparse"
)

func main() {
	stack := debug.Stack()
	goroutines, _ := gostackparse.Parse(bytes.NewReader(stack))
	json.NewEncoder(os.Stdout).Encode(goroutines)
}
