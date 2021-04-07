# gostackparse

Package stackparse parses goroutines stack trace dumps as produced by `panic()` or `runtime.Stack()`.

## Design Goals

1. Safe: No panics should be thrown.
2. Simple: Keep this pkg small and easy to modify.
3. Forgiving: Favor producing partial results over no results, even if the input data is different than expected.
4. Efficient: Parse several hundred MB/s.

## Testing

gostackparse has been tested using a combination of hand picked [test-fixtures](./test-fixtures), [property based testing](https://github.com/DataDog/gostackparse/search?q=TestParse_PropertyBased), and [fuzzing](https://github.com/DataDog/gostackparse/search?q=Fuzz).

## Benchmarks

gostackparse includes a small benchmark that shows that it can parse [test-fixtures/waitsince.txt](./test-fixtures/waitsince.txt) at ~300 MB/s.

```
goos: darwin
goarch: amd64
pkg: github.com/DataDog/gostackparse
cpu: Intel(R) Core(TM) i7-9750H CPU @ 2.60GHz
BenchmarkParse-12    	  44792	    26681 ns/op	297.74 MB/s	  17214 B/op	    306 allocs/op
PASS
ok  	github.com/DataDog/gostackparse	2.217s
```

## License

This work is dual-licensed under Apache 2.0 or BSD3. See [LICENSE](./LICENSE).
