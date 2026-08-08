package cli

import "errors"

// errNotImplemented marks scaffolded commands whose behavior lands in a later
// milestone (GO-PORT-PLAN.md §4). It exists so the command tree — flags,
// argument handling, passthrough semantics — can be built and tested before
// the implementations arrive.
var errNotImplemented = errors.New("not implemented yet in the Go port")
