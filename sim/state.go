package sim

// BattlePhase is the stage of a battle as the client sees it.
type BattlePhase uint8

const (
	PhaseDeploy BattlePhase = iota // phase 0: Deploy / Load / Start
	PhasePlayer                    // the player acts (turn 1..N)
	PhaseOver                      // finished, see State.EndReason
)

// ZoneKind is the kind of a timed zone effect (spells and hero abilities).
type ZoneKind uint8

const (
	ZoneAirstrike ZoneKind = iota + 1
	ZoneMedkit
	ZoneSmoke
	ZoneAdrenaline
)

// EntityState is the client view of one entity (contract 0.2.5). ID is the canonical EntityID (spec 2.4) and is
// stable for the whole battle; entities are never removed, a dead one has Alive=false.
type EntityState struct {
	ID        EntityID
	Owner     uint8 // 0 defender, 1 attacker
	Kind      EntityKind
	ContentID string
	Level     int
	Pos       Cell // top-left tile; meaningful when OnField
	W, H      int
	HP, MaxHP int
	AP        int // action points per turn (0 for buildings)
	APLeft    int
	Cooldown  int // turns until the hero ability is ready again
	Alive     bool
	OnField   bool     // occupies grid cells (false: inside a bunker / aboard a carrier)
	Aboard    EntityID // carrier, 0 = none
	Inside    EntityID // bunker holding the unit, 0 = none
	Cargo     []EntityID
	Revealed  bool
	// Effects, as the last turn they are active (0 = none): jammed / EMP, hero hidden by a_vanish, decoy lifetime.
	DisabledUntil int
	StealthUntil  int
	DecoyUntil    int
}

// ZoneState is an active timed zone (smoke, airstrike, medkit, adrenaline) for rendering.
type ZoneState struct {
	Kind         ZoneKind
	Centre       Cell
	R            int
	First, Last  int // active during the turns First..Last
	DmgPermille  int
	MovePermille int
}

// State is a snapshot of the world at the current moment. It is derived data: it never changes the battle and is
// not part of StateHash. It contains no hidden information: a live, unrevealed hidden trap (OQ-7.8) is left out
// (its EntityID is simply skipped), the RNG state is not exposed.
type State struct {
	Phase          BattlePhase
	Turn           int // 0 in deployment
	EndReason      EndReason
	Entities       []EntityState // ascending ID
	Zones          []ZoneState   // in the order cast
	SpellsLeft     []ArmySlot    // remaining support calls
	SpellsThisTurn int
	StockLeft      []ArmySlot // remaining army to deploy
	HeroUsed       bool
}

// State returns the client view of the current world.
func (b *Battle) State() State {
	s := State{Turn: b.turn, EndReason: b.endReason, SpellsThisTurn: b.spellsTurn, HeroUsed: b.heroUsed}
	switch b.ph {
	case phaseDeploy:
		s.Phase = PhaseDeploy
	case phasePlayer:
		s.Phase = PhasePlayer
	default:
		s.Phase = PhaseOver
	}
	s.Entities = make([]EntityState, 0, len(b.ents))
	for i := range b.ents {
		e := &b.ents[i]
		if e.bd != nil && e.bd.Hidden && !e.revealed && e.alive {
			continue
		}
		es := EntityState{ID: e.id, Owner: uint8(e.owner), Kind: e.kind, ContentID: e.contentID, Level: e.level,
			Pos: e.pos, W: e.w, H: e.h, HP: e.hp, MaxHP: e.maxHP, AP: e.ap, APLeft: e.apLeft, Cooldown: e.cd,
			Alive: e.alive, OnField: e.onField, Aboard: e.aboard, Inside: e.inside, Revealed: e.revealed,
			DisabledUntil: e.disabledUntil, StealthUntil: e.stealthUntil, DecoyUntil: e.decoyUntil}
		if len(e.cargo) > 0 {
			es.Cargo = append([]EntityID(nil), e.cargo...)
		}
		s.Entities = append(s.Entities, es)
	}
	for i := range b.zones {
		z := &b.zones[i]
		s.Zones = append(s.Zones, ZoneState{Kind: ZoneKind(z.kind), Centre: z.centre, R: z.r, First: z.first, Last: z.last,
			DmgPermille: z.dmgPm, MovePermille: z.movePm})
	}
	s.SpellsLeft = append([]ArmySlot(nil), b.spells...)
	s.StockLeft = append([]ArmySlot(nil), b.stock...)
	return s
}
