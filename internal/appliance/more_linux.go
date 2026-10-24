package appliance

import (
	"fmt"
	"io"
	"os"
	"syscall"

	"fractalcr/internal/filedisplay"
)

func moreFile(words []string) (string, error) {
	format := ""
	if len(words) == 3 {
		format = words[1]
	} else if len(words) != 2 {
		return "", fmt.Errorf("Filename required")
	}
	url := words[len(words)-1]
	v, name, err := resolveFile(url)
	if err != nil {
		return "", err
	}
	root, err := os.OpenRoot(v.Root)
	if err != nil {
		return "", fileError(url, err)
	}
	defer root.Close()
	f, err := root.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return "", fileError(url, err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return "", fileError(url, err)
	}
	if !st.Mode().IsRegular() {
		return "", fmt.Errorf("%s: Not a regular file", url)
	}
	if st.Size() > filedisplay.Limit {
		return "", fmt.Errorf("File exceeds display limit")
	}
	data, err := io.ReadAll(io.LimitReader(f, filedisplay.Limit+1))
	if err != nil {
		return "", fileError(url, err)
	}
	return filedisplay.Format(data, format)
}
