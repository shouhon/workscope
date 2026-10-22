package baseline

import (
	"path/filepath"
	"syscall"
)

func actualName(path string) (string, error) {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	buffer := make([]uint16, 256)
	for {
		n, err := syscall.GetLongPathName(name, &buffer[0], uint32(len(buffer)))
		if err != nil {
			return "", err
		}
		if n < uint32(len(buffer)) {
			return filepath.Base(syscall.UTF16ToString(buffer[:n])), nil
		}
		buffer = make([]uint16, n+1)
	}
}
