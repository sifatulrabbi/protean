//go:build !unix

package diskusage

import "errors"

// ErrUnsupportedPlatform is returned where statfs is unavailable. Protean runs
// on linux and darwin; this file only keeps other platforms compiling.
var ErrUnsupportedPlatform = errors.New("host disk statistics are not supported on this platform")

func hostDisk(string) (freeBytes, totalBytes int64, err error) {
	return 0, 0, ErrUnsupportedPlatform
}
