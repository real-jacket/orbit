//go:build darwin

package scanner

import (
	"encoding/binary"
	"errors"
	"io/fs"
	"sync"
	"syscall"
	"unsafe"
)

// getattrlistbulk(2) returns the names, types and allocated sizes of many
// directory entries per call, so measuring a file costs no lstat of its own.
const sysGetattrlistbulk = 461

type attrList struct {
	bitmapCount uint16
	reserved    uint16
	common      uint32
	volume      uint32
	dir         uint32
	file        uint32
	fork        uint32
}

// Attribute bits, in the order the kernel packs them into each record.
const (
	attrReturned  = 0x80000000 // ATTR_CMN_RETURNED_ATTRS; first in every record
	attrName      = 0x00000001
	attrDevID     = 0x00000002
	attrObjType   = 0x00000008
	attrModTime   = 0x00000400
	attrFileID    = 0x02000000
	attrLinkCount = 0x00000001 // file attribute
	attrAllocSize = 0x00000004 // file attribute
)

var bulkBuffers = sync.Pool{New: func() any { b := make([]byte, 128<<10); return &b }}

// listDirItems lists dir with getattrlistbulk, falling back to ReadDir on a
// filesystem that does not support it.
func listDirItems(dir string) ([]dirItem, error) {
	items, err := listDirBulk(dir)
	if errors.Is(err, syscall.ENOTSUP) || errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.ENOSYS) {
		return listDirStd(dir)
	}
	return items, err
}

func listDirBulk(dir string) ([]dirItem, error) {
	var fd int
	var err error
	for { // Go's syscall.Open, unlike os.Open, does not retry an interrupted call
		if fd, err = syscall.Open(dir, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC, 0); err != syscall.EINTR {
			break
		}
	}
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: dir, Err: err}
	}
	defer syscall.Close(fd)
	bufp := bulkBuffers.Get().(*[]byte)
	defer bulkBuffers.Put(bufp)
	buf := *bufp
	al := attrList{bitmapCount: 5, common: attrReturned | attrName | attrDevID | attrObjType | attrModTime | attrFileID, file: attrLinkCount | attrAllocSize}
	var items []dirItem
	for {
		n, _, errno := syscall.Syscall6(sysGetattrlistbulk, uintptr(fd), uintptr(unsafe.Pointer(&al)), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 0, 0)
		if errno == syscall.EINTR {
			continue
		}
		if errno != 0 {
			return items, &fs.PathError{Op: "getattrlistbulk", Path: dir, Err: errno}
		}
		if n == 0 {
			return items, nil
		}
		off := 0
		for i := 0; i < int(n); i++ {
			item, length, ok := parseBulkRecord(buf[off:])
			if !ok {
				return items, &fs.PathError{Op: "getattrlistbulk", Path: dir, Err: syscall.EIO}
			}
			items = append(items, item)
			off += length
		}
	}
}

// parseBulkRecord decodes one record: a length, the attribute sets actually
// returned, then each returned attribute packed in bit order.
func parseBulkRecord(rec []byte) (item dirItem, length int, ok bool) {
	if len(rec) < 24 {
		return item, 0, false
	}
	length = int(binary.LittleEndian.Uint32(rec))
	if length < 24 || length > len(rec) {
		return item, 0, false
	}
	rec = rec[:length]
	common := binary.LittleEndian.Uint32(rec[4:])
	file := binary.LittleEndian.Uint32(rec[16:])
	p := 24
	take := func(n int) []byte {
		if p+n > len(rec) {
			ok = false
			return make([]byte, n)
		}
		b := rec[p : p+n]
		p += n
		return b
	}
	ok = true
	var objType uint32
	if common&attrName != 0 {
		ref := p
		b := take(8)
		off, size := int(int32(binary.LittleEndian.Uint32(b))), int(binary.LittleEndian.Uint32(b[4:]))
		if start := ref + off; size > 0 && start >= 0 && start+size <= len(rec) {
			item.name = string(rec[start : start+size-1]) // drop the NUL
		} else {
			ok = false
		}
	}
	if common&attrDevID != 0 {
		item.dev = uint64(binary.LittleEndian.Uint32(take(4)))
	}
	if common&attrObjType != 0 {
		objType = binary.LittleEndian.Uint32(take(4))
	}
	if common&attrModTime != 0 {
		item.mtime = int64(binary.LittleEndian.Uint64(take(16)))
	}
	if common&attrFileID != 0 {
		item.ino = binary.LittleEndian.Uint64(take(8))
	}
	if file&attrLinkCount != 0 {
		item.nlink = binary.LittleEndian.Uint32(take(4))
	}
	if file&attrAllocSize != 0 {
		item.bytes = max(0, int64(binary.LittleEndian.Uint64(take(8))))
	}
	switch objType {
	case 1: // VREG
		item.kind = itemFile
	case 2: // VDIR
		item.kind = itemDir
	}
	if item.kind != itemFile {
		item.bytes, item.mtime = 0, 0
	}
	return item, length, ok && item.name != ""
}
