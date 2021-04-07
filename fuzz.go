// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

package gostackparse

import "bytes"

// Fuzz implements fuzzing using https://github.com/dvyukov/go-fuzz. See
// TestFuzzCorupus for generating an initial test corpus. This API is not part
// of the semver compatability of this module, do not reference it.
func Fuzz(data []byte) int {
	goroutines, _ := Parse(bytes.NewReader(data))
	if len(goroutines) > 0 {
		return 1
	}
	return 0
}
