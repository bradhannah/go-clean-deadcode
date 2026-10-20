package api

import "testing"

func TestOnlyTested(t *testing.T) {
	if onlyTested() != 3 {
		t.Fatal("unexpected value")
	}
}
