// Command schema-gen refreshes the checked-in wire schemas in this repository.
package main

import (
	"fmt"
	"os"

	"github.com/bradhannah/go-clean-deadcode/internal/contract"
)

func main() {
	for _, kind := range []string{"config", "report"} {
		data, err := contract.Schema(kind)
		if err == nil {
			err = os.WriteFile("schemas/"+kind+"-v1.schema.json", data, 0644)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}
