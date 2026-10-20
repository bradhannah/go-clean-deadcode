package contract

import (
	"bytes"
	"os"
	"testing"
)

func TestPublishedSchemasMatchWireTypes(t *testing.T) {
	for _, kind := range []string{"config", "report"} {
		want, err := Schema(kind)
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile("../../schemas/" + kind + "-v1.schema.json")
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("%s schema drift; run go run ./scripts/schema-gen", kind)
		}
	}
}
