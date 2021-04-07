// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021 Datadog, Inc.

package main

import (
	"encoding/json"
	"fmt"
	"os"

	stackparse "github.com/DataDog/gostackparse"
)

// usage: go run stack2json.go < example.txt
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	gs, errs := stackparse.Parse(os.Stdin)
	out, err := json.MarshalIndent(gs, "", "  ")
	if err != nil {
		return err
	}
	fmt.Printf("%s\n", out)

	if len(errs) > 0 {
		for i, e := range errs {
			fmt.Printf("error %d: %s\n", i+1, e)
		}
		return fmt.Errorf("%d errors occured", len(errs))
	}
	return nil
}
