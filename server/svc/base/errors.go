package base

import "fmt"

// Code is a machine-readable rule violation.
type Code string

const (
	CodeUnknownType    Code = "unknown_type"
	CodeBadRot         Code = "bad_rotation"
	CodeBadLevel       Code = "bad_level"
	CodeOutOfBounds    Code = "out_of_bounds"
	CodeOverlap        Code = "overlap"
	CodeLocked         Code = "hq_too_low"
	CodeLimit          Code = "limit_reached"
	CodeNeedsWater     Code = "requires_water"
	CodeOnWater        Code = "on_water"
	CodeSealed         Code = "sealed_base"
	CodeNoHQ           Code = "no_hq"
	CodeDuplicateUID   Code = "duplicate_uid"
	CodeNotFound       Code = "not_found"
	CodeConflict       Code = "conflict"
	CodeNotPlaceable   Code = "not_placeable"
	CodeNotImplemented Code = "not_implemented"
	CodeBadCommand     Code = "bad_command"
	CodeNoPrice        Code = "no_price"
	CodeNoBuilder      Code = "no_free_builder"
	CodeMaxLevel       Code = "max_level"
	CodeBusy           Code = "busy"
	CodeHQGate         Code = "hq_gate"
	CodeInsufficient   Code = "insufficient_resources"
	CodeLayoutChanged  Code = "layout_mismatch"
)

// Error is a validation failure. API mapping (see APICode): CodeConflict -> `conflict`,
// CodeInsufficient -> `insufficient_resources`, CodeNotFound -> `not_found`,
// everything else -> `invalid`.
type Error struct {
	Code Code
	UID  string
	Msg  string
}

func (e *Error) Error() string {
	if e.UID != "" {
		return fmt.Sprintf("base: %s (%s): %s", e.Code, e.UID, e.Msg)
	}
	return fmt.Sprintf("base: %s: %s", e.Code, e.Msg)
}

func fail(c Code, uid, format string, a ...any) *Error {
	return &Error{Code: c, UID: uid, Msg: fmt.Sprintf(format, a...)}
}

// APICode maps a rule violation to the error code of docs/03-api.md.
func APICode(c Code) string {
	switch c {
	case CodeConflict, CodeBusy, CodeNoBuilder:
		return "conflict"
	case CodeInsufficient:
		return "insufficient_resources"
	case CodeNotFound:
		return "not_found"
	}
	return "invalid"
}
