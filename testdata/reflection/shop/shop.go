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
