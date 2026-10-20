package lib

// A blank function is a compile-time assertion; nothing can call it.
func _() {
	var x [1]struct{}
	_ = x[liveIndex-1]
}

const liveIndex = 1
