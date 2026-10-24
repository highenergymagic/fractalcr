package appliance

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"fractalcr/internal/config"
	"golang.org/x/sys/unix"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
)

// Resolve parent directories through os.Root before the no-replace rename,
// avoiding both path escape and a check-then-overwrite race.
func renameFile(root *os.Root, source, destination string) error {
	src, err := root.OpenRoot(path.Dir(source))
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := root.OpenRoot(path.Dir(destination))
	if err != nil {
		return err
	}
	defer dst.Close()
	a, err := src.Open(".")
	if err != nil {
		return err
	}
	defer a.Close()
	b, err := dst.Open(".")
	if err != nil {
		return err
	}
	defer b.Close()
	return unix.Renameat2(int(a.Fd()), path.Base(source), int(b.Fd()), path.Base(destination), unix.RENAME_NOREPLACE)
}

func copyFile(w []string, c config.Config) (string, error) {
	return copyFileOverwrite(w, c, false)
}

func copyFileOverwrite(w []string, c config.Config, overwrite bool) (string, error) {
	if len(w) != 3 {
		return "", fmt.Errorf("Source and destination required")
	}
	var input io.Reader
	if data, ok, err := configurationExport(w[1], c); ok {
		if err != nil {
			return "", err
		}
		input = bytes.NewReader(data)
	} else {
		source := w[1]
		if source == "startup-config" {
			source = "nvram:startup-config.json"
		}
		v, name, err := resolveFile(source)
		if err != nil {
			return "", err
		}
		root, err := os.OpenRoot(v.Root)
		if err != nil {
			return "", fileError(source, err)
		}
		defer root.Close()
		f, err := root.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK, 0)
		if err != nil {
			return "", fileError(source, err)
		}
		defer f.Close()
		st, err := f.Stat()
		if err != nil || !st.Mode().IsRegular() {
			return "", fmt.Errorf("Source must be a regular file")
		}
		if st.Size() > 512*1024*1024 {
			return "", fmt.Errorf("File exceeds transfer limit")
		}
		input = f
	}
	v, name, err := resolveFile(w[2])
	if err != nil {
		return "", err
	}
	if protectedFile(v, name) {
		return "", fmt.Errorf("Destination is a protected system object")
	}
	root, err := os.OpenRoot(v.Root)
	if err != nil {
		return "", fileError(w[2], err)
	}
	defer root.Close()
	nonce := make([]byte, 12)
	if _, err = rand.Read(nonce); err != nil {
		return "", fmt.Errorf("Cannot allocate transfer")
	}
	temporary := path.Join(path.Dir(name), fmt.Sprintf(".transfer-%x", nonce))
	f, err := root.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", fileError(w[2], err)
	}
	defer root.Remove(temporary)
	count, err := io.Copy(f, io.LimitReader(input, 512*1024*1024+1))
	if err == nil && count > 512*1024*1024 {
		err = fmt.Errorf("transfer too large")
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return "", fileError(w[2], err)
	}
	if overwrite {
		err = root.Rename(temporary, name)
	} else {
		err = renameFile(root, temporary, name)
	}
	if err != nil {
		return "", fileError(w[2], err)
	}
	if parent, e := root.Open(path.Dir(name)); e == nil {
		_ = parent.Sync()
		parent.Close()
	}
	return fmt.Sprintf("%d bytes copied", count), nil
}

type applianceFS struct {
	Name, Root string
	ReadOnly   bool
}

var fileSystems = discoverFileSystems

func filesystemNames() []string {
	var names []string
	for _, v := range fileSystems() {
		names = append(names, v.Name+":")
		if v.Name == "bootflash" {
			names = append(names, "flash:")
		}
	}
	sort.Strings(names)
	return names
}

func deleteURL(w []string) string {
	for _, word := range w[1:] {
		if word != "/force" && word != "/recursive" {
			return word
		}
	}
	return ""
}

// Only named appliance volumes are addressable. os.Root confines every operation
// even if a container concurrently replaces a path component with a symlink.
func discoverFileSystems() []applianceFS {
	result := []applianceFS{}
	if dataOnline() == nil {
		if os.MkdirAll("/datastore/data0/files", 0700) == nil {
			result = append(result, applianceFS{"data0", "/datastore/data0/files", false})
		}
		result = append(result, applianceFS{"nvram", "/datastore/data0/config", true})
	}
	entries, _ := os.ReadDir("/sys/class/block")
	usbIndex := 0
	haveBoot := false
	for _, e := range entries {
		device := "/dev/" + e.Name()
		physical, _ := filepath.EvalSymlinks("/sys/class/block/" + e.Name())
		usb := strings.Contains(physical, "/usb")
		f, err := os.Open(device)
		if err != nil {
			continue
		}
		sector := make([]byte, 512)
		_, err = io.ReadFull(f, sector)
		f.Close()
		if err != nil || sector[510] != 0x55 || sector[511] != 0xaa {
			continue
		}
		offset := 43
		if string(sector[82:90]) == "FAT32   " {
			offset = 71
		} else if string(sector[54:62]) != "FAT16   " {
			continue
		}
		label := strings.TrimSpace(string(sector[offset : offset+11]))
		name := ""
		if label == "FRFLASH" && !haveBoot {
			name = "bootflash"
			haveBoot = true
		} else if usb && usbIndex < 4 {
			name = fmt.Sprintf("usbflash%d", usbIndex)
			usbIndex++
		} else {
			continue
		}
		root := "/run/filesystems/" + name
		if os.MkdirAll(root, 0700) != nil {
			continue
		}
		flags := uintptr(syscall.MS_NOSUID | syscall.MS_NODEV | syscall.MS_NOEXEC)
		err = syscall.Mount(device, root, "vfat", flags, "utf8=0,umask=0077")
		if err != nil && !errors.Is(err, syscall.EBUSY) {
			continue
		}
		// Do not accept an unrelated pre-existing mount at the same pathname.
		mounts, _ := os.ReadFile("/proc/mounts")
		matched := false
		for _, line := range strings.Split(string(mounts), "\n") {
			v := strings.Fields(line)
			if len(v) > 2 && v[0] == device && v[1] == root && v[2] == "vfat" {
				matched = true
			}
		}
		if matched {
			result = append(result, applianceFS{name, root, false})
		}
	}
	return result
}

func resolveFile(url string) (applianceFS, string, error) {
	prefix, name, ok := strings.Cut(url, ":")
	if !ok {
		prefix = "bootflash"
		name = url
	}
	if prefix == "flash" {
		prefix = "bootflash"
	}
	if strings.ContainsAny(name, "\\\x00\r\n") {
		return applianceFS{}, "", fmt.Errorf("Invalid filename")
	}
	for _, part := range strings.Split(name, "/") {
		if part == ".." {
			return applianceFS{}, "", fmt.Errorf("Invalid filename")
		}
	}
	name = path.Clean("/" + name)
	name = strings.TrimPrefix(name, "/")
	if name == "" {
		name = "."
	}
	for _, v := range fileSystems() {
		if v.Name == prefix {
			return v, name, nil
		}
	}
	return applianceFS{}, "", fmt.Errorf("Filesystem %s: is unavailable", prefix)
}
func fileError(url string, err error) error {
	reason := "Operation failed"
	switch {
	case errors.Is(err, fs.ErrNotExist):
		reason = "No such file"
	case errors.Is(err, fs.ErrExist):
		reason = "File already exists"
	case errors.Is(err, fs.ErrPermission):
		reason = "Permission denied"
	case errors.Is(err, syscall.ENOSPC):
		reason = "Not enough space"
	case errors.Is(err, syscall.ENOTEMPTY):
		reason = "Directory is not empty"
	}
	return fmt.Errorf("%s: %s", url, reason)
}
func protectedFile(v applianceFS, name string) bool {
	if v.ReadOnly || name == "." {
		return true
	}
	if v.Name == "bootflash" {
		first := strings.ToLower(strings.Split(name, "/")[0])
		return first == "efi" || strings.HasPrefix(first, "rommon") || strings.HasPrefix(first, "env")
	}
	return false
}
func fileOperation(w []string) (string, error) {
	if len(w) == 0 {
		return "", fmt.Errorf("File operation required")
	}
	if w[0] == "dir" {
		return directoryOperation(w[1:])
	}
	recursive := false
	if w[0] == "delete" {
		args := []string{"delete"}
		for _, arg := range w[1:] {
			switch arg {
			case "/force":
			case "/recursive":
				recursive = true
			default:
				args = append(args, arg)
			}
		}
		w = args
		if len(w) != 2 {
			return "", fmt.Errorf("Filename required")
		}
	}
	if strings.Join(w, " ") == "show file systems" {
		var b strings.Builder
		b.WriteString("File Systems:\n\n         Size(b)       Free(b)      Type  Flags  Prefixes\n")
		for _, v := range fileSystems() {
			var st syscall.Statfs_t
			if syscall.Statfs(v.Root, &st) != nil {
				continue
			}
			flags := "rw"
			if v.ReadOnly {
				flags = "ro"
			}
			marker, kind, prefixes := " ", "disk", v.Name+":"
			switch v.Name {
			case "bootflash":
				marker, kind, prefixes = "*", "flash", "bootflash: flash:"
			case "nvram":
				kind = "nvram"
			}
			fmt.Fprintf(&b, "%s %14d %13d  %8s  %-5s  %s\n", marker, st.Blocks*uint64(st.Bsize), st.Bavail*uint64(st.Bsize), kind, flags, prefixes)
		}
		return b.String(), nil
	}
	url := "bootflash:"
	if len(w) > 1 {
		url = w[1]
	}
	v, name, err := resolveFile(url)
	if err != nil {
		return "", err
	}
	root, err := os.OpenRoot(v.Root)
	if err != nil {
		return "", fileError(url, err)
	}
	defer root.Close()
	if protectedFile(v, name) {
		return "", fmt.Errorf("%s: Protected system object", url)
	}
	switch w[0] {
	case "delete", "rmdir":
		if recursive {
			// The descriptor-rooted operation cannot escape through symlinks.
			if _, err = root.Lstat(name); err == nil {
				err = root.RemoveAll(name)
			}
		} else {
			err = root.Remove(name)
		}
	case "mkdir":
		err = root.Mkdir(name, 0700)
	case "move", "rename":
		if len(w) != 3 {
			return "", fmt.Errorf("Destination filename required")
		}
		dest, target, e := resolveFile(w[2])
		if e != nil {
			return "", e
		}
		if dest.Name != v.Name {
			return "", fmt.Errorf("Move requires source and destination on the same filesystem; use copy then delete")
		}
		if protectedFile(dest, target) {
			return "", fmt.Errorf("Destination is a protected system object")
		}
		if _, e = root.Lstat(target); e == nil {
			return "", fmt.Errorf("Destination already exists")
		} else if !errors.Is(e, fs.ErrNotExist) {
			return "", fileError(w[2], e)
		}
		err = renameFile(root, name, target)
	default:
		return "", fmt.Errorf("Unknown file operation")
	}
	if err != nil {
		return "", fileError(url, err)
	}
	if f, e := root.Open("."); e == nil {
		_ = f.Sync()
		f.Close()
	}
	return "", nil
}
