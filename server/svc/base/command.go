package base

import "encoding/json"

// Op is a BaseCommand operation (CONTRACT.md).
type Op string

const (
	OpBuild         Op = "build"
	OpUpgrade       Op = "upgrade"
	OpMove          Op = "move"
	OpRemove        Op = "remove"
	OpCollect       Op = "collect"
	OpSpeedup       Op = "speedup"
	OpClearObstacle Op = "clear_obstacle"
)

// Command is one entry of POST /v1/base/commands. Rules.Apply evaluates only the
// placement ops (build, move); the other ops need prices and timers and go through
// Engine.Plan, so Rules.Apply returns CodeNotImplemented for them.
//
// On the wire the fields may sit at the top level or inside `args` ({op,args} in
// docs/03-api.md); see UnmarshalJSON.
type Command struct {
	Op   Op     `json:"op"`
	UID  string `json:"uid"`
	Type string `json:"type,omitempty"` // build only
	X    int    `json:"x"`
	Y    int    `json:"y"`
	Rot  int    `json:"rot"`
	// To is the target level of an upgrade (optional; makes a retried upgrade a no-op).
	To int `json:"to,omitempty"`
}

// UnmarshalJSON accepts both {op,uid,...} and {op,args:{uid,...}}; args wins.
func (c *Command) UnmarshalJSON(b []byte) error {
	type plain Command
	var w struct {
		plain
		Args *plain `json:"args"`
	}
	if err := json.Unmarshal(b, &w); err != nil {
		return err
	}
	*c = Command(w.plain)
	if w.Args != nil {
		op := c.Op
		*c = Command(*w.Args)
		c.Op = op
	}
	return nil
}

// Result reports what Apply did. Replayed=true means the command was already
// in effect, so the layout was returned unchanged (idempotent retry).
type Result struct {
	Replayed bool
}

// Apply validates and applies one placement command, returning a new layout.
// The input layout is never modified. On error the returned layout is the input.
//
// Idempotency: build with an existing uid, same type and same placement, and move
// to the current placement are no-ops (Replayed). Build with an existing uid but a
// different type/placement is a conflict.
//
// ASSUMPTION: build/move check bounds, terrain, overlap, unlock and limits only; the
// "HQ not sealed" rule is enforced by ValidateLayout on save, so transient editing
// states are allowed.
func (r Rules) Apply(l Layout, c Command) (Layout, Result, error) {
	switch c.Op {
	case OpBuild:
		return r.build(l, c)
	case OpMove:
		return r.move(l, c)
	case OpUpgrade, OpRemove, OpCollect, OpSpeedup, OpClearObstacle:
		return l, Result{}, fail(CodeNotImplemented, c.UID, "op %q is not implemented yet", c.Op)
	}
	return l, Result{}, fail(CodeBadCommand, c.UID, "unknown op %q", c.Op)
}

// ApplyAll applies commands in order, all-or-nothing: on the first error the
// original layout is returned together with that error.
func (r Rules) ApplyAll(l Layout, cmds []Command) (Layout, []Result, error) {
	cur := l
	res := make([]Result, 0, len(cmds))
	for _, c := range cmds {
		next, rs, err := r.Apply(cur, c)
		if err != nil {
			return l, nil, err
		}
		cur = next
		res = append(res, rs)
	}
	return cur, res, nil
}

func (r Rules) build(l Layout, c Command) (Layout, Result, error) {
	if c.UID == "" {
		return l, Result{}, fail(CodeBadCommand, "", "build needs a uid")
	}
	d, ok := r.Cat.Def(c.Type)
	if !ok {
		return l, Result{}, fail(CodeUnknownType, c.UID, "unknown type %q", c.Type)
	}
	if i := l.Find(c.UID); i >= 0 {
		ex := l.Items[i]
		if ex.Type == c.Type && ex.X == c.X && ex.Y == c.Y && ex.Rot == c.Rot {
			return l, Result{Replayed: true}, nil
		}
		return l, Result{}, fail(CodeConflict, c.UID, "uid already used by a different %s", ex.Type)
	}
	if d.Kind == KindCore || d.Kind == KindObstacle {
		return l, Result{}, fail(CodeNotPlaceable, c.UID, "%s cannot be built by command", c.Type)
	}
	hq := l.HQLevel(r.Cat)
	if hq == 0 {
		return l, Result{}, fail(CodeNoHQ, c.UID, "layout has no HQ")
	}
	it := Item{UID: c.UID, Type: c.Type, Level: 1, X: c.X, Y: c.Y, Rot: c.Rot}
	if err := r.checkShape(it); err != nil {
		return l, Result{}, err
	}
	if err := r.checkUnlock(it, hq); err != nil {
		return l, Result{}, err
	}
	out := l.Clone()
	out.Items = append(out.Items, it)
	if _, err := r.occupy(out); err != nil {
		return l, Result{}, err
	}
	if err := r.checkCount(out, c.Type, hq); err != nil {
		return l, Result{}, err
	}
	return out, Result{}, nil
}

func (r Rules) move(l Layout, c Command) (Layout, Result, error) {
	i := l.Find(c.UID)
	if i < 0 {
		return l, Result{}, fail(CodeNotFound, c.UID, "no such item")
	}
	ex := l.Items[i]
	d, _ := r.Cat.Def(ex.Type)
	if d.Kind == KindObstacle {
		return l, Result{}, fail(CodeNotPlaceable, c.UID, "obstacles cannot be moved")
	}
	if ex.X == c.X && ex.Y == c.Y && ex.Rot == c.Rot {
		return l, Result{Replayed: true}, nil
	}
	moved := ex
	moved.X, moved.Y, moved.Rot = c.X, c.Y, c.Rot
	if err := r.checkShape(moved); err != nil {
		return l, Result{}, err
	}
	out := l.Clone()
	out.Items[i] = moved
	if _, err := r.occupy(out); err != nil {
		return l, Result{}, err
	}
	return out, Result{}, nil
}
