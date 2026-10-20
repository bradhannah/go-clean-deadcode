package shop

import "testing"

func TestTotal(t *testing.T) {
	if (Receipt{amount: 2}).Total() != 4 {
		t.Fatal("total")
	}
}
