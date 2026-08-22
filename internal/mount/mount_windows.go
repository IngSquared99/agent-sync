//go:build windows

package mount

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"syscall"

	"github.com/IngSquared99/agent-sync/i18n"
)

// Windows uses junctions (directory reparse points) instead of symlinks:
// creating one requires no privileges — a regular user account without
// Developer Mode is enough (§12-13).
// Junctions can only store absolute paths; after moving the project, rerun
// agsy apply to rebuild them.

const (
	_FSCTL_SET_REPARSE_POINT     = 0x900A4
	_IO_REPARSE_TAG_MOUNT_POINT  = 0xA0000003
	_FILE_FLAG_OPEN_REPARSE_PT   = 0x00200000
	_FILE_FLAG_BACKUP_SEMANTICS_ = 0x02000000
)

func linkDir(absTarget, linkPath string) error {
	abs, err := filepath.Abs(absTarget)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(linkPath, 0o755); err != nil {
		return err
	}
	// A junction is built by "create the directory first, then set the reparse
	// point". If the second step fails (e.g. non-NTFS), the just-created empty
	// directory must be removed: a leftover real directory would be classified
	// on the next apply as "not created by this tool, refusing to delete".
	// os.Remove does nothing to a non-empty directory, so it cannot remove
	// existing data by mistake.
	cleanup := func(e error) error {
		_ = os.Remove(linkPath)
		return e
	}
	p, err := syscall.UTF16PtrFromString(linkPath)
	if err != nil {
		return cleanup(err)
	}
	h, err := syscall.CreateFile(p, syscall.GENERIC_WRITE,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE, nil, syscall.OPEN_EXISTING,
		_FILE_FLAG_OPEN_REPARSE_PT|_FILE_FLAG_BACKUP_SEMANTICS_, 0)
	if err != nil {
		return cleanup(err)
	}

	// REPARSE_MOUNTPOINT_DATA_BUFFER
	subst := syscall.StringToUTF16(`\??\` + abs) // includes trailing NUL
	print16 := syscall.StringToUTF16(abs)
	substBytes := (len(subst) - 1) * 2
	printBytes := (len(print16) - 1) * 2

	buf := make([]byte, 8+8+substBytes+2+printBytes+2)
	binary.LittleEndian.PutUint32(buf[0:], _IO_REPARSE_TAG_MOUNT_POINT)
	binary.LittleEndian.PutUint16(buf[4:], uint16(len(buf)-8)) // ReparseDataLength
	// buf[6:8] Reserved
	binary.LittleEndian.PutUint16(buf[8:], 0)                     // SubstituteNameOffset
	binary.LittleEndian.PutUint16(buf[10:], uint16(substBytes))   // SubstituteNameLength
	binary.LittleEndian.PutUint16(buf[12:], uint16(substBytes+2)) // PrintNameOffset
	binary.LittleEndian.PutUint16(buf[14:], uint16(printBytes))   // PrintNameLength
	off := 16
	for _, u := range subst { // includes NUL
		binary.LittleEndian.PutUint16(buf[off:], u)
		off += 2
	}
	for _, u := range print16 {
		binary.LittleEndian.PutUint16(buf[off:], u)
		off += 2
	}

	var ret uint32
	ioErr := syscall.DeviceIoControl(h, _FSCTL_SET_REPARSE_POINT,
		&buf[0], uint32(len(buf)), nil, 0, &ret, nil)
	syscall.CloseHandle(h) // release the handle before cleanup, or the directory cannot be deleted
	if ioErr != nil {
		return cleanup(ioErr)
	}
	return nil
}

// isLink: Go's os.Lstat reports ModeSymlink for both junctions and symlinks;
// ModeIrregular is also accepted to tolerate edge cases with some reparse points.
func isLink(fi os.FileInfo, path string) bool {
	if fi.Mode()&os.ModeSymlink != 0 {
		return true
	}
	if fi.Mode()&os.ModeIrregular != 0 {
		return true
	}
	// Belt and braces: query the reparse-point attribute directly.
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return false
	}
	attrs, err := syscall.GetFileAttributes(p)
	if err != nil {
		return false
	}
	return attrs&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0
}

func platformHint() string {
	return i18n.T("\n(Windows note: agsy mounts using junctions, which should require no privileges; if this still fails, make sure the target drive is NTFS)")
}
