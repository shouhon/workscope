//go:build !windows

package baseline

import "path/filepath"

func actualName(path string) (string, error) { return filepath.Base(path), nil }
