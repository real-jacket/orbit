// Package smc reads the System Management Controller.
//
// Temperatures and fan speeds are not exposed by any public API on macOS; they
// live behind AppleSMC's IOConnect method. This is the same interface
// smcFanControl, osx-cpu-temp and FriendsOfMole use: open the AppleSMC service,
// then exchange a fixed 80-byte SMCKeyData structure with
// IOConnectCallStructMethod. The struct layout is part of the on-disk API and
// has been stable for a decade, which is why it is spelled out here.
//
// On Apple Silicon the temperature keys are floats ("flt ", little endian);
// Intel Macs use the older "sp78" fixed-point encoding. Both are decoded.
package smc

import (
	"errors"
	"math"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// SMC commands (KERNEL_INDEX_SMC protocol).
const (
	cmdReadBytes   = 5
	cmdReadKeyInfo = 9
	selectorKernel = 2
)

// Temperature keys worth trying, in order of preference. Apple Silicon reports
// per-core temperatures as Tp0x; Intel Macs use the TC0x die sensors.
var cpuTempKeys = []string{
	"Tp01", "Tp05", "Tp09", "Tp0D", "Tp02", "Tp06", "Tp0A", "Tp0E",
	"TC0P", "TC0D", "TC0E", "TC0F", "TC0H", "TC0J",
}

// batteryTempKeys are the battery thermistor keys, in the order macOS itself
// reads them.
var batteryTempKeys = []string{"TB0T", "TB1T", "TB2T"}

// fanKeys are the fan tachometer keys for fan 0..1; FNum says how many exist.
var fanKeys = []string{"F0Ac", "F1Ac"}

// KeyData is the request/response structure AppleSMC expects. Field order and
// size (80 bytes) mirror the kernel's SMCKeyData_t exactly — the padding is
// part of the ABI, not an accident.
type KeyData struct {
	Key     uint32
	Vers    [6]byte
	_       [2]byte
	PLimit  [16]byte
	KeyInfo struct {
		DataSize   uint32
		DataType   uint32
		Attributes byte
		_          [3]byte
	}
	Result byte
	Status byte
	Data8  byte
	_      [1]byte
	Data32 uint32
	Bytes  [32]byte
}

// Client is an open AppleSMC connection.
type Client struct {
	mu   sync.Mutex
	conn uint32
}

// Shared is the process-wide SMC connection, opened on first use.
var (
	sharedOnce sync.Once
	shared     *Client
	sharedErr  error
)

// SharedClient returns the process-wide client, or an error on machines where
// AppleSMC is not reachable (for example inside a restricted sandbox).
func SharedClient() (*Client, error) {
	sharedOnce.Do(func() {
		shared, sharedErr = Open()
	})
	return shared, sharedErr
}

var (
	libIOKit, libSystem uintptr
	serviceMatching     func(string) uintptr
	getMatchingService  func(uint32, uintptr) uint32
	serviceOpen         func(uint32, uint32, uint32, *uint32) int32
	connectStructMethod func(uint32, uint32, uintptr, uint, uintptr, *uint) int32
	objectRelease       func(uint32) int32
	machTaskSelf        func() uint32
	libOnce             sync.Once
	libErr              error
)

// loadIOKit resolves the IOKit entry points once.
func loadIOKit() error {
	libOnce.Do(func() {
		libIOKit, libErr = purego.Dlopen(
			"/System/Library/Frameworks/IOKit.framework/IOKit",
			purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if libErr != nil {
			return
		}
		purego.RegisterLibFunc(&serviceMatching, libIOKit, "IOServiceMatching")
		purego.RegisterLibFunc(&getMatchingService, libIOKit, "IOServiceGetMatchingService")
		purego.RegisterLibFunc(&serviceOpen, libIOKit, "IOServiceOpen")
		purego.RegisterLibFunc(&connectStructMethod, libIOKit, "IOConnectCallStructMethod")
		purego.RegisterLibFunc(&objectRelease, libIOKit, "IOObjectRelease")

		libSystem, libErr = purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_NOW)
		if libErr != nil {
			return
		}
		// mach_task_self() is the documented accessor for the task port;
		// reading the mach_task_self_ global would need an unsafe uintptr
		// round trip that the vet unsafeptr check rightly rejects.
		purego.RegisterLibFunc(&machTaskSelf, libSystem, "mach_task_self")
	})
	return libErr
}

// Open connects to AppleSMC.
func Open() (*Client, error) {
	if err := loadIOKit(); err != nil {
		return nil, err
	}
	matching := serviceMatching("AppleSMC")
	if matching == 0 {
		return nil, errors.New("smc: IOServiceMatching(AppleSMC) failed")
	}
	service := getMatchingService(0, matching)
	if service == 0 {
		return nil, errors.New("smc: AppleSMC not found")
	}
	defer objectRelease(service)
	var conn uint32
	if kr := serviceOpen(service, machTaskSelf(), 0, &conn); kr != 0 {
		return nil, errors.New("smc: IOServiceOpen failed")
	}
	return &Client{conn: conn}, nil
}

// call exchanges one SMCKeyData with the kernel.
func (c *Client) call(in KeyData) (KeyData, bool) {
	var out KeyData
	size := uint(unsafe.Sizeof(out))
	c.mu.Lock()
	kr := connectStructMethod(c.conn, selectorKernel,
		uintptr(unsafe.Pointer(&in)), uint(unsafe.Sizeof(in)),
		uintptr(unsafe.Pointer(&out)), &size)
	c.mu.Unlock()
	if kr != 0 || out.Result != 0 {
		return KeyData{}, false
	}
	return out, true
}

// key packs a four-character key the way SMC expects it (big endian).
func key(name string) uint32 {
	var k uint32
	for i := 0; i < 4; i++ {
		var ch byte
		if i < len(name) {
			ch = name[i]
		}
		k = k<<8 | uint32(ch)
	}
	return k
}

// read returns a key's raw bytes and its four-character data type.
func (c *Client) read(name string) ([]byte, string, bool) {
	info, ok := c.call(KeyData{Key: key(name), Data8: cmdReadKeyInfo})
	if !ok || info.KeyInfo.DataSize == 0 || info.KeyInfo.DataSize > 32 {
		return nil, "", false
	}
	in := KeyData{Key: key(name), Data8: cmdReadBytes}
	in.KeyInfo.DataSize = info.KeyInfo.DataSize
	values, ok := c.call(in)
	if !ok {
		return nil, "", false
	}
	dataType := string([]byte{
		byte(info.KeyInfo.DataType >> 24), byte(info.KeyInfo.DataType >> 16),
		byte(info.KeyInfo.DataType >> 8), byte(info.KeyInfo.DataType),
	})
	return values.Bytes[:info.KeyInfo.DataSize], dataType, true
}

// decode turns a key's bytes into a number using its SMC data type.
func decode(dataType string, raw []byte) (float64, bool) {
	switch dataType {
	case "flt ":
		if len(raw) != 4 {
			return 0, false
		}
		// Apple Silicon stores these little endian, Intel big endian. Both are
		// tried and the one that lands in a plausible range wins.
		le := math.Float32frombits(uint32(raw[0]) | uint32(raw[1])<<8 | uint32(raw[2])<<16 | uint32(raw[3])<<24)
		if !absurd(float64(le)) {
			return float64(le), true
		}
		be := math.Float32frombits(uint32(raw[3]) | uint32(raw[2])<<8 | uint32(raw[1])<<16 | uint32(raw[0])<<24)
		if !absurd(float64(be)) {
			return float64(be), true
		}
		return 0, false
	case "sp78":
		if len(raw) != 2 {
			return 0, false
		}
		// sp78 is a signed 8.8 fixed point number: whole degrees in the first
		// byte, 1/256ths in the second.
		return float64(int8(raw[0])) + float64(raw[1])/256, true
	case "fpe2":
		if len(raw) != 2 {
			return 0, false
		}
		return float64(uint16(raw[0])<<8|uint16(raw[1])) / 4, true
	case "ui8 ":
		if len(raw) != 1 {
			return 0, false
		}
		return float64(raw[0]), true
	case "ui16":
		if len(raw) != 2 {
			return 0, false
		}
		return float64(uint16(raw[0])<<8 | uint16(raw[1])), true
	case "ui32":
		if len(raw) != 4 {
			return 0, false
		}
		return float64(uint32(raw[0])<<24 | uint32(raw[1])<<16 | uint32(raw[2])<<8 | uint32(raw[3])), true
	}
	return 0, false
}

// absurd reports whether a decoded float is so far out of range that the byte
// order must be the other way around. It decides the endianness of "flt " keys
// without assuming which kind of sensor produced the number.
func absurd(v float64) bool {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return true
	}
	abs := math.Abs(v)
	return abs > 1e6 || (abs != 0 && abs < 1e-6)
}

// Float reads one key as a number.
func (c *Client) Float(name string) (float64, bool) {
	raw, dataType, ok := c.read(name)
	if !ok {
		return 0, false
	}
	value, ok := decode(dataType, raw)
	if !ok {
		return 0, false
	}
	return value, true
}

// CPUTemperature returns the hottest CPU sensor the machine exposes.
func (c *Client) CPUTemperature() (float64, bool) {
	best, found := 0.0, false
	for _, name := range cpuTempKeys {
		value, ok := c.Float(name)
		if !ok || value < 5 || value > 125 {
			continue
		}
		if !found || value > best {
			best, found = value, true
		}
	}
	return best, found
}

// BatteryTemperature returns the battery thermistor reading.
func (c *Client) BatteryTemperature() (float64, bool) {
	for _, name := range batteryTempKeys {
		if value, ok := c.Float(name); ok && value > 0 && value < 120 {
			return value, true
		}
	}
	return 0, false
}

// FanCount returns how many fans the machine has, or 0 when the key is absent.
func (c *Client) FanCount() int {
	if value, ok := c.Float("FNum"); ok && value >= 0 && value <= 8 {
		return int(value)
	}
	return 0
}

// Fans returns the current speed of every fan in RPM.
func (c *Client) Fans() []int {
	count := len(fanKeys)
	if n := c.FanCount(); n > 0 && n < count {
		count = n
	}
	speeds := make([]int, 0, count)
	for _, name := range fanKeys[:count] {
		value, ok := c.Float(name)
		if !ok || value <= 0 || value > 12000 {
			continue
		}
		speeds = append(speeds, int(math.Round(value)))
	}
	return speeds
}

// StructSize is exported for the layout test: AppleSMC interprets these bytes
// by offset, so a regression here silently breaks every reading.
func StructSize() int { return int(unsafe.Sizeof(KeyData{})) }
