package taskdump

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
)

// TaskFields are the top-level JSON keys of a TaskDocument, in output order.
var TaskFields = jsonFieldNames(reflect.TypeFor[TaskDocument]())

// Printed whatever the field selection is.
var alwaysSelected = []string{"id", "warnings"}

func jsonFieldNames(t reflect.Type) []string {
	var names []string
	for i := 0; i < t.NumField(); i++ {
		name, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ",")
		if name != "" && name != "-" {
			names = append(names, name)
		}
	}
	return names
}

// ValidateFields checks that every name is one of TaskFields.
func ValidateFields(fields []string) error {
	for _, f := range fields {
		if !slices.Contains(TaskFields, f) {
			return fmt.Errorf("unknown field %q (valid fields: %v)", f, strings.Join(TaskFields, ", "))
		}
	}
	return nil
}

// MarshalJSON encodes v like json.Marshal, but without escaping <, > and & (they
// are common in markdown) and without a trailing newline.
func MarshalJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// SelectFields encodes doc as a JSON object that has only the given top-level fields,
// in TaskFields order; no fields means all of them. id and warnings are always kept,
// and comments_omitted goes with comments.
func SelectFields(doc *TaskDocument, fields []string) (json.RawMessage, error) {
	raw, err := MarshalJSON(doc)
	if err != nil || len(fields) == 0 {
		return raw, err
	}
	var values map[string]json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, err
	}
	keep := func(name string) bool {
		return slices.Contains(fields, name) || slices.Contains(alwaysSelected, name) ||
			(name == "comments_omitted" && slices.Contains(fields, "comments"))
	}
	var buf bytes.Buffer
	buf.WriteByte('{')
	for _, name := range TaskFields {
		value, ok := values[name]
		if !ok || !keep(name) {
			continue
		}
		if buf.Len() > 1 {
			buf.WriteByte(',')
		}
		fmt.Fprintf(&buf, "%q:", name)
		buf.Write(value)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}
