package sim

// RNG is a splitmix64 stream: tiny, deterministic, identical on all platforms.
type RNG struct{ s uint64 }

func NewRNG(seed uint64) *RNG { return &RNG{s: seed} }
func (r *RNG) Next() uint64 {
	r.s += 0x9E3779B97F4A7C15
	z := r.s
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	return z ^ (z >> 31)
}

// Intn returns [0,n). Uses multiply-shift (no modulo bias for our n).
func (r *RNG) Intn(n int) int { return int((r.Next() >> 33) * uint64(n) >> 31) }
