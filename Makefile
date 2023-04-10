# Makefile

SHELL := /bin/bash


BINARY_DIRS := $(shell find cmd -type d -mindepth 1)
BINARIES := $(patsubst cmd/%,build/%,$(BINARY_DIRS))
DEBUG_BINARIES := $(patsubst cmd/%,build/debug/%,$(BINARY_DIRS))
TAGS := sqlite_vtable,sqlite_introspect,sqlite_json
TIMESTAMP := $(shell date +%Y-%m-%d)

# Default target (build all)
.PHONY: all
all: $(BINARIES)

# Build individual binaries
.PHONY: $(BINARIES)
$(BINARIES): build/%: cmd/%
	@echo "Building $@"
	@mkdir -p $(@D)
	@go build --tags $(TAGS) -o $@ ./$< 

# Build debug binaries
.PHONY: debug
debug: $(DEBUG_BINARIES)

.PHONY: $(DEBUG_BINARIES)
$(DEBUG_BINARIES): build/debug/%: cmd/%
	@echo "Building debug version of $@"
	@mkdir -p $(@D)
	@go build --tags $(TAGS) -gcflags="all=-N -l" -o $@ ./$<

# Clean build directory
clean:
	@echo "Cleaning build directory"
	@rm -rf build

.PHONY: test-coverage
test-coverage:
	@echo "Running tests with coverage..."
	@go test --tags $(TAGS) -coverprofile=coverage.out ./...
	@echo "Coverage summary:"
	@go tool cover -func=coverage.out
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Detailed coverage report generated: coverage.html"