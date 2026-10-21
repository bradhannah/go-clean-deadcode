package shop

import "testing"

type pricer interface{ Price() int }

// fakePricer's Price is required only by its conversion to pricer.
type fakePricer struct{}

func (fakePricer) Price() int { return 1 }

func TestFakeConverts(t *testing.T) {
	var p pricer = fakePricer{}
	if p == nil {
		t.Fatal("nil")
	}
}
