//go:build darwin

package telemetry

import (
	"encoding/binary"
	"syscall"
	"unsafe"
)

// netCounters is how many bytes each interface has moved, keyed by interface
// name. Per-interface deltas are needed because the kernel reports these
// counters as 32-bit values in the RTM_IFINFO2 records: on a busy interface they
// wrap every few gigabytes, and a wrap must only cost that one interface's
// interval, never the machine-wide total.
type netCounters map[string]netPair

type netPair struct {
	rx uint64
	tx uint64
}

// The routing sysctl returns one record per interface. if_msghdr2 is 160 bytes
// (offsets verified against <net/route.h>), followed by the interface
// sockaddrs, the first of which is an AF_LINK record that carries the name.
const (
	ctlNet         = 4  // CTL_NET
	pfRoute        = 17 // PF_ROUTE
	netRtIflist2   = 6  // NET_RT_IFLIST2
	rtmIfinfo2     = 18 // RTM_IFINFO2
	ifmHeaderSize  = 160
	ifDataRxOffset = 96  // offsetof(if_msghdr2, ifm_data) + offsetof(if_data64, ifi_ibytes)
	ifDataTxOffset = 104 // … + offsetof(if_data64, ifi_obytes)
	sockaddrDLExt  = 8   // offsetof(sockaddr_dl, sdl_data)
)

// readNetCounters reads every interface's byte counters from the routing sysctl.
func readNetCounters() netCounters {
	mib := [6]int32{ctlNet, pfRoute, 0, 0, netRtIflist2, 0}
	buf, err := sysctlRaw(mib[:])
	if err != nil || len(buf) == 0 {
		return nil
	}
	counters := make(netCounters, 8)
	for offset := 0; offset+4 <= len(buf); {
		messageLength := int(binary.LittleEndian.Uint16(buf[offset:]))
		if messageLength < 4 || offset+messageLength > len(buf) {
			break
		}
		record := buf[offset : offset+messageLength]
		if record[3] == rtmIfinfo2 && len(record) >= ifDataTxOffset+8 {
			if name := interfaceName(record); name != "" {
				counters[name] = netPair{
					rx: binary.LittleEndian.Uint64(record[ifDataRxOffset:]),
					tx: binary.LittleEndian.Uint64(record[ifDataTxOffset:]),
				}
			}
		}
		offset += messageLength
	}
	if len(counters) == 0 {
		return nil
	}
	return counters
}

// interfaceName reads the AF_LINK sockaddr that follows the header.
func interfaceName(record []byte) string {
	if len(record) < ifmHeaderSize+2 {
		return ""
	}
	link := record[ifmHeaderSize:]
	if link[1] != 0x12 { // AF_LINK
		return ""
	}
	nameLength := int(link[5])
	if nameLength == 0 || ifmHeaderSize+sockaddrDLExt+nameLength > len(record) {
		return ""
	}
	return string(record[ifmHeaderSize+sockaddrDLExt : ifmHeaderSize+sockaddrDLExt+nameLength])
}

// sysctlRaw calls sysctl(3) with a numeric MIB and returns the buffer.
func sysctlRaw(mib []int32) ([]byte, error) {
	name := unsafe.Pointer(&mib[0])
	nameLength := uintptr(len(mib))

	var size uintptr
	if _, _, errno := syscall.Syscall6(syscall.SYS___SYSCTL,
		uintptr(name), nameLength, 0, uintptr(unsafe.Pointer(&size)), 0, 0); errno != 0 {
		return nil, errno
	}
	if size == 0 {
		return nil, nil
	}
	buf := make([]byte, size)
	if _, _, errno := syscall.Syscall6(syscall.SYS___SYSCTL,
		uintptr(name), nameLength, uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&size)), 0, 0); errno != 0 {
		return nil, errno
	}
	return buf[:size], nil
}
