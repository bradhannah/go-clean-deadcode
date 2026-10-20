package main

import (
	"example.com/reflection/plugin"
	"example.com/reflection/shop"
)

func main() {
	plugin.Start()
	_ = shop.Print()
	_ = shop.Count()
	_ = shop.Tallies()
}
