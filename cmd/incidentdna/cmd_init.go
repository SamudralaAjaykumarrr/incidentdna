package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

const starterTemplate = `# IncidentDNA starter incident document.
#
# Fill in the fields below to describe a production incident as a permanent,
# executable regression scenario. Run "incidentdna validate .incidentdna/incident.yaml"
# once you've edited it, and "incidentdna fingerprint .incidentdna/incident.yaml"
# to see its deterministic identity fingerprint.
#
# See docs/idir-specification-v0.1.md in the incidentdna repository for the
# full field reference.
schema_version: "0.1"

incident:
  id: INC-CHANGE-ME
  title: Short, specific incident title
  summary: >
    One or two sentences describing what happened.
  occurred_at: "2026-01-01T00:00:00Z"

application:
  name: my-application
  service: my-service
  environment: production
  release_version: "0.0.0"

trigger:
  type: change-me
  description: What condition set this incident in motion?

events:
  - id: evt-1
    type: change-me
    description: First event in the causal chain.

services:
  - name: my-service
    role: change-me
    technology_category: change-me

side_effects:
  - type: change-me
    description: The externally observable business consequence.
    affected_entity: change-me
    reversible: true

business_invariants:
  - id: inv-1
    statement: The business rule this incident violated.

expected_corrected_behavior:
  - What should happen instead once this is fixed.

evidence: []

reproduction_sequence:
  - step: 1
    description: First step to reproduce the incident.
    ref_event_id: evt-1

privacy:
  sensitivity: internal
  redacted: false
`

func runInit(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	force := fs.Bool("force", false, "overwrite an existing .incidentdna directory")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: incidentdna init [--force]")
	}
	if err := fs.Parse(args); err != nil {
		return exitError
	}
	if fs.NArg() != 0 {
		fs.Usage()
		return exitError
	}

	dir := ".incidentdna"
	path := filepath.Join(dir, "incident.yaml")

	if _, err := os.Stat(path); err == nil && !*force {
		fmt.Fprintf(os.Stderr, "incidentdna: %s already exists; use --force to overwrite\n", path)
		return exitError
	} else if err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "incidentdna: %v\n", err)
		return exitError
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "incidentdna: %v\n", err)
		return exitError
	}
	if err := os.WriteFile(path, []byte(starterTemplate), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "incidentdna: %v\n", err)
		return exitError
	}

	fmt.Printf("Created %s\n", path)
	return exitOK
}
