// Package privatefile reads owner-only deployment inputs.
package privatefile

import (
	"errors"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

var errUnavailable = errors.New("private deployment file unavailable")

// Read checks the opened file rather than the path. This allows projected
// secret mounts to point to owner-only regular files using symlinks.
func Read(path string, min, max int64) ([]byte, error) {
	file, err := os.OpenFile(path, os.O_RDONLY|unix.O_NONBLOCK|unix.O_NOCTTY, 0)
	if err != nil {
		return nil, errUnavailable
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil || !privateRegular(info) || info.Size() < min || info.Size() > max {
		return nil, errUnavailable
	}
	raw, err := io.ReadAll(io.LimitReader(file, max+1))
	if err != nil || int64(len(raw)) != info.Size() {
		clear(raw)
		return nil, errUnavailable
	}
	latest, err := file.Stat()
	if err != nil || !privateRegular(latest) || latest.Size() != info.Size() || !latest.ModTime().Equal(info.ModTime()) {
		clear(raw)
		return nil, errUnavailable
	}
	return raw, nil
}

func privateRegular(info os.FileInfo) bool {
	return info.Mode().IsRegular() && info.Mode().Perm()&0077 == 0
}
