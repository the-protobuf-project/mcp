package generator

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/the-protobuf-project/protokit/factory"
	"google.golang.org/protobuf/compiler/protogen"
)

// PluginVersion is set by the protoc-gen-mcp binary before generation.
var PluginVersion = "dev"

// LangAll requests every language the target supports.
const LangAll = "all"

// Model is the MCP model a source produces: the proto files selected for
// generation, in the order protoc listed them.
//
// It is the plugin-defined model type M that protokit's factory is generic
// over; protokit itself stays free of it.
type Model struct {
	Files []*protogen.File
}

// ProtoSource builds the Model from the CodeGeneratorRequest protoc hands the
// plugin.
type ProtoSource struct{}

// Name identifies the source in a [factory.Registry].
func (ProtoSource) Name() string { return "proto" }

// Build collects the files protoc marked for generation.
func (ProtoSource) Build(ctx factory.Ctx) (*Model, error) {
	if ctx.Plugin == nil {
		return nil, fmt.Errorf("proto source requires plugin (protoc) mode")
	}
	m := &Model{}
	for _, f := range ctx.Plugin.Files {
		if !f.Generate {
			continue
		}
		m.Files = append(m.Files, f)
	}
	return m, nil
}

// Target renders the MCP model into MCP server bindings for one language.
type Target struct {
	// PackageSuffix is Go-specific: sub-package suffix for generated files.
	PackageSuffix string
}

// Name identifies the target in a [factory.Registry].
func (Target) Name() string { return "mcp" }

// Languages lists the languages this target can emit.
func (Target) Languages() []string { return []string{LangGo, LangPython, LangRust, LangCpp} }

// Generate renders m for a single language.
func (t Target) Generate(ctx factory.Ctx, m *Model, lang string) error {
	if ctx.Plugin == nil {
		return fmt.Errorf("mcp target requires plugin (protoc) mode")
	}
	switch lang {
	case LangGo:
		for _, f := range m.Files {
			NewFileGenerator(f, ctx.Plugin).Generate(t.PackageSuffix)
		}
	case LangPython:
		for _, f := range m.Files {
			NewPythonFileGenerator(f, ctx.Plugin).Generate()
		}
	case LangRust:
		for _, f := range m.Files {
			NewRustFileGenerator(f, ctx.Plugin).Generate()
		}
	case LangCpp:
		t.generateCpp(ctx.Plugin, m)
	default:
		return fmt.Errorf("unsupported language %q (supported: %s)",
			lang, strings.Join(t.Languages(), ", "))
	}
	return nil
}

// generateCpp emits C++ for every file that declares a service, in path order.
// The shared files (rust/*, Makefile, main.cc) are emitted once, with the first
// file, so that a multi-file request does not write them repeatedly.
func (t Target) generateCpp(gen *protogen.Plugin, m *Model) {
	files := make([]*protogen.File, 0, len(m.Files))
	for _, f := range m.Files {
		if len(f.Services) > 0 {
			files = append(files, f)
		}
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].Desc.Path() < files[j].Desc.Path()
	})
	for i, f := range files {
		NewCppFileGenerator(f, gen).Generate(i == 0)
	}
}

// Registry returns the registry of sources and targets this plugin ships.
func Registry(packageSuffix string) *factory.Registry[*Model] {
	reg := factory.NewRegistry[*Model]()
	reg.AddSource(ProtoSource{})
	reg.AddTarget(Target{PackageSuffix: packageSuffix})
	return reg
}

// Generate builds the MCP model from gen and renders it for each requested
// language. A lang of [LangAll] expands to every language the target supports.
func Generate(gen *protogen.Plugin, lang, packageSuffix string) error {
	reg := Registry(packageSuffix)
	ctx := factory.Ctx{Plugin: gen}

	src, ok := reg.Sources["proto"]
	if !ok {
		return fmt.Errorf("no %q source registered", "proto")
	}
	model, err := src.Build(ctx)
	if err != nil {
		return fmt.Errorf("build model from %s source: %w", src.Name(), err)
	}

	tgt, ok := reg.Targets["mcp"]
	if !ok {
		return fmt.Errorf("no %q target registered (have: %s)", "mcp", reg.TargetNames())
	}

	langs, err := resolveLanguages(tgt, lang)
	if err != nil {
		return err
	}
	for _, l := range langs {
		if err := tgt.Generate(ctx, model, l); err != nil {
			return fmt.Errorf("target %s (%s): %w", tgt.Name(), l, err)
		}
	}
	return nil
}

// resolveLanguages expands [LangAll] and validates a single language against
// what the target actually emits.
func resolveLanguages(tgt factory.Target[*Model], lang string) ([]string, error) {
	supported := tgt.Languages()
	if lang == LangAll {
		return supported, nil
	}
	if slices.Contains(supported, lang) {
		return []string{lang}, nil
	}
	return nil, fmt.Errorf("unsupported language %q (supported: %s, %s)",
		lang, strings.Join(supported, ", "), LangAll)
}
