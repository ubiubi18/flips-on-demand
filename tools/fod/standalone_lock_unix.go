//go:build darwin || linux || freebsd

package main

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// The kernel releases this advisory lock even if the app is killed. Keep the
// file in place so a second start cannot race an unlink of a locked inode.
func acquireStandaloneLock(path string) (func(), error) {
	fd, err := syscall.Open(path, syscall.O_RDWR|syscall.O_CREAT|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0600)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	closeWithError := func(err error) (func(), error) {
		_ = file.Close()
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		return closeWithError(err)
	}
	owner, ok := info.Sys().(*syscall.Stat_t)
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || !ok || owner.Uid != uint32(os.Getuid()) {
		return closeWithError(errors.New("lock must be an owner-only regular file"))
	}
	if err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return closeWithError(err)
	}
	if err := file.Truncate(0); err != nil {
		return closeWithError(err)
	}
	if _, err := file.Seek(0, 0); err != nil {
		return closeWithError(err)
	}
	if _, err := fmt.Fprintf(file, "pid=%d\n", os.Getpid()); err != nil {
		return closeWithError(err)
	}
	if err := file.Sync(); err != nil {
		return closeWithError(err)
	}
	return func() { _ = syscall.Flock(fd, syscall.LOCK_UN); _ = file.Close() }, nil
}
