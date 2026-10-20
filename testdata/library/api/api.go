// Package api is a library with no executable; importers outside it call its API.
package api

import "example.com/library/internal/core"

var registered = register()

func register() int { return 1 }

// Exported is API and reaches an unexported helper and an internal function.
func Exported() int { return helper() + core.Used() }

func helper() int { return 1 }

func unusedHelper() int { return 2 }

func onlyTested() int { return 3 }

// Engine promotes the exported methods of an internal type.
type Engine struct{ core.Base }

func (e *Engine) Run() int { return run() }

func run() int { return 4 }

type hidden struct{}

// Value is callable by importers through the interface New returns.
func (hidden) Value() int { return value() }

func value() int { return 5 }

func New() interface{ Value() int } { return hidden{} }

// Map is generic API.
func Map[T any](x T) T { return generic(x) }

func generic[T any](x T) T { return x }
