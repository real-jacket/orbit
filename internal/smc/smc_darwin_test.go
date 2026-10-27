//go:build darwin

package smc

import (
	"math"
	"testing"
)

// TestStructSizeIsTheKernelLayout pins the ABI: AppleSMC reads these bytes by
// offset, so a field added or reordered here silently breaks every reading.
func TestStructSizeIsTheKernelLayout(t *testing.T) {
	if got := StructSize(); got != 80 {
		t.Fatalf("StructSize() = %d, want 80 (sizeof(SMCKeyData_t))", got)
	}
}

func TestKeyPackingIsBigEndian(t *testing.T) {
	cases := map[string]uint32{
		"TC0P": 0x54433050,
		"#KEY": 0x234B4559,
		"F0Ac": 0x46304163,
		"TB0T": 0x54423054,
	}
	for name, want := range cases {
		if got := key(name); got != want {
			t.Errorf("key(%q) = %#x, want %#x", name, got, want)
		}
	}
}

// The values below are the bytes a real Apple M1 Max reports for TB0T — the
// battery thermistor — and for F0Mn, the minimum fan speed.
func TestDecodeFloatReadsAppleSiliconLittleEndian(t *testing.T) {
	battery := decodeFloat(t, "flt ", []byte{0xcc, 0xcc, 0x0c, 0x42})
	if math.Abs(battery-35.2) > 0.01 {
		t.Errorf("TB0T decoded to %.3f, want 35.2", battery)
	}
	fan := decodeFloat(t, "flt ", []byte{0x00, 0x00, 0x96, 0x44})
	if math.Abs(fan-1200) > 0.01 {
		t.Errorf("F0Mn decoded to %.3f, want 1200", fan)
	}
}

// Intel Macs store the same data type big endian; the decoder must notice that
// the little endian reading is impossible rather than report nonsense.
func TestDecodeFloatFallsBackToBigEndian(t *testing.T) {
	value := decodeFloat(t, "flt ", []byte{0x42, 0x0c, 0xcc, 0xcd})
	if math.Abs(value-35.2) > 0.01 {
		t.Errorf("decoded to %.3f, want 35.2", value)
	}
}

func TestDecodeSp78IsSignedFixedPoint(t *testing.T) {
	cases := []struct {
		raw  []byte
		want float64
	}{
		{[]byte{0x23, 0x80}, 35.5},
		{[]byte{0x00, 0x00}, 0},
		{[]byte{0xfd, 0x00}, -3},
	}
	for _, c := range cases {
		if got := decodeFloat(t, "sp78", c.raw); math.Abs(got-c.want) > 0.0001 {
			t.Errorf("sp78 %x decoded to %.4f, want %.4f", c.raw, got, c.want)
		}
	}
}

func TestDecodeIntegerTypes(t *testing.T) {
	cases := []struct {
		dataType string
		raw      []byte
		want     float64
	}{
		{"fpe2", []byte{0x12, 0xc0}, 1200},
		{"ui8 ", []byte{0x02}, 2},
		{"ui16", []byte{0x00, 0x03}, 3},
		{"ui32", []byte{0x00, 0x00, 0x08, 0xcb}, 2251},
	}
	for _, c := range cases {
		if got := decodeFloat(t, c.dataType, c.raw); math.Abs(got-c.want) > 0.0001 {
			t.Errorf("%q %x decoded to %.4f, want %.4f", c.dataType, c.raw, got, c.want)
		}
	}
}

func TestDecodeRejectsWhatItCannotRead(t *testing.T) {
	cases := []struct {
		dataType string
		raw      []byte
	}{
		{"flt ", []byte{0x00, 0x00}},
		{"sp78", []byte{0x00}},
		{"fpe2", []byte{0x00}},
		{"ui8 ", []byte{0x00, 0x00}},
		{"ui16", []byte{0x00}},
		{"ui32", []byte{0x00}},
		{"hex_", []byte{0x00, 0x00, 0x00, 0x00}},
		{"", nil},
	}
	for _, c := range cases {
		if got, ok := decode(c.dataType, c.raw); ok {
			t.Errorf("decode(%q, %x) = %v, true; want a refusal", c.dataType, c.raw, got)
		}
	}
}

// TestAppKitSMCIsReachable checks the private interface still answers on the
// machine running the tests. It skips (rather than fails) on machines without a
// readable AppleSMC, such as CI virtual machines.
func TestAppKitSMCIsReachable(t *testing.T) {
	client, err := SharedClient()
	if err != nil {
		t.Skipf("AppleSMC is not reachable here: %v", err)
	}
	temp, ok := client.CPUTemperature()
	if !ok {
		// A VM can expose AppleSMC without any of the temperature keys; that is
		// an environment limitation, not a defect in the decoder.
		t.Skip("AppleSMC is open but this machine exposes no CPU temperature key")
	}
	if temp < 5 || temp > 125 {
		t.Fatalf("CPU temperature %.1f°C is outside the range a Mac can report", temp)
	}
	// A fan may legitimately stand still on a cool machine, so zero is allowed;
	// anything above 12000 RPM would mean the decoder mixed up units.
	for _, speed := range client.Fans() {
		if speed < 0 || speed > 12000 {
			t.Fatalf("fan speed %d RPM is outside the range a Mac can report", speed)
		}
	}
}

func decodeFloat(t *testing.T, dataType string, raw []byte) float64 {
	t.Helper()
	value, ok := decode(dataType, raw)
	if !ok {
		t.Fatalf("decode(%q, %x) refused", dataType, raw)
	}
	return value
}
