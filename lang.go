package main

// The page owns the interface language the way it owns the theme: the choice
// lives in the page's localStorage (orbit_lang) and is posted through the
// bridge on every load and every switch. Go keeps it for the text it draws
// itself, which the page cannot translate: the menu bar, the menu-bar item's
// tooltip and the updater window. Strings the Go side only sends to the page
// (reasons, receipts, descriptions) stay Chinese here and are translated by the
// page's dictionary (Resources/src-ui/i18n/en-backend.js), so a language switch
// never needs a rescan.

import (
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/egoist/mygo"
)

const (
	langZH = "zh"
	langEN = "en"
)

// currentLang is the language Go-drawn text is in. The zero value reads as
// Chinese, the source language.
var currentLang atomic.Value

func language() string {
	if v, ok := currentLang.Load().(string); ok && v == langEN {
		return langEN
	}
	return langZH
}

// L picks the text for the current language: the Chinese source or its English.
func L(zh, en string) string {
	if language() == langEN {
		return en
	}
	return zh
}

// systemLanguage maps the first language the user reads to one of ours: Chinese
// for any zh tag, English for everything else. It is what the page does too
// (core.js detect) when no choice has been saved.
func systemLanguage(preferred []string) string {
	if len(preferred) == 0 || strings.HasPrefix(strings.ToLower(preferred[0]), "zh") {
		return langZH
	}
	return langEN
}

// localeTag is the BCP 47 tag the framework knows for one of our languages.
func localeTag(lang string) string {
	if lang == langEN {
		return "en"
	}
	return "zh-Hans"
}

// initLanguage settles the language before the first window exists: the saved
// choice, else the system's. It also tells the framework which languages the
// app is written in, so the updater window (whose language follows the app's
// locale) can be either.
func initLanguage() {
	mygo.App.SetLanguages("en", "zh-Hans")
	saved := loadSavedLanguage()
	setLanguageNow(saved, saved != "")
	// The update menu item says its words in the locale that is current when the
	// menu is built, and SetLocale applies after this call returns: build again
	// once it has.
	mygo.App.OnLocaleChanged(func(string) { mygo.App.SetMenu(buildApplicationMenu()) })
}

// setLanguageNow records the language and points the framework's locale at it.
// An explicit choice pins the locale; without one it follows the user's
// languages, which is what the system-language default means.
func setLanguageNow(saved string, explicit bool) {
	lang := saved
	if !explicit {
		lang = systemLanguage(mygo.App.PreferredLanguages())
	}
	currentLang.Store(lang)
	if explicit {
		mygo.App.SetLocale(localeTag(lang))
	} else {
		mygo.App.SetLocale("")
	}
}

// applyLanguage is what the page's setLanguage message ends up in. explicit says
// the user picked the language (it is remembered); otherwise the page is following
// the system and nothing is stored.
func applyLanguage(lang string, explicit bool) {
	if lang != langZH && lang != langEN {
		return
	}
	if explicit {
		saveLanguage(lang)
	} else {
		clearSavedLanguage()
	}
	setLanguageNow(lang, explicit)
	mygo.App.SetMenu(buildApplicationMenu())
	if trayItem != nil {
		trayItem.SetToolTip(trayDefaultToolTip())
		pushRealTelemetry(false) // replaces the tooltip with the live one, in the new language
	}
	pushFlyoutLanguage(lang)
}

func trayDefaultToolTip() string {
	return L("orbit · 点击展开系统状态栏", "orbit · Click to open the menu bar panel")
}

// telemetryToolTip is the menu-bar item's tooltip once a sample exists.
func telemetryToolTip(cpu *float64, memPressure string, freeDiskGb float64) string {
	cpuText := L("未知", "Unknown")
	if cpu != nil {
		cpuText = fmt.Sprintf("%.1f%%", *cpu)
	}
	pressureText := L("未知", "Unknown")
	switch memPressure {
	case "normal":
		pressureText = L("正常", "Normal")
	case "warn":
		pressureText = L("偏高", "High")
	case "critical":
		pressureText = L("很高", "Critical")
	}
	if language() == langEN {
		return fmt.Sprintf("orbit · Last sample\nCPU %s · Memory pressure: %s\nStartup disk free %.1f GB", cpuText, pressureText, freeDiskGb)
	}
	return fmt.Sprintf("orbit · 最近采样\nCPU %s · 内存压力%s\n启动磁盘可用 %.1f GB", cpuText, pressureText, freeDiskGb)
}
