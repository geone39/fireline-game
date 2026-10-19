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
)

// Error is a validation failure. API mapping: CodeConflict -> `conflict`,
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
