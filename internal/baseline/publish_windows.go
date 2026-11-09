package baseline

import (
	"fmt"
	"os"
	"runtime"
	"syscall"
	"unsafe"
)

var (
	reopenFile            = syscall.NewLazyDLL("kernel32.dll").NewProc("ReOpenFile")
	ntSetInformationFile  = syscall.NewLazyDLL("ntdll.dll").NewProc("NtSetInformationFile")
	rtlNtStatusToDosError = syscall.NewLazyDLL("ntdll.dll").NewProc("RtlNtStatusToDosError")
)

// publishNewRecord はディレクトリのハンドルを基準に、既存先を置換せず確定する。
// ハードリンクを必要とせず、既存先の置換をWindowsに拒否させる。
func publishNewRecord(dir *os.Root, file *os.File, temporary, name string) error {
	parent, err := dir.Open(".")
	if err != nil {
		return err
	}
	defer parent.Close()
	// パスを開き直さず、同じ一時ファイルに名前変更用のDELETE権限を取得する。
	const deleteAccess = 0x00010000
	handle, _, callErr := reopenFile.Call(file.Fd(), deleteAccess,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE, 0)
	runtime.KeepAlive(file)
	if syscall.Handle(handle) == syscall.InvalidHandle {
		return fmt.Errorf("ReOpenFile: %w", callErr)
	}
	defer syscall.CloseHandle(syscall.Handle(handle))
	name16, err := syscall.UTF16FromString(name)
	if err != nil {
		return err
	}
	// FILE_RENAME_INFORMATIONの可変長のFileNameを、対象OSのアラインメントで配置する。
	type renameInfo struct {
		ReplaceIfExists uint32
		RootDirectory   syscall.Handle
		FileNameLength  uint32
		FileName        [1]uint16
	}
	offset := unsafe.Offsetof(renameInfo{}.FileName)
	buffer := make([]byte, int(offset)+len(name16)*2)
	info := (*renameInfo)(unsafe.Pointer(&buffer[0]))
	info.RootDirectory = syscall.Handle(parent.Fd())
	info.FileNameLength = uint32((len(name16) - 1) * 2)
	copy(unsafe.Slice(&info.FileName[0], len(name16)), name16)
	const fileRenameInformation = 10
	var ioStatus [2]uintptr
	status, _, _ := ntSetInformationFile.Call(handle, uintptr(unsafe.Pointer(&ioStatus[0])),
		uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)), fileRenameInformation)
	runtime.KeepAlive(parent)
	runtime.KeepAlive(buffer)
	if int32(status) < 0 {
		errno, _, _ := rtlNtStatusToDosError.Call(status)
		return fmt.Errorf("NtSetInformationFile: %w", syscall.Errno(errno))
	}
	return nil
}
