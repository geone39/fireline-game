package sim

import "testing"

// Golden test: if this changes, replays recorded earlier become invalid -> bump ContentVersion/engine version.
func TestRNGGolden(t *testing.T) {
	r := NewRNG(42)
	got := [3]uint64{r.Next(), r.Next(), r.Next()}
	want := [3]uint64{13679457532755275413, 2949826092126892291, 5139283748462763858}
	if got != want {
		t.Fatalf("rng changed: %v", got)
	}
}
func TestIntnRange(t *testing.T) {
	r := NewRNG(1)
	for i := 0; i < 10000; i++ {
		if v := r.Intn(7); v < 0 || v > 6 {
			t.Fatal(v)
		}
	}
}
