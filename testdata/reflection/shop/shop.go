// Package shop has methods that only reflection's exported-method rule keeps.
package shop

import "fmt"

// Receipt reaches fmt; nothing calls Total except its test.
type Receipt struct{ amount int }

func (r Receipt) Total() int { return double(r.amount) }

func double(n int) int { return n * 2 }

func Print() string { return fmt.Sprint(Receipt{amount: 1}) }

// Shape's Area is required by converting square to Shape, though nothing calls it.
type Shape interface{ Area() int }

type square struct{}

func (square) Area() int { return 1 }

func Count() int { return len([]Shape{square{}}) }

// counter satisfies the Counted constraint, which requires Reset though no
// code calls it.
type Counted interface{ Reset() }

type counter struct{ n int }

func (c *counter) Reset() { c.n = 0 }

type Tally[T Counted] struct{ items []T }

func Tallies() int { return len(Tally[*counter]{}.items) }

// Only dead code converts tag to Labeler, so Label goes with it.
type Labeler interface{ Label() string }

type tag struct{}

func (tag) Label() string { return "tag" }

func unusedLabel() Labeler { return tag{} }

// Describer is asserted under its own name and through an alias, so its
// obligations must name both, in a stable order.
type Describer interface{ describe() string }

type DescriberAlias = Describer

type item struct{}

func (item) describe() string { return "item" }

// Describe checks an item.
func Describe() bool {
	var v any = item{}
	_, named := v.(Describer)
	_, aliased := v.(DescriberAlias)
	return named || aliased
}
