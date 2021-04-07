# gostackparse

Package stackparse parses goroutines stack trace dumps as produced by panic() or runtime.Stack().

## Design Goals

1. Safe: No panics should be thrown.
2. Simple: Keep this pkg small and easy to modify.
3. Forgiving: Favor producing partial results over no results, even if the input data is different than expected.
4. Efficient: Parse several hundred MB/s.

## License

This work is dual-licensed under Apache 2.0 or BSD3. See [LICENSE](./LICENSE).
