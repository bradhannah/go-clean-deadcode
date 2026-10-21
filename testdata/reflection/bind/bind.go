// Package bind registers a type's fields by reflection, as UI and scripting
// bindings do. Such code can read any field and, through unsafe, set
// unexported ones, so source alone cannot show which fields matter.
package bind

import "reflect"

// Register returns the field names of T.
func Register[T any]() []string {
	t := reflect.TypeFor[T]()
	names := make([]string, 0, t.NumField())
	for i := range t.NumField() {
		names = append(names, t.Field(i).Name)
	}
	return names
}
