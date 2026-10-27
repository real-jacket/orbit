package main

// This file ports the Swift app's applyTheme.
//
// The page owns the interface colours: it stores the user's choice under the
// localStorage key orbit_theme and posts it back through the bridge on every
// load. What the shell has to do is the native part — set the app appearance so
// native controls and the title bar follow it, and repaint the window's own
// background so the area behind the page matches. The Swift app kept the choice
// in UserDefaults under orbit.theme; MyGo has no equivalent, so it lives in a
// small JSON file in the app's user data directory.

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
)

// Appearance names. These are the three values the page sends.
const (
	themeSystem = "system"
	themeLight  = "light"
	themeDark   = "dark"
)

// Window background colours. These are the values of --bg-window in
// Resources/src-ui/index.html, so the native window behind the page matches it
// while the page is still loading.
const (
	darkBackground  = "#111110"
	lightBackground = "#faf8f5"
)

// applyTheme sets the app appearance, remembers the choice when asked to, and
// repaints the window background.
func applyTheme(theme string, remember bool) {
	switch theme {
	case themeDark:
		mygo.Theme.SetSource(mygo.ThemeDark)
	case themeLight:
		mygo.Theme.SetSource(mygo.ThemeLight)
	default:
		mygo.Theme.SetSource(mygo.ThemeSystem)
	}
	if remember {
		if theme == themeSystem {
			clearSavedTheme()
		} else {
			saveTheme(theme)
		}
	}
	setWindowBackground(windowBackground(theme))
}

// windowBackground returns the background colour for a theme. Asking the system
// only happens for the system theme, where the answer is its business: for an
// explicit theme the page has already said which way to go, and MyGo reports the
// appearance it was told to set, which may not have been applied yet.
func windowBackground(theme string) string {
	if theme == themeDark {
		return darkBackground
	}
	if theme == themeLight {
		return lightBackground
	}
	if mygo.Theme.IsDark() {
		return darkBackground
	}
	return lightBackground
}

// setWindowBackground paints the main window's background. The menu-bar panel
// is left alone: it shows a popover, whose material sits behind a page that
// draws its own background.
func setWindowBackground(color string) {
	w := activeMainWindow()
	if w == nil {
		return
	}
	_ = w.SetBackgroundColor(color)
}

// preferences is the shape of the file the chosen theme is kept in.
type preferences struct {
	Theme string `json:"theme"`
}

// preferencesPath returns the file the app's own preferences live in, creating
// the directory it sits in.
func preferencesPath() (string, error) {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "preferences.json"), nil
}

// loadSavedTheme returns the theme the user last chose, or the system theme
// when none has been stored or the file cannot be read.
func loadSavedTheme() string {
	path, err := preferencesPath()
	if err != nil {
		return themeSystem
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return themeSystem
	}
	var saved preferences
	if err := json.Unmarshal(data, &saved); err != nil {
		return themeSystem
	}
	switch saved.Theme {
	case themeDark, themeLight:
		return saved.Theme
	}
	return themeSystem
}

// saveTheme stores an explicit theme choice.
func saveTheme(theme string) {
	path, err := preferencesPath()
	if err != nil {
		return
	}
	data, err := json.Marshal(preferences{Theme: theme})
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0o644)
}

// clearSavedTheme drops the stored choice, which is what choosing the system
// theme means: follow the system from now on.
func clearSavedTheme() {
	path, err := preferencesPath()
	if err != nil {
		return
	}
	_ = os.Remove(path)
}
