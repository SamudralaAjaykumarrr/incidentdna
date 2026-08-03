package policy

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

// LoadFile reads and decodes an IGP policy document from path. The format
// (JSON or YAML) is chosen by file extension, falling back to content
// sniffing for unrecognized extensions — the exact discipline
// scenario.LoadFile/suite.LoadFile already established, deliberately not
// reused directly since Document is unrelated to either package's own type.
//
// The file is size-capped at MaxPolicyDocumentSize before any parsing
// occurs, so an oversized policy document is rejected without ever being
// fully buffered or handed to a parser. LoadFile performs no semantic
// validation; use Validate for that. It does perform structural decoding
// into a fixed typed struct (never map[string]interface{} or
// interface{}-rooted trees), so a malformed document is still rejected here.
func LoadFile(path string) (*Document, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%s is a directory, not a file", path)
	}
	if err := checkDocumentSize(info.Size()); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	// Belt-and-suspenders against a file that grows between Stat and Open,
	// or a non-regular file whose reported size is unreliable.
	limited := io.LimitReader(f, MaxPolicyDocumentSize+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if err := checkDocumentSize(int64(len(data))); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	doc, err := decode(data, formatHint(path, data))
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return doc, nil
}

type format int

const (
	formatJSON format = iota
	formatYAML
)

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

// decode parses data as a policy document in the given format, targeting
// only the fixed typed Document struct — see docs/threat-model.md, "YAML
// parser abuse", the same rationale idir.Decode/scenario.decode already
// document.
func decode(data []byte, f format) (*Document, error) {
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
