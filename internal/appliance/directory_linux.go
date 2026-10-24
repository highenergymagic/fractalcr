package appliance

import (
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"syscall"
)

// IOS directory wildcards match a filename prefix; text after '*' is ignored.
// Walk only real directories, never symlinks, including on recursive listings.
func directoryOperation(args []string) (string, error) {
	recursive, allFS := false, false
	url := "bootflash:"
	for _, arg := range args {
		switch arg {
		case "/all": // These filesystems unlink immediately; no deleted entries exist.
		case "/recursive":
			recursive = true
		case "all-filesystems":
			allFS = true
		default:
			url = arg
		}
	}
	if allFS {
		var outputs []string
		for _, v := range fileSystems() {
			flags := []string{v.Name + ":"}
			if recursive {
				flags = append(flags, "/recursive")
			}
			out, err := directoryOperation(flags)
			if err != nil {
				outputs = append(outputs, "% "+err.Error())
			} else {
				outputs = append(outputs, out)
			}
		}
		return strings.Join(outputs, "\n"), nil
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
	var b strings.Builder
	var walk func(string, string, int) error
	count := 0
	walk = func(directory, pattern string, depth int) error {
		if depth > 64 || count > 100000 {
			return fmt.Errorf("Directory listing limit exceeded")
		}
		f, err := root.OpenFile(directory, os.O_RDONLY|syscall.O_NONBLOCK, 0)
		if err != nil {
			return err
		}
		entries, err := f.Readdir(100001 - count)
		f.Close()
		if err != nil && err != io.EOF {
			return err
		}
		count += len(entries)
		if count > 100000 {
			return fmt.Errorf("Directory listing limit exceeded")
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		shown := directory
		if shown == "." {
			shown = ""
		}
		fmt.Fprintf(&b, "Directory of %s:/%s\n\n", v.Name, displayText(shown))
		for i, entry := range entries {
			if pattern != "" && !strings.HasPrefix(entry.Name(), pattern) {
				continue
			}
			mode := "-rw-"
			if v.ReadOnly {
				mode = "-r--"
			}
			if entry.IsDir() {
				mode = "drw-"
			}
			fmt.Fprintf(&b, "%5d  %s %12d  %s  %s\n", i+1, mode, entry.Size(), entry.ModTime().Format("Jan 2 2006 15:04:05 -07:00"), displayText(entry.Name()))
		}
		b.WriteByte('\n')
		if recursive {
			for _, entry := range entries {
				if entry.IsDir() && (pattern == "" || strings.HasPrefix(entry.Name(), pattern)) {
					if err := walk(path.Join(directory, entry.Name()), "", depth+1); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}
	if index := strings.IndexByte(path.Base(name), '*'); index >= 0 {
		err = walk(path.Dir(name), path.Base(name)[:index], 0)
	} else {
		st, e := root.Stat(name)
		if e != nil {
			return "", fileError(url, e)
		}
		if st.IsDir() {
			err = walk(name, "", 0)
		} else {
			fmt.Fprintf(&b, "Directory of %s:/%s\n\n%5d  -rw- %12d  %s  %s\n\n", v.Name, displayText(name), 1, st.Size(), st.ModTime().Format("Jan 2 2006 15:04:05 -07:00"), displayText(path.Base(name)))
		}
	}
	if err != nil {
		return "", fileError(url, err)
	}
	f, err := root.Open(".")
	if err == nil {
		defer f.Close()
		var st syscall.Statfs_t
		if syscall.Fstatfs(int(f.Fd()), &st) == nil {
			fmt.Fprintf(&b, "%d bytes total (%d bytes free)\n", st.Blocks*uint64(st.Bsize), st.Bavail*uint64(st.Bsize))
		}
	}
	return b.String(), nil
}
