package consumer

import (
	"testing"

	"example.com/sample/lib"
)

// checkRetired is a helper in another package that uses test-only lib code.
func checkRetired(t *testing.T) {
	t.Helper()
	lib.Retired()
}

func TestConsumer(t *testing.T) {
	checkRetired(t)
	lib.Live()
}
