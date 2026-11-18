// Copyright 2026 The Protobuf Project authors.
// SPDX-License-Identifier: Apache-2.0

package generator

import (
	"testing"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

// serviceFile describes one proto file for cppFiles: a package, a service, and
// whether its single RPC streams its response.
type serviceFile struct {
	path      string
	pkg       string
	service   string
	streaming bool
}

// cppFiles compiles specs into the *protogen.File values generateCpp works on,
// in the order given.
func cppFiles(t *testing.T, specs ...serviceFile) []*protogen.File {
	t.Helper()

	req := &pluginpb.CodeGeneratorRequest{}
	for _, spec := range specs {
		req.FileToGenerate = append(req.FileToGenerate, spec.path)
		req.ProtoFile = append(req.ProtoFile, &descriptorpb.FileDescriptorProto{
			Name:    proto.String(spec.path),
			Package: proto.String(spec.pkg),
			Syntax:  proto.String("proto3"),
			MessageType: []*descriptorpb.DescriptorProto{
				{Name: proto.String("Req")},
				{Name: proto.String("Resp")},
			},
			Service: []*descriptorpb.ServiceDescriptorProto{{
				Name: proto.String(spec.service),
				Method: []*descriptorpb.MethodDescriptorProto{{
					Name:            proto.String("Do"),
					InputType:       proto.String("." + spec.pkg + ".Req"),
					OutputType:      proto.String("." + spec.pkg + ".Resp"),
					ServerStreaming: proto.Bool(spec.streaming),
				}},
			}},
			Options: &descriptorpb.FileOptions{
				GoPackage: proto.String("example.com/gen/" + spec.pkg + ";genpb"),
			},
		})
	}

	plugin, err := protogen.Options{}.New(req)
	if err != nil {
		t.Fatalf("build plugin: %v", err)
	}
	return plugin.Files
}

// The C++ target cannot express a streaming RPC, so a file whose every RPC
// streams contributes no tools. It must not be the file the shared bridge and
// main.cc are generated from: that pairing compiles and links happily and
// yields an MCP server advertising an empty tool list, which is what the C++
// example shipped while counter/v1 sorted ahead of todo/v1.
func TestCppSharedFileIndexSkipsStreamingOnlyFiles(t *testing.T) {
	files := cppFiles(t,
		serviceFile{path: "counter/v1/counter_service.proto", pkg: "counter.v1", service: "CounterService", streaming: true},
		serviceFile{path: "todo/v1/todo_service.proto", pkg: "todo.v1", service: "TodoService"},
	)

	if got := cppSharedFileIndex(files); got != 1 {
		t.Errorf("cppSharedFileIndex = %d (%s), want 1 (todo/v1/todo_service.proto)",
			got, files[got].Desc.Path())
	}
}

// The common case is unchanged: the first file that yields tools wins, and when
// that is files[0] nothing moves.
func TestCppSharedFileIndexPrefersTheFirstEligibleFile(t *testing.T) {
	files := cppFiles(t,
		serviceFile{path: "a/v1/a_service.proto", pkg: "a.v1", service: "AService"},
		serviceFile{path: "b/v1/b_service.proto", pkg: "b.v1", service: "BService"},
	)

	if got := cppSharedFileIndex(files); got != 0 {
		t.Errorf("cppSharedFileIndex = %d, want 0", got)
	}
}

// With nothing eligible anywhere there is still a build to emit, so the shared
// outputs fall back to the first file rather than being dropped.
func TestCppSharedFileIndexFallsBackWhenNothingIsEligible(t *testing.T) {
	files := cppFiles(t,
		serviceFile{path: "counter/v1/counter_service.proto", pkg: "counter.v1", service: "CounterService", streaming: true},
		serviceFile{path: "ticker/v1/ticker_service.proto", pkg: "ticker.v1", service: "TickerService", streaming: true},
	)

	if got := cppSharedFileIndex(files); got != 0 {
		t.Errorf("cppSharedFileIndex = %d, want 0 (fallback)", got)
	}
}

// main.cc starts a single service. With several services in one file it must
// be the first one declared, on every run: ranging the services map picked a
// different one from run to run. Zed is declared ahead of Alpha so a fix that
// sorts by name instead fails too.
func TestCppFirstServiceFollowsDeclarationOrder(t *testing.T) {
	method := func() []*descriptorpb.MethodDescriptorProto {
		return []*descriptorpb.MethodDescriptorProto{{
			Name:       proto.String("Do"),
			InputType:  proto.String(".multi.v1.Req"),
			OutputType: proto.String(".multi.v1.Resp"),
		}}
	}
	var services []*descriptorpb.ServiceDescriptorProto
	for _, name := range []string{"ZedService", "MidService", "OtherService", "AlphaService"} {
		services = append(services, &descriptorpb.ServiceDescriptorProto{Name: proto.String(name), Method: method()})
	}
	plugin, err := protogen.Options{}.New(&pluginpb.CodeGeneratorRequest{
		FileToGenerate: []string{"multi/v1/multi.proto"},
		ProtoFile: []*descriptorpb.FileDescriptorProto{{
			Name:    proto.String("multi/v1/multi.proto"),
			Package: proto.String("multi.v1"),
			Syntax:  proto.String("proto3"),
			MessageType: []*descriptorpb.DescriptorProto{
				{Name: proto.String("Req")},
				{Name: proto.String("Resp")},
			},
			Service: services,
			Options: &descriptorpb.FileOptions{GoPackage: proto.String("example.com/gen/multi/v1;multipb")},
		}},
	})
	if err != nil {
		t.Fatalf("build plugin: %v", err)
	}

	g := NewCppFileGenerator(plugin.Files[0], plugin)
	for range 20 {
		p := g.buildCppParams("multi/v1", "multi")
		if p.FirstServiceName != "ZedService" || p.FirstServiceSnake != "zed_service" {
			t.Fatalf("first service = %q (%q), want ZedService (zed_service)", p.FirstServiceName, p.FirstServiceSnake)
		}
	}
}
