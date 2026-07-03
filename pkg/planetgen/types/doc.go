// Package types defines shared data types for the planetgen
// pipeline that need to be referenced by both the root planetgen
// package (for Generate dispatch) and the render subpackage
// (which consumes the types as renderer parameters). Splitting
// these into a leaf package breaks the otherwise-circular
// planetgen ↔ render import.
package types
