// Package errors defines the typed errors used by the generator.
//
// Every error returned by a public function in the generator implements
// AppError, which carries a stable string Code (prefixed GEN-*) so callers
// and tests can branch on the code rather than a message. The full inventory
// is in docs/error-codes.md § "Generator error codes".
//
// Since 1.4.0.
package errors

// AppError is the typed error contract. Every public generator function
// returns an error that is either nil or an AppError. Since 1.4.0.
type AppError interface {
	error
	Code() string // stable string, e.g. "GEN-001"
}

// Code constants. Since 1.4.0.
const (
	CodeConfig         = "GEN-001" // missing/invalid env var or flag
	CodeDatabase       = "GEN-002" // connection / migration / tx failure
	CodeIntrospection  = "GEN-003" // information_schema query failed
	CodeGeneration     = "GEN-004" // template parse/execute/write failure
	CodeNaming         = "GEN-005" // identifier cannot be sanitised
	CodeOutputDir      = "GEN-006" // output dir non-empty without --force
	CodeTemplate       = "GEN-007" // template helper missing or errored
)

// GenError is the concrete AppError for the generator. Since 1.4.0.
type GenError struct {
	C   string // the stable code
	Msg string // human-readable detail
	Err error  // optional wrapped cause
}

// Code returns the stable code. Since 1.4.0.
func (e GenError) Code() string { return e.C }

// Error returns a formatted message including the code. Since 1.4.0.
func (e GenError) Error() string {
	if e.Err != nil {
		return e.C + ": " + e.Msg + ": " + e.Err.Error()
	}
	return e.C + ": " + e.Msg
}

// Unwrap exposes the wrapped cause for errors.Is / errors.As. Since 1.4.0.
func (e GenError) Unwrap() error { return e.Err }

// New constructs a GenError without a wrapped cause. Since 1.4.0.
func New(code, msg string) GenError { return GenError{C: code, Msg: msg} }

// Wrap constructs a GenError wrapping a cause. Since 1.4.0.
func Wrap(code, msg string, cause error) GenError {
	return GenError{C: code, Msg: msg, Err: cause}
}
