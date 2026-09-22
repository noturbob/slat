.PHONY: build install run test clean demo

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

clean:
	rm -rf bin

# Re-records docs/demo.gif from docs/demo.tape. Needs vhs, ttyd and ffmpeg.
#
# In its own XDG_RUNTIME_DIR, because the tape ends by quitting the session:
# with the usual one it would attach to whatever slat you have running and
# take your shells down with it.
demo: build
	@dir=$$(mktemp -d /tmp/slat-demo.XXXXXX) && \
	  PATH="$(CURDIR)/bin:$$PATH" XDG_RUNTIME_DIR=$$dir vhs docs/demo.tape; \
	  rc=$$?; rm -rf $$dir; exit $$rc
