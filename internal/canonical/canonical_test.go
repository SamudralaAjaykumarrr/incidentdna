package canonical

import (
	"testing"
)

func TestMarshalJSON_KeyOrderIndependent(t *testing.T) {
	a := []byte(`{"b": 1, "a": 2, "c": {"y": 1, "x": 2}}`)
	b := []byte(`{"c": {"x": 2, "y": 1}, "a": 2, "b": 1}`)

	got, err := MarshalJSON(a)
	if err != nil {
		t.Fatalf("MarshalJSON(a): %v", err)
	}
	want, err := MarshalJSON(b)
	if err != nil {
		t.Fatalf("MarshalJSON(b): %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("key-order-independent inputs produced different canonical bytes:\n got: %s\nwant: %s", got, want)
	}
	if string(got) != `{"a":2,"b":1,"c":{"x":2,"y":1}}` {
		t.Fatalf("unexpected canonical form: %s", got)
	}
}

func TestMarshalJSON_WhitespaceIndependent(t *testing.T) {
	compact := []byte(`{"a":[1,2,3]}`)
	spaced := []byte("{\n  \"a\": [1, 2, 3]\n}\n")

	got, err := MarshalJSON(compact)
	if err != nil {
		t.Fatalf("MarshalJSON(compact): %v", err)
	}
	want, err := MarshalJSON(spaced)
	if err != nil {
		t.Fatalf("MarshalJSON(spaced): %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("whitespace-only difference changed canonical bytes:\n got: %s\nwant: %s", got, want)
	}
}

func TestMarshalJSON_ContentChangeProducesDifferentBytes(t *testing.T) {
	a := []byte(`{"a":1}`)
	b := []byte(`{"a":2}`)

	got, err := MarshalJSON(a)
	if err != nil {
		t.Fatalf("MarshalJSON(a): %v", err)
	}
	other, err := MarshalJSON(b)
	if err != nil {
		t.Fatalf("MarshalJSON(b): %v", err)
	}
	if string(got) == string(other) {
		t.Fatalf("differing content produced identical canonical bytes: %s", got)
	}
}

func TestMarshalJSON_NumberFormattingPreserved(t *testing.T) {
	got, err := MarshalJSON([]byte(`{"a": 10}`))
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	if string(got) != `{"a":10}` {
		t.Fatalf("unexpected number formatting: %s", got)
	}
}

func TestMarshalJSON_RejectsTrailingData(t *testing.T) {
	_, err := MarshalJSON([]byte(`{"a":1} garbage`))
	if err == nil {
		t.Fatal("expected error for trailing data after JSON value, got nil")
	}
}

func TestMarshal_UsesStructTags(t *testing.T) {
	type inner struct {
		B int `json:"b"`
		A int `json:"a"`
	}
	got, err := Marshal(inner{B: 2, A: 1})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(got) != `{"a":1,"b":2}` {
		t.Fatalf("unexpected canonical form: %s", got)
	}
}

func TestMarshalJSON_ArrayOrderSignificant(t *testing.T) {
	a := []byte(`{"a":[1,2]}`)
	b := []byte(`{"a":[2,1]}`)

	got, err := MarshalJSON(a)
	if err != nil {
		t.Fatalf("MarshalJSON(a): %v", err)
	}
	other, err := MarshalJSON(b)
	if err != nil {
		t.Fatalf("MarshalJSON(b): %v", err)
	}
	if string(got) == string(other) {
		t.Fatal("array element order should be significant, but canonical bytes matched")
	}
}
