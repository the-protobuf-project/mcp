// Copyright 2026 The Protobuf Project authors.
// SPDX-License-Identifier: Apache-2.0

package generator_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Generated files carry no separator comments (`// ----`, `// ====`,
// `// ── section ──`) and no TODO, FIXME or XXX: output people commit is not a
// place for scaffolding or a to-do list. The goldens and the committed
// examples are everything the plugin writes, so checking them checks it.
var (
	styleComment   = regexp.MustCompile(`^\s*(//|#|--|/\*|\*|<!--)`)
	styleSeparator = regexp.MustCompile(`-{4,}|={4,}|[─━]{2,}`)
	styleTodo      = regexp.MustCompile(`\b(TODO|FIXME|XXX)\b`)
)

func TestGeneratedStyle(t *testing.T) {
	for _, root := range []string{"../../examples/proto/generated", "../../examples/mime/gen"} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !styleChecked(path) {
				return err
			}
			checkGeneratedStyle(t, path)
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
}

// styleChecked reports whether path is protoc-gen-mcp output: a golden file, or
// under an examples tree, and not protoc's own (`.pb.*`, `_pb2*`, binary
// descriptor sets).
func styleChecked(path string) bool {
	slash := filepath.ToSlash(path)
	if strings.Contains(slash, "testdata/") && !strings.Contains(slash, "/golden/") {
		return false
	}
	return !strings.Contains(slash, ".pb.") && !strings.Contains(slash, "_pb2") && !strings.HasSuffix(slash, ".binpb")
}

func checkGeneratedStyle(t *testing.T, path string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for i, line := range strings.Split(string(b), "\n") {
		if styleComment.MatchString(line) && styleSeparator.MatchString(line) {
			t.Errorf("%s:%d: separator comment in generated output: %s", path, i+1, strings.TrimSpace(line))
		}
		if styleTodo.MatchString(line) {
			t.Errorf("%s:%d: to-do marker in generated output: %s", path, i+1, strings.TrimSpace(line))
		}
	}
}
