// Package plugin looks methods up by name, so reachability must assume it could
// call any exported method of a type it is given. It is only given types from
// the standard library.
package plugin

import (
	"reflect"
	"strings"
)

// Call runs the named method of v.
func Call(v any, name string) {
	reflect.ValueOf(v).MethodByName(name).Call(nil)
}

// Start resets a builder by name.
func Start() { Call(new(strings.Builder), "Reset") }
