package tests

import (
	"bytes"
	"encoding/json"
	"maps"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/zadykian/cld/internal/tabs"
)

// schemaFile is tabs.json's schema in the repository, which tabs.Schema names on main.
const schemaFile = "internal/tabs/tabs.v1.schema.json"

// schemaPath is schemaFile from the tests' directory.
var schemaPath = filepath.Join("..", filepath.FromSlash(schemaFile))

// recordSchema is tabs.json's schema, compiled once for every record readRecord reads.
var recordSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return jsonschema.NewCompiler().Compile(schemaPath)
})

// checkSchema reports a record, data, that tabs.json's schema does not allow.
func checkSchema(t *testing.T, data []byte) {
	t.Helper()
	schema, err := recordSchema()
	if err != nil {
		t.Fatal(err)
	}
	record, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err == nil {
		err = schema.Validate(record)
	}
	if err != nil {
		t.Errorf("tabs.json against its schema: %#v\n%s", err, data)
	}
}

// Each object of the schema has the json names of a type's fields as its properties, from
// tabs.Record down. A field added to one fails the test until the schema has it.
func TestTabsSchemaFields(t *testing.T) {
	t.Parallel()
	type object struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	var schema struct {
		object
		ID   string            `json:"$id"`
		Defs map[string]object `json:"$defs"`
	}
	if err := json.Unmarshal([]byte(readFile(t, schemaPath)), &schema); err != nil {
		t.Fatal(err)
	}
	want := "https://raw.githubusercontent.com/zadykian/cld/main/" + schemaFile
	if tabs.Schema != want || schema.ID != want {
		t.Errorf("tabs.Schema is %s and the schema's $id %s, want both %s", tabs.Schema, schema.ID,
			want)
	}
	for def, value := range map[string]any{
		"": tabs.Record{}, "run": tabs.Run{}, "pane": tabs.Pane{}, "size": tabs.Size{},
		"last": tabs.Last{}, "window": tabs.Window{}, "tab": tabs.Tab{}, "node": tabs.Node{},
		"loss": tabs.Loss{}, "anchor": tabs.Anchor{},
	} {
		properties := schema.Properties
		if def != "" {
			properties = schema.Defs[def].Properties
		}
		var fields []string
		for field := range reflect.TypeOf(value).Fields() {
			name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
			fields = append(fields, name)
		}
		if got := slices.Sorted(maps.Keys(properties)); !slices.Equal(got, slices.Sorted(
			slices.Values(fields))) {
			t.Errorf("the schema's %q has %v, want %T's %v", def, got, value, fields)
		}
	}
}
