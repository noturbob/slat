.PHONY: build install run test test-fast clean

build:
	go build -o bin/slat ./cmd/slat

# Installs to $(go env GOBIN), or $(go env GOPATH)/bin when that's unset.
install:
	go install ./cmd/slat

run: build
	./bin/slat

test:
	go vet ./...
	go test -race ./...

test-fast:
	go test -short ./...

clean:
	rm -rf bin

