package lib

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
