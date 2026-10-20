package lib

import "encoding/binary"

func Live()         {}
func Utility()      {}
func Legacy()       { legacyHelper() }
func legacyHelper() {}
func DeadA()        { DeadB() }
func DeadB()        { DeadA() }
func RecursiveLive(n int) {
	if n > 0 {
		RecursiveLive(n - 1)
	}
}
func init() { RecursiveLive(2) }

type Worker interface{ Work() }
type Real struct{}

func (Real) Work() {}

// This is not an interface marker even though Real implements Worker.
func (Real) unrelatedEmpty() {}

type Unused struct{}

func (Unused) Work() {}

var _ Worker = Unused{}

func Dispatch(w Worker)        { w.Work() }
func Generic[T any](v T) T     { return v }
func DeadGeneric[T any](v T) T { return v }

func callback(f func()) { f() }
func CallbackUsed()     {}
func init()             { callback(CallbackUsed) }

func SubtestOnly() {}

type sealed interface{ seal() }
type Hidden struct{}

func (Hidden) seal() {}

var _ sealed = Hidden{}

// finisher is checked dynamically; RunJob only calls finish, yet a runtime type
// needs label too, or the assertion silently fails. Unexported methods are the
// case reachability alone misses.
type finisher interface {
	finish(step int)
	label() string
}

type job struct{}

func (job) finish(int)    {}
func (job) label() string { return "job" }

// idleJob satisfies finisher but never reaches the assertion.
type idleJob struct{}

func (idleJob) finish(int)    {}
func (idleJob) label() string { return "idle" }

func NewJob() any { return job{} }

func RunJob(x any) {
	if f, ok := x.(finisher); ok {
		f.finish(1)
	}
}

// Obsolete is reached only by tests; removing it also removes TestObsolete and
// then exposes obsoleteSetup, which only TestObsolete called.
func Obsolete() {}

// Shape is an enum in use: Square is an unused value, which is a note only.
type Shape int

const (
	Circle Shape = iota
	Square
)

func currentShape() Shape { return Circle }

func ShapeName() string {
	if currentShape() == Circle {
		return "circle"
	}
	return ""
}

// Nothing uses these declarations.
type deadConfig struct{ size int }

const deadLimit = 3

var deadTable = []int{1}

// registry's initializer has effects, so it stays although nothing reads it.
var registry = register()

func register() int { return 1 }

// settings has an unused field, a never-assigned field, an exported field and a
// tagged field; only the first two are findings.
type settings struct {
	name     string
	unused   int
	enabled  bool
	Exported int
	tagged   int `layout:"t"`
}

// box is generic: its methods use instantiated fields, which are the declared ones.
type box[T any] struct {
	value T
	spare int
}

func (b *box[T]) Get() T { return b.value }

func Boxed() int { b := &box[int]{value: 1}; return b.Get() }

// digest is written only through a slice of the array, which still assigns it.
type digest struct{ sum [4]byte }

func Digest() byte {
	var d digest
	copy(d.sum[:], "abcd")
	return d.sum[0]
}

func Settings() string {
	s := settings{name: "x"}
	if s.enabled {
		return "on"
	}
	return s.name
}

// header is laid out by encoding/binary, so its unused field stays.
type header struct {
	magic uint32
	spare uint16
}

func HeaderSize() int { return binary.Size(header{}) }

// testOnlyKind is used only by tests.
type testOnlyKind struct{}
