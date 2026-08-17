.PHONY: test sim

test:
	go test ./...

sim:
	go run ./cmd/promtact-sim -seed 0x4A2C -steps 10000 -nodes 5
