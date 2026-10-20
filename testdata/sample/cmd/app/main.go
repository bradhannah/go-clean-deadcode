package main

import "example.com/sample/lib"

func main() {
	lib.Live()
	lib.Dispatch(lib.Real{})
	lib.Generic(3)
	lib.RunJob(lib.NewJob())
	lib.ShapeName()
	lib.Settings()
	lib.HeaderSize()
}
