# mcp Justfile
# Run `just --list` to see all available recipes.

mod := "github.com/the-protobuf-project/mcp"
bin := "protoc-gen-mcp"

# Default: list all recipes
default:
    @just --list

# Build the protoc-gen-mcp plugin
build:
    go build -o ./bin/{{bin}} ./plugin/cmd/protoc-gen-mcp/

# Build with version info baked in (for releases)
build-release version="dev":
    go build -trimpath -ldflags "-s -w -X main.version={{version}}" -o ./bin/{{bin}} ./plugin/cmd/protoc-gen-mcp/

# Cross-compile for a specific OS/ARCH
build-cross os arch version="dev":
    GOOS={{os}} GOARCH={{arch}} go build -trimpath -ldflags "-s -w -X main.version={{version}}" \
        -o ./bin/{{bin}}-{{os}}-{{arch}}{{if os == "windows" { ".exe" } else { "" } }} \
        ./plugin/cmd/protoc-gen-mcp/

# Build binaries for all release platforms
build-all version="dev":
    just build-cross linux   amd64 {{version}}
    just build-cross linux   arm64 {{version}}
    just build-cross darwin  amd64 {{version}}
    just build-cross darwin  arm64 {{version}}
    just build-cross windows amd64 {{version}}
    just build-cross windows arm64 {{version}}

# Install the plugin to $GOPATH/bin
install:
    go install ./plugin/cmd/protoc-gen-mcp/

# Run golangci-lint
lint:
    golangci-lint run ./...

# Run go vet
vet:
    go vet ./plugin/...

# Check formatting
fmt-check:
    @test -z "$(gofmt -l .)" || (echo "Files need formatting:" && gofmt -l . && exit 1)

# Format all Go files
fmt:
    gofmt -w .

# Lint proto files
buf-lint:
    cd protobuf && buf lint

# Regenerate the Go types the plugin reads MCP options through. Run this after
# editing anything under protobuf/mcp/v1, or the plugin keeps compiling against
# the previous shape of the schema.
proto-gen-go:
    cd protobuf && buf generate
    gofmt -w protobuf/mcppb

# Run all Go tests
test:
    go test ./...

# Run tests with verbose output
test-verbose:
    go test -v ./...

# Run tests with race detector
test-race:
    go test -race ./...

# Run tests with coverage
test-cover:
    go test -coverprofile=coverage.out ./...
    go tool cover -func=coverage.out

# Run Rust check
test-rust:
    cd examples/rust && cargo check --all-targets

# Run C++ example build (Make)
test-cpp:
    cd examples/cpp && make

# Run all tests (Go + Rust + C++)
test-all: test test-rust test-cpp

# Generate C++ proto stubs with local protoc (matches system libprotobuf)
generate-cpp:
    cd examples && buf export proto -o /tmp/proto_export
    mkdir -p examples/proto/generated/cpp
    rm -rf examples/proto/generated/cpp/todo examples/proto/generated/cpp/google examples/proto/generated/cpp/mcp
    protoc -I /tmp/proto_export \
        --cpp_out=examples/proto/generated/cpp \
        --grpc_out=examples/proto/generated/cpp \
        --plugin=protoc-gen-grpc=`which grpc_cpp_plugin` \
        /tmp/proto_export/mcp/protobuf/*.proto /tmp/proto_export/google/api/*.proto /tmp/proto_export/todo/v1/*.proto /tmp/proto_export/counter/v1/*.proto

# Build C++ example with Bazel
build-cpp-bazel:
    cd examples/cpp && bazel build //...

# Build C++ example with Make
build-cpp:
    cd examples/cpp && make

# Rebuild the plugin and regenerate all example proto code.
#
# buf resolves `local: protoc-gen-mcp` off PATH, where a released build — the
# Homebrew tap installs one — shadows the one just built and silently generates
# against the old templates. Every recipe that generates therefore puts ./bin
# first, so what runs is always what this working tree compiles.
generate: proto-gen-go build
    cd examples && PATH="{{justfile_directory()}}/bin:$PATH" buf generate
    # generated Go lives in the examples module, so it is formatted from there
    cd examples && go fmt ./proto/generated/go/...
    go fmt ./plugin/...
    just generate-cpp
    just generate-mime

# Regenerate the examples/mime gallery for Go and Rust.
#
# Separate from `generate` because the gallery resolves the MCP annotations from
# protobuf/ rather than the published module, so it cannot share a buf image
# with the other examples: both schemas claim extension numbers 51000-51006.
generate-mime: build
    PATH="{{justfile_directory()}}/bin:$PATH" buf generate

# Validate the gallery in both languages.
check-mime: generate-mime
    cd examples/mime && go test ./...
    cd examples/mime/rust && cargo test

# Run all checks (fmt, vet, lint, test, build)
check: fmt-check vet lint test build
# Quick check (vet + test + build, no lint)
check-quick: vet test build

# Remove build artifacts
clean:
    rm -rf ./bin ./coverage.out
    rm -rf ./dist


# Push proto module to buf.build/the-protobuf-project/mcp
buf-push:
    cd protobuf && buf push

# Push proto module with a specific label (e.g. a release tag)
buf-push-label label:
    cd protobuf && buf push --label {{label}}

# Dry-run publish Rust proto library to crates.io
publish-crates-dry:
    cd protobuf/rust && cargo publish --dry-run

# Publish Rust proto library to crates.io
publish-crates:
    cd protobuf/rust && cargo publish

# Create release archives for all platforms and push protos to BSR
release version: clean (build-all version)
    mkdir -p dist
    cd bin && for f in {{bin}}-*; do \
        if echo "$f" | grep -q windows; then \
            zip ../dist/"$f".zip "$f"; \
        else \
            tar czf ../dist/"$f".tar.gz "$f"; \
        fi \
    done
    @echo "Release archives in ./dist/"
    @ls -lh dist/
    @echo ""
    @echo "Pushing proto module to BSR with label {{version}} ..."
    cd protobuf && buf push --label {{version}}
    @echo "Done. Proto published as buf.build/the-protobuf-project/mcp:{{version}}"
