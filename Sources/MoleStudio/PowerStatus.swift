import Foundation
import IOKit

/// Battery, thermal and memory-pressure facts for the status page and the menu-bar popover.
/// Everything here comes from public interfaces: the `AppleSmartBattery` IORegistry entry,
/// `ProcessInfo.thermalState` and the `kern.memorystatus_vm_pressure_level` sysctl.
enum PowerStatus {
    /// JSON-ready snapshot. `battery` is absent on Macs without a battery.
    static func snapshot() -> [String: Any] {
        var result: [String: Any] = [
            "thermal": thermalLabel(),
            "memPressure": memoryPressureLabel()
        ]
        if let battery = batteryInfo() {
            result["battery"] = battery
        }
        return result
    }

    private static func thermalLabel() -> String {
        switch ProcessInfo.processInfo.thermalState {
        case .fair: return "fair"
        case .serious: return "serious"
        case .critical: return "critical"
        default: return "nominal"
        }
    }

    private static func memoryPressureLabel() -> String {
        var level: Int32 = 0
        var size = MemoryLayout<Int32>.size
        let status = sysctlbyname("kern.memorystatus_vm_pressure_level", &level, &size, nil, 0)
        guard status == 0 else { return "normal" }
        switch level {
        case 4: return "critical"
        case 2: return "warn"
        default: return "normal"
        }
    }

    // MARK: - Battery

    private static func batteryInfo() -> [String: Any]? {
        let service = IOServiceGetMatchingService(kIOMainPortDefault, IOServiceMatching("AppleSmartBattery"))
        guard service != 0 else { return nil }
        defer { IOObjectRelease(service) }
        var unmanaged: Unmanaged<CFMutableDictionary>?
        let status = IORegistryEntryCreateCFProperties(service, &unmanaged, kCFAllocatorDefault, 0)
        guard status == KERN_SUCCESS else { return nil }
        guard let props = unmanaged?.takeRetainedValue() as? [String: Any] else { return nil }
        var info = capacityFields(props)
        for (key, value) in extraFields(props) {
            info[key] = value
        }
        info["charging"] = flag(props, "IsCharging")
        info["plugged"] = flag(props, "ExternalConnected")
        info["full"] = flag(props, "FullyCharged")
        return info
    }

    private static func number(_ props: [String: Any], _ key: String) -> Int? {
        (props[key] as? NSNumber)?.intValue
    }

    private static func flag(_ props: [String: Any], _ key: String) -> Bool {
        (props[key] as? NSNumber)?.boolValue ?? false
    }

    /// Current charge level, in percent.
    private static func chargePercent(_ props: [String: Any]) -> Int? {
        guard let current = number(props, "CurrentCapacity") else { return nil }
        guard let maximum = number(props, "MaxCapacity"), maximum > 0 else { return nil }
        return min(100, current * 100 / maximum)
    }

    /// "Maximum capacity": full-charge capacity against design capacity, in percent.
    private static func healthPercent(_ props: [String: Any]) -> Int? {
        guard let design = number(props, "DesignCapacity"), design > 0 else { return nil }
        let nominal = number(props, "NominalChargeCapacity")
        guard let full = nominal ?? number(props, "AppleRawMaxCapacity") else { return nil }
        return min(100, full * 100 / design)
    }

    /// Minutes to empty or to full, as reported by the battery. Values outside this range mean "still calculating".
    private static func minutesEstimate(_ props: [String: Any]) -> Int? {
        guard let minutes = number(props, "TimeRemaining") else { return nil }
        guard minutes > 0, minutes < 6000 else { return nil }
        return minutes
    }

    private static func adapterWatts(_ props: [String: Any]) -> Int? {
        guard let adapter = props["AdapterDetails"] as? [String: Any] else { return nil }
        return number(adapter, "Watts")
    }

    private static func capacityFields(_ props: [String: Any]) -> [String: Any] {
        var info: [String: Any] = [:]
        if let value = chargePercent(props) { info["percent"] = value }
        if let value = healthPercent(props) { info["healthPct"] = value }
        return info
    }

    /// Cycle count, temperature (reported in hundredths of a degree C), time estimate, adapter wattage.
    private static func extraFields(_ props: [String: Any]) -> [String: Any] {
        var info: [String: Any] = [:]
        if let value = number(props, "CycleCount") { info["cycles"] = value }
        if let value = number(props, "Temperature") { info["tempC"] = Double(value) / 100.0 }
        if let value = minutesEstimate(props) { info["minutes"] = value }
        if let value = adapterWatts(props) { info["adapterWatts"] = value }
        return info
    }
}
