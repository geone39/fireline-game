package sim

import "math/bits"

// xxHash64 (seed 0), little-endian input - the StateHash function of spec 11.5. Implemented here to keep sim
// dependency-free (the same bytes hash identically on native and WASM).
const (
	xxP1 uint64 = 11400714785074694791
	xxP2 uint64 = 14029467366897019727
	xxP3 uint64 = 1609587929392839161
	xxP4 uint64 = 9650029242287828579
	xxP5 uint64 = 2870177450012600261
)

func xxRound(acc, in uint64) uint64 { return bits.RotateLeft64(acc+in*xxP2, 31) * xxP1 }
func xxMerge(h, v uint64) uint64    { return (h^xxRound(0, v))*xxP1 + xxP4 }
func le64(b []byte) uint64 {
	return uint64(b[0]) | uint64(b[1])<<8 | uint64(b[2])<<16 | uint64(b[3])<<24 |
		uint64(b[4])<<32 | uint64(b[5])<<40 | uint64(b[6])<<48 | uint64(b[7])<<56
}

func xxh64(b []byte) uint64 {
	n := len(b)
	var h uint64
	if n >= 32 {
		p1, p2 := xxP1, xxP2
		v1, v2, v3, v4 := p1+p2, p2, uint64(0), -p1
		for len(b) >= 32 {
			v1 = xxRound(v1, le64(b))
			v2 = xxRound(v2, le64(b[8:]))
			v3 = xxRound(v3, le64(b[16:]))
			v4 = xxRound(v4, le64(b[24:]))
			b = b[32:]
		}
		h = bits.RotateLeft64(v1, 1) + bits.RotateLeft64(v2, 7) + bits.RotateLeft64(v3, 12) + bits.RotateLeft64(v4, 18)
		h = xxMerge(h, v1)
		h = xxMerge(h, v2)
		h = xxMerge(h, v3)
		h = xxMerge(h, v4)
	} else {
		h = xxP5
	}
	h += uint64(n)
	for len(b) >= 8 {
		h ^= xxRound(0, le64(b))
		h = bits.RotateLeft64(h, 27)*xxP1 + xxP4
		b = b[8:]
	}
	if len(b) >= 4 {
		h ^= uint64(uint32(b[0])|uint32(b[1])<<8|uint32(b[2])<<16|uint32(b[3])<<24) * xxP1
		h = bits.RotateLeft64(h, 23)*xxP2 + xxP3
		b = b[4:]
	}
	for _, c := range b {
		h ^= uint64(c) * xxP5
		h = bits.RotateLeft64(h, 11) * xxP1
	}
	h ^= h >> 33
	h *= xxP2
	h ^= h >> 29
	h *= xxP3
	h ^= h >> 32
	return h
}

type hbuf struct{ b []byte }

func (h *hbuf) i(v int) {
	u := uint64(int64(v))
	h.b = append(h.b, byte(u), byte(u>>8), byte(u>>16), byte(u>>24), byte(u>>32), byte(u>>40), byte(u>>48), byte(u>>56))
}
func (h *hbuf) u(v uint64) { h.i(int(int64(v))) }
func (h *hbuf) s(s string) {
	h.i(len(s))
	h.b = append(h.b, s...)
}
func (h *hbuf) t(v bool) {
	if v {
		h.i(1)
	} else {
		h.i(0)
	}
}

// stateHash is spec 11.5: turn, phase, entities by ascending id (id, kind, content id, level, owner, cell, hp,
// ap_left, cd, effects, heat; plus alive/onField/containment), remaining stock, RNG state, rejection log.
func (b *Battle) stateHash() uint64 {
	var h hbuf
	h.i(b.turn)
	h.i(int(b.ph))
	h.i(len(b.ents))
	for i := range b.ents {
		e := &b.ents[i]
		h.i(int(e.id))
		h.i(int(e.kind))
		h.s(e.contentID)
		h.i(e.level)
		h.i(int(e.owner))
		h.i(int(e.pos.X))
		h.i(int(e.pos.Y))
		h.i(e.hp)
		h.i(e.apLeft)
		h.i(e.cd)
		h.i(0) // effects (SIM-2+)
		h.i(0) // heat (SIM-2+)
		h.t(e.alive)
		h.t(e.onField)
		h.i(int(e.aboard))
		h.i(int(e.inside))
		h.t(e.revealed)
	}
	h.i(len(b.stock))
	for _, s := range b.stock {
		h.s(s.ID)
		h.i(s.Level)
		h.i(s.Count)
	}
	h.t(b.heroUsed)
	h.u(b.rng.s)
	h.i(len(b.rejected))
	for _, r := range b.rejected {
		h.i(r.Index)
		h.i(int(r.Code))
	}
	return xxh64(h.b)
}
