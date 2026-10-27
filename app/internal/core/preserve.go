package core

import (
	"encoding/json"
	"os"
)

// Merge unknown fields by object key and stable record ID; removed records stay removed.
func preserveJSON(path string, b []byte) ([]byte, error) {
	old, e := os.ReadFile(path)
	if os.IsNotExist(e) {
		return b, nil
	}
	if e != nil {
		return nil, e
	}
	var a, c any
	if json.Unmarshal(old, &a) != nil {
		return b, nil
	}
	if e = json.Unmarshal(b, &c); e != nil {
		return nil, e
	}
	return json.MarshalIndent(mergeUnknown(a, c), "", "  ")
}
func mergeUnknown(old, next any) any {
	switch n := next.(type) {
	case map[string]any:
		o, ok := old.(map[string]any)
		if !ok {
			return n
		}
		for k, v := range n {
			o[k] = mergeUnknown(o[k], v)
		}
		return o
	case []any:
		o, ok := old.([]any)
		if !ok {
			return n
		}
		byID := map[any]any{}
		for _, v := range o {
			if m, ok := v.(map[string]any); ok && m["id"] != nil {
				byID[m["id"]] = v
			}
		}
		for i, v := range n {
			if m, ok := v.(map[string]any); ok && m["id"] != nil {
				n[i] = mergeUnknown(byID[m["id"]], v)
			}
		}
		return n
	}
	return next
}
