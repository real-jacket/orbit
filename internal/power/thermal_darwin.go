//go:build darwin

package power

import (
	"sync"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

var (
	thermalLoadOnce sync.Once
	thermalReady    bool
	processInfoSel  objc.SEL
	thermalStateSel objc.SEL
)

// appkitThermalState asks Foundation for -[NSProcessInfo thermalState], which is
// what the Swift build read through ProcessInfo.thermalState. It reports false
// when Foundation cannot be reached, so the caller can fall back to sysctl.
//
// A pure Go binary does not link Foundation, so its classes are absent from the
// Objective-C runtime until the framework is dlopened here.
func appkitThermalState() (int, bool) {
	thermalLoadOnce.Do(func() {
		if _, err := purego.Dlopen(
			"/System/Library/Frameworks/Foundation.framework/Foundation",
			purego.RTLD_GLOBAL|purego.RTLD_NOW,
		); err != nil {
			return
		}
		class := objc.GetClass("NSProcessInfo")
		if class == 0 {
			return
		}
		processInfoSel = objc.RegisterName("processInfo")
		thermalStateSel = objc.RegisterName("thermalState")
		info := objc.ID(class).Send(processInfoSel)
		if info == 0 {
			return
		}
		thermalReady = true
	})
	if !thermalReady {
		return 0, false
	}
	info := objc.ID(objc.GetClass("NSProcessInfo")).Send(processInfoSel)
	if info == 0 {
		return 0, false
	}
	state := int(objc.Send[uintptr](info, thermalStateSel))
	if state < thermalStateNominal || state > thermalStateCritical {
		return 0, false
	}
	return state, true
}
