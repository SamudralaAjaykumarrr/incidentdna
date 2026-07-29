package idir

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadFile reads and decodes an IDIR document from path. The format (JSON or
// YAML) is chosen by file extension, falling back to content sniffing for
// unrecognized extensions. The file is size-capped at MaxDocumentSize before
// any parsing occurs, so an oversized document is rejected without ever
// being fully buffered or handed to a parser.
//
// LoadFile performs no semantic validation; use the validate package for
// that. It does perform structural decoding, so a malformed document is
// still rejected here.
func LoadFile(path string) (*Document, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%s is a directory, not a file", path)
	}
	if info.Size() > MaxDocumentSize {
		return nil, fmt.Errorf("%s is %d bytes, which exceeds the maximum document size of %d bytes", path, info.Size(), MaxDocumentSize)
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	// Belt-and-suspenders against a file that grows between Stat and Open,
	// or a non-regular file whose reported size is unreliable (e.g. a pipe).
	limited := io.LimitReader(f, MaxDocumentSize+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if len(data) > MaxDocumentSize {
		return nil, fmt.Errorf("%s exceeds the maximum document size of %d bytes", path, MaxDocumentSize)
	}

	doc, err := Decode(data, formatHint(path, data))
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return doc, nil
}

// format identifies which decoder Decode should use.
type format int

const (
	formatJSON format = iota
	formatYAML
)

// formatHint chooses a format based on the file extension, falling back to
// sniffing the first non-whitespace byte of the content (JSON documents
// always start with '{' or '['; IDIR documents are always object-rooted).
func formatHint(path string, data []byte) format {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		return formatJSON
	case ".yaml", ".yml":
		return formatYAML
	}
	trimmed := bytes.TrimLeft(data, " \t\r\n")
	if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
		return formatJSON
	}
	return formatYAML
}

// Decode parses data as an IDIR document in the given format. Decoding uses
// typed struct targets only (never map[string]interface{} or
// interface{}-rooted trees), which keeps the YAML decoder from doing
// anything more elaborate than populating known scalar/slice/struct fields —
// see docs/threat-model.md, "YAML parser abuse".
func Decode(data []byte, f format) (*Document, error) {
	var doc Document
	switch f {
	case formatJSON:
		dec := json.NewDecoder(bytes.NewReader(data))
		if err := dec.Decode(&doc); err != nil {
			return nil, fmt.Errorf("decode JSON: %w", err)
		}
	case formatYAML:
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return nil, fmt.Errorf("decode YAML: %w", err)
		}
	default:
		return nil, fmt.Errorf("unknown format")
	}
	return &doc, nil
}
