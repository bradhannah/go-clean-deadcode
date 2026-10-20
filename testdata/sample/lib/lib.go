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
