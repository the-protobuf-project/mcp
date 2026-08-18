# Build root for C++ MCP server. Binary and build artifacts live here.
# Generated code is in ../proto/generated/cpp (source only).
# Run: make (from examples/cpp/)

CXX ?= g++
CXXFLAGS ?= -std=c++17 -O2 -Wall -Wno-deprecated-declarations

CPP_DIR := $(CURDIR)
GEN_DIR := $(CPP_DIR)/../proto/generated/cpp
RUST_DIR := $(GEN_DIR)/rust
SRC_DIR := $(CPP_DIR)/src
CXX_INCLUDE := $(RUST_DIR)/target/cxx_include
# The crate is named after whichever service protoc-gen-mcp generated the
# bridge for, so the library name is read from the generated Cargo.toml
# rather than globbed. A glob picks the alphabetically first lib*.a, which
# after the bridge is regenerated for a different service is the stale one
# left behind in target/release -- and linking that produces a server that
# builds cleanly while serving the previous service's (or no) tools.
RUST_CRATE := $(shell sed -n 's/^name = "\(.*\)"/\1/p' $(RUST_DIR)/Cargo.toml | head -1)
RUST_LIB := $(RUST_DIR)/target/release/lib$(RUST_CRATE).a

GRPC_CFLAGS := $(shell pkg-config --cflags grpc++ protobuf 2>/dev/null || echo "")
GRPC_LIBS := $(shell pkg-config --libs grpc++ protobuf 2>/dev/null || echo "") -lgrpc++_reflection

PROTO_CC := $(shell find $(GEN_DIR) \( -name '*.pb.cc' -o -name '*.grpc.pb.cc' \) 2>/dev/null)
MCP_CC := $(shell find $(GEN_DIR) -name '*.mcp.cc' 2>/dev/null)
MAIN_GEN := $(GEN_DIR)/main.cc
HAS_USER_MAIN := $(wildcard $(SRC_DIR)/main.cc)
USER_CC := $(filter-out $(SRC_DIR)/main.cc,$(wildcard $(SRC_DIR)/*.cc))

GEN_SRCS := $(if $(HAS_USER_MAIN),,$(MAIN_GEN)) $(MCP_CC) $(PROTO_CC)
GEN_OBJS := $(patsubst $(GEN_DIR)/%.cc,$(CPP_DIR)/build/gen/%.o,$(GEN_SRCS))
SRC_OBJS := $(patsubst $(SRC_DIR)/%.cc,$(CPP_DIR)/build/src/%.o,$(USER_CC))
SRC_OBJS += $(if $(HAS_USER_MAIN),$(CPP_DIR)/build/src/main.o,)
OBJS := $(GEN_OBJS) $(SRC_OBJS)

OBJ_DIR := $(CPP_DIR)/build
BINARY := $(CPP_DIR)/server

.PHONY: all rust clean

all: rust $(BINARY)

rust:
	cd $(RUST_DIR) && CARGO_TARGET_DIR=$(RUST_DIR)/target cargo build --release

UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Darwin)
  MACOSX_DEPLOYMENT_TARGET ?= 26.4
  export MACOSX_DEPLOYMENT_TARGET
  CXXFLAGS += -mmacosx-version-min=$(MACOSX_DEPLOYMENT_TARGET)
  RUST_LDFLAGS := -framework CoreFoundation -framework Security -framework SystemConfiguration
else
  RUST_LDFLAGS :=
endif

$(BINARY): rust $(OBJS)
	$(CXX) $(CXXFLAGS) -o $@ $(OBJS) $(RUST_LIB) $(GRPC_LIBS) $(RUST_LDFLAGS) -lpthread -ldl

$(OBJ_DIR)/gen/%.o: $(GEN_DIR)/%.cc
	@mkdir -p $(dir $@)
	$(CXX) $(CXXFLAGS) -I$(GEN_DIR) -I$(SRC_DIR) -I$(CXX_INCLUDE) $(GRPC_CFLAGS) -c -o $@ $<

$(OBJ_DIR)/src/%.o: $(SRC_DIR)/%.cc
	@mkdir -p $(dir $@)
	$(CXX) $(CXXFLAGS) -I$(GEN_DIR) -I$(SRC_DIR) -I$(CXX_INCLUDE) $(GRPC_CFLAGS) -c -o $@ $<

clean:
	rm -rf $(OBJ_DIR) $(BINARY)
	cd $(RUST_DIR) && cargo clean
