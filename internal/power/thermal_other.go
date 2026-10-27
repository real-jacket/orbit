//go:build !darwin

package power

// appkitThermalState is unavailable away from macOS; thermalLabel falls back to
// the sysctl, and then to nominal.
func appkitThermalState() (int, bool) { return 0, false }
