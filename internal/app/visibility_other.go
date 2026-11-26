//go:build !js

package app

// watchSight does nothing: a desktop window's show tells the app itself.
func watchSight(func()) {}
