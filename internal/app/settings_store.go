//go:build !js

package app

// A desktop build keeps the settings in settings.json, in the user data
// directory, so there is no other store to read or write.
func loadBrowserSettings(*settings) {}

func saveBrowserSettings(settings) {}
