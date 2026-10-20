// Package core is internal: exporting a name here does not make it API.
package core

func Used() int { return 1 }

func Unused() int { return 2 }

type Base struct{}

func (Base) Promoted() int { return promoted() }

func promoted() int { return 3 }
