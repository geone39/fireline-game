package sim

// Apply feeds one command and returns the animation events. A refused command is not an error:
// it is recorded in Result.Rejected and leaves the state unchanged (the only exception is the implicit
// EndTurn that a turn change in Command.Turn performs, spec 4.2). ErrNotImplemented leaves the state unchanged
// and does not consume a log index.
func (b *Battle) Apply(cmd Command) ([]Event, error) {
	evs, code, err := b.apply(cmd)
	if err != nil {
		return nil, err
	}
	idx := b.nCmd
	b.nCmd++
	if code != 0 {
		b.rejected = append(b.rejected, Rejection{Index: idx, Code: code})
		return evs, nil // only the implicit EndTurn events, if any
	}
	return evs, nil
}

func (b *Battle) apply(c Command) ([]Event, RejectCode, error) {
	if b.ph == phaseOver {
		return nil, RejectBattleOver, nil
	}
	if c.Op > OpRetreat {
		return nil, RejectBadPhase, nil
	}
	// phase / op compatibility
	deployOp := c.Op == OpDeploy || c.Op == OpLoad || c.Op == OpStart
	switch {
	case c.Op == OpRetreat:
	case deployOp && b.ph != phaseDeploy:
		return nil, RejectBadPhase, nil
	case !deployOp && b.ph != phasePlayer:
		return nil, RejectBadPhase, nil
	}
	// turn: 0 in deployment; in the player phase t (current) or t+1 (implicit EndTurn of t)
	var evs []Event
	if b.ph == phaseDeploy {
		if c.Turn != 0 {
			return nil, RejectBadTurn, nil
		}
	} else {
		if c.Turn < b.turn || c.Turn > b.turn+1 {
			return nil, RejectBadTurn, nil
		}
		if c.Turn == b.turn+1 {
			evs = append(evs, b.endTurn()...)
			if b.ph == phaseOver {
				return evs, RejectBattleOver, nil
			}
		}
	}

	var more []Event
	var code RejectCode
	switch c.Op {
	case OpDeploy:
		more, code = b.cmdDeploy(c)
	case OpLoad:
		more, code = b.cmdLoad(c)
	case OpStart:
		more, code = b.cmdStart()
	case OpMove:
		more, code = b.cmdMove(c)
	case OpAttack:
		more, code = b.cmdAttack(c)
	case OpEndTurn:
		more = b.endTurn()
	case OpRetreat:
		b.finish(EndRetreat)
		more = []Event{{Kind: EventBattleEnded, Value: int(EndRetreat)}}
	default: // OpAbility, OpCast: SIM-5
		return nil, 0, ErrNotImplemented
	}
	if code != 0 {
		return evs, code, nil
	}
	evs = append(evs, more...)
	if b.ph != phaseOver {
		if r := b.checkEnd(); r != EndNone {
			b.finish(r)
			evs = append(evs, Event{Kind: EventBattleEnded, Value: int(r)})
		}
	}
	return evs, 0, nil
}

// finish closes the battle. TurnsUsed is read from b.turn (0 while still in deployment).
func (b *Battle) finish(r EndReason) {
	b.ph = phaseOver
	b.endReason = r
}

// checkEnd evaluates the end conditions that follow an action (spec 10.1): full destruction, no attacker units.
func (b *Battle) checkEnd() EndReason {
	if b.ph == phaseDeploy {
		return EndNone
	}
	done, total := b.destruction()
	if total > 0 && done == total {
		return EndFullDestruction
	}
	for i := range b.ents {
		e := &b.ents[i]
		if e.owner == ownerAttacker && e.alive {
			return EndNone
		}
	}
	return EndNoUnits
}

// endTurn closes the player phase of turn t. The defence phase (spec 7) is SIM-3: until then it is a no-op.
// Then either the max_turns fuse ends the battle or turn t+1 starts.
func (b *Battle) endTurn() []Event {
	ev := []Event{{Kind: EventTurnEnded, Value: b.turn}}
	// SIM-3: defence phase goes here (triggers, zone effects, buildings, garrison, timers).
	if b.turn >= b.rules.MaxTurns {
		b.finish(EndTurnLimit)
		return append(ev, Event{Kind: EventBattleEnded, Value: int(EndTurnLimit)})
	}
	b.turn++
	b.startPlayerTurn()
	return ev
}

func (b *Battle) startPlayerTurn() {
	for i := range b.ents {
		e := &b.ents[i]
		if e.owner == ownerAttacker && e.alive {
			e.apLeft = e.ap
			if e.cd > 0 {
				e.cd--
			}
		}
	}
}

func (b *Battle) cmdStart() ([]Event, RejectCode) {
	any := false
	for i := range b.ents {
		if b.ents[i].owner == ownerAttacker && b.ents[i].alive {
			any = true
			break
		}
	}
	if !any {
		return nil, RejectInvalidTarget // nothing deployed: the player may Retreat instead
	}
	b.ph = phasePlayer
	b.turn = 1
	b.startPlayerTurn()
	return nil, 0
}
