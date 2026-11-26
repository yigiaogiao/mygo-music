package main

import (
	"log"

	"github.com/egoist/mygo"

	"mygo-music/internal/app"
	"mygo-music/internal/version"
)

func main() {
	// The browser has no package to read a version from: it's this app's.
	mygo.App.SetVersion(version.Version)
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
