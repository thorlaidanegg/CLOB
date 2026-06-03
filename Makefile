.PHONY: test test-race stress bench bench-long lint

# Full unit + property suite.
test:
	go test ./... -count=1

# Same, with the race detector (needs a 64-bit C toolchain; runs in CI).
test-race:
	go test ./... -race -count=1

# Concurrency stress + determinism + exact-money conservation, under -race.
# This is the suite that breaks a naive engine (data races, nondeterminism, float drift).
stress:
	go test ./engine/ -run 'Stress|Determinism|Conservation' -race -count=1

# End-to-end engine throughput (place+cancel, matching, multi-market scaling).
bench:
	go test ./engine/ -run '^$$' -bench BenchmarkEngine -benchmem

# Longer, steadier benchmark numbers.
bench-long:
	go test ./engine/ -run '^$$' -bench BenchmarkEngine -benchmem -benchtime=5s -count=3

lint:
	golangci-lint run ./...
