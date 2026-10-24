package gitapi

import "golang.org/x/sys/unix"

func publishCheckout(staging, target string) error {
	return unix.Renameat2(unix.AT_FDCWD, staging, unix.AT_FDCWD, target, unix.RENAME_NOREPLACE)
}
