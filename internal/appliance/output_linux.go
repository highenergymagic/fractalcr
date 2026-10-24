package appliance

import (
	"crypto/rand"
	"fmt"
	"fractalcr/internal/cli"
	"io"
	"os"
	"path"
	"strings"
	"syscall"
)

func (s *loginSession) writeOutput(filter *cli.Filter, text string) (string, error) {
	if (len(s.outputCommand) == 0 && s.cli.Privilege != 15) || (len(s.outputCommand) > 0 && !s.cli.AllowedEXEC(s.outputCommand)) {
		return "", fmt.Errorf("Authorization failed.")
	}
	if len(text) > 1024*1024 {
		return "", fmt.Errorf("Command output exceeds redirection limit")
	}
	url := s.fileURL(filter.Pattern)
	volume, name, err := resolveFile(url)
	if err != nil {
		return "", err
	}
	if protectedFile(volume, name) {
		return "", fmt.Errorf("Destination is a protected system object")
	}
	root, err := os.OpenRoot(volume.Root)
	if err != nil {
		return "", fileError(url, err)
	}
	defer root.Close()
	parent, err := root.OpenRoot(path.Dir(name))
	if err != nil {
		return "", fileError(url, err)
	}
	defer parent.Close()
	base := path.Base(name)
	var input *os.File
	if filter.Mode == "append" || filter.Mode == "tee-append" {
		input, err = parent.OpenFile(base, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
		if err != nil && !os.IsNotExist(err) {
			return "", fileError(url, err)
		}
		if input != nil {
			defer input.Close()
			st, e := input.Stat()
			if e != nil || !st.Mode().IsRegular() || st.Size() > 64*1024*1024 {
				return "", fmt.Errorf("Invalid append destination")
			}
		}
	}
	nonce := make([]byte, 12)
	if _, err = rand.Read(nonce); err != nil {
		return "", err
	}
	tmp := fmt.Sprintf(".output-%x", nonce)
	f, err := parent.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", fileError(url, err)
	}
	defer parent.Remove(tmp)
	if input != nil {
		_, err = io.Copy(f, io.LimitReader(input, 64*1024*1024+1))
	}
	if err == nil {
		_, err = io.WriteString(f, text)
	}
	if err == nil {
		err = f.Sync()
	}
	closed := f.Close()
	if err == nil {
		err = closed
	}
	if err == nil {
		err = parent.Rename(tmp, base)
	}
	if err == nil {
		if directory, e := parent.Open("."); e == nil {
			err = directory.Sync()
			directory.Close()
		} else {
			err = e
		}
	}
	if err != nil {
		return "", fileError(url, err)
	}
	if strings.HasPrefix(filter.Mode, "tee") {
		return text, nil
	}
	return "", nil
}
