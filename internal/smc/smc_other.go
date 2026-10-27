//go:build !darwin

package smc

import "errors"

// ErrUnsupported is returned on platforms without an AppleSMC.
var ErrUnsupported = errors.New("smc: only available on macOS")

// Client is an open AppleSMC connection.
type Client struct{}

// SharedClient returns an error on platforms without an AppleSMC.
func SharedClient() (*Client, error) { return nil, ErrUnsupported }

// Open returns ErrUnsupported on platforms without an AppleSMC.
func Open() (*Client, error) { return nil, ErrUnsupported }

// Float reports that no key can be read off macOS.
func (c *Client) Float(string) (float64, bool) { return 0, false }

// CPUTemperature reports that no CPU temperature is readable here.
func (c *Client) CPUTemperature() (float64, bool) { return 0, false }

// BatteryTemperature reports that no battery thermistor is readable here.
func (c *Client) BatteryTemperature() (float64, bool) { return 0, false }

// Fans reports that no fan tachometer is readable here.
func (c *Client) Fans() []int { return nil }

// FanCount reports that there are no readable fans here.
func (c *Client) FanCount() int { return 0 }

// StructSize reports 0: the SMC ABI does not exist on this platform.
func StructSize() int { return 0 }
