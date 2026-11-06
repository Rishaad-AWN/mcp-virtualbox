package tools

import (
	"context"
	"encoding/json"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Rishaad-AWN/mcp-virtualbox/internal/vbox"
	"github.com/Rishaad-AWN/mcp-virtualbox/internal/vboxweb"
)

// deps carries everything the tool handlers need.
type deps struct {
	vb    *vbox.Client
	mouse *vboxweb.Client
}

// handler is the simplified per-tool handler signature used throughout this
// package. The generic In type is decoded from the validated JSON arguments.
type handler[In any] func(ctx context.Context, in In) (*mcp.CallToolResult, error)

// add registers a tool with a hand-built input schema (so descriptions can be
// plain Go strings and defaults are applied by the SDK), adapting our simple
// handler shape to the SDK's typed handler.
func add[In any](s *mcp.Server, name, desc string, schema *jsonschema.Schema, h handler[In]) {
	tool := &mcp.Tool{Name: name, Description: desc, InputSchema: schema}
	mcp.AddTool(s, tool, func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
		res, err := h(ctx, in)
		return res, nil, err
	})
}

// --- schema builders -------------------------------------------------------

type prop struct {
	name   string
	schema *jsonschema.Schema
}

func p(name string, s *jsonschema.Schema) prop { return prop{name: name, schema: s} }

// object builds an object schema. required lists the property names that must
// be supplied; every other property is optional (and may carry a default).
func object(required []string, props ...prop) *jsonschema.Schema {
	m := make(map[string]*jsonschema.Schema, len(props))
	order := make([]string, 0, len(props))
	for _, pr := range props {
		m[pr.name] = pr.schema
		order = append(order, pr.name)
	}
	return &jsonschema.Schema{
		Type:          "object",
		Properties:    m,
		PropertyOrder: order,
		Required:      required,
	}
}

func str(desc string) *jsonschema.Schema {
	return &jsonschema.Schema{Type: "string", Description: desc}
}
func integer(desc string) *jsonschema.Schema {
	return &jsonschema.Schema{Type: "integer", Description: desc}
}
func boolean(desc string) *jsonschema.Schema {
	return &jsonschema.Schema{Type: "boolean", Description: desc}
}

func enum(desc string, vals ...string) *jsonschema.Schema {
	e := make([]any, len(vals))
	for i, v := range vals {
		e[i] = v
	}
	return &jsonschema.Schema{Type: "string", Description: desc, Enum: e}
}

func array(desc string, items *jsonschema.Schema) *jsonschema.Schema {
	return &jsonschema.Schema{Type: "array", Description: desc, Items: items}
}

// def attaches a default value to a schema. Properties with a default are
// treated as optional and the SDK fills the value in before the handler runs.
func def(s *jsonschema.Schema, v any) *jsonschema.Schema {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err) // defaults are compile-time literals; marshal cannot fail
	}
	s.Default = json.RawMessage(b)
	return s
}

// --- result builders -------------------------------------------------------

func text(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
}

func errText(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}, IsError: true}
}

// jsonText renders v as pretty JSON text content.
func jsonText(v any) *mcp.CallToolResult {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return errText("failed to encode result: " + err.Error())
	}
	return text(string(b))
}

// fail converts a VBoxManage error into a consistently formatted error result.
func fail(err error) *mcp.CallToolResult {
	return errText(vbox.FormatError(err))
}
