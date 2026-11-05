.PHONY: all build service desktop test check clean run-service run-desktop

BUILD_DIR := build

all: build

build: service desktop

service:
	mkdir -p $(BUILD_DIR)/bin
	go build -trimpath -o $(BUILD_DIR)/bin/rippled ./cmd/rippled

desktop: service
	mkdir -p $(BUILD_DIR)/desktop
	cd $(BUILD_DIR)/desktop && qmake6 ../../desktop/ripple.pro
	$(MAKE) -C $(BUILD_DIR)/desktop
	cp $(BUILD_DIR)/desktop/ripple-desktop $(BUILD_DIR)/bin/ripple-desktop

test:
	go test ./...

check:
	gofmt -d $$(find cmd internal -name '*.go' -type f)
	go vet ./...
	go test ./...

run-service: service
	$(BUILD_DIR)/bin/rippled

run-desktop: desktop
	$(BUILD_DIR)/bin/ripple-desktop

clean:
	rm -rf $(BUILD_DIR)
