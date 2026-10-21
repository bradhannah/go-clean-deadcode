package main

import (
	"example.com/reflection/panel"
	"example.com/reflection/plugin"
	"example.com/reflection/shop"
)

func main() {
	plugin.Start()
	_ = shop.Print()
	_ = shop.Count()
	_ = shop.Tallies()
	_ = shop.Describe()
	_ = panel.Open()
}
