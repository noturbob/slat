.PHONY: build install clean run

BINARY := slat
BUILD_DIR := bin

build:
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY) ./cmd/slat

install: build
	@cp $(BUILD_DIR)/$(BINARY) $(GOPATH)/bin/$(BINARY) 2>/dev/null || 	cp $(BUILD_DIR)/$(BINARY) ~/go/bin/$(BINARY) 2>/dev/null || 	sudo cp $(BUILD_DIR)/$(BINARY) /usr/local/bin/$(BINARY)
	@echo "Installed $(BINARY)"

run: build
	./$(BUILD_DIR)/$(BINARY)

clean:
	rm -rf $(BUILD_DIR)

test:
	go test ./...
