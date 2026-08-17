package generator

import (
	"strings"

	"github.com/the-protobuf-project/protokit/naming"
	"google.golang.org/protobuf/compiler/protogen"
)

// toSnakeCase converts a CamelCase string to snake_case, treating an acronym as
// one word ("GetHTTPConfig" -> "get_http_config").
func toSnakeCase(s string) string {
	return naming.SnakeCase(s)
}

// toScreamingSnakeCase converts a CamelCase or snake_case string to
// SCREAMING_SNAKE_CASE.
//
// This is the upper-case of [toSnakeCase] rather than naming.ScreamingSnake,
// which splits only on lower->upper boundaries and so runs an acronym into the
// following word ("HTTPService" -> "HTTPSERVICE", not "HTTP_SERVICE").
func toScreamingSnakeCase(s string) string {
	return strings.ToUpper(naming.SnakeCase(s))
}

// rsStringEscape escapes backslashes and double quotes for use inside a Rust "..." string literal.
func rsStringEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

// cppStringEscape escapes backslashes and double quotes for use inside a C++ "..." string literal.
func cppStringEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

// cppTypeName returns the C++ type name for a protobuf message relative to
// currentPkg. Same-package types use the short name (with _ for nested types);
// cross-package types use the fully-qualified :: form.
func cppTypeName(msg *protogen.Message, currentPkg string) string {
	msgPkg := string(msg.Desc.ParentFile().Package())
	fullName := string(msg.Desc.FullName())
	localName := strings.TrimPrefix(fullName, msgPkg+".")
	cppLocal := strings.ReplaceAll(localName, ".", "_")
	if msgPkg == currentPkg {
		return cppLocal
	}
	cppNs := "::" + strings.ReplaceAll(msgPkg, ".", "::")
	return cppNs + "::" + cppLocal
}
