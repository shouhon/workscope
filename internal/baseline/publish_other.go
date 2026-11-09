//go:build !windows

package baseline

import "os"

func publishNewRecord(dir *os.Root, file *os.File, temporary, name string) error {
	return dir.Link(temporary, name)
}
