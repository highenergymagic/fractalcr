package appliance

import (
	"fmt"
	"os"
	"path"
	"strings"

	"fractalcr/internal/config"
)

func (s *loginSession) pwd() string {
	if s.workingDirectory == "" {
		return "bootflash:/"
	}
	return s.workingDirectory
}

// Resolve appliance URLs, not host paths. Parent traversal is normalized inside
// the selected volume; os.Root still enforces containment at every operation.
func (s *loginSession) fileURL(name string) string {
	prefix, directory, _ := strings.Cut(s.pwd(), ":")
	if p, n, ok := strings.Cut(name, ":"); ok {
		return p + ":" + path.Clean("/"+n)
	}
	if strings.HasPrefix(name, "/") {
		directory = "/"
	}
	return prefix + ":" + path.Join(directory, name)
}

func (s *loginSession) fileWords(words []string) []string {
	if len(words) == 0 {
		return words
	}
	switch words[0] {
	case "dir", "delete", "move", "rename", "mkdir", "rmdir", "copy", "more":
	default:
		return words
	}
	w := append([]string(nil), words...)
	target := false
	for i := 1; i < len(w); i++ {
		switch w[i] {
		case "/force", "/recursive", "/all", "/ascii", "/binary", "/compressed", "/ebcdic":
			continue
		case "all-filesystems":
			target = true
			continue
		case "running-config", "startup-config", "candidate-config":
			continue
		}
		target = true
		bareVolume := strings.HasSuffix(w[i], ":")
		w[i] = s.fileURL(w[i])
		// copy uses a bare volume to request an interactive filename.
		if bareVolume && w[0] == "copy" {
			w[i] = strings.TrimSuffix(w[i], "/")
		}
	}
	if w[0] == "dir" && !target {
		w = append(w, s.pwd())
	}
	return w
}

func (s *loginSession) operations(w []string, c config.Config) (string, error) {
	if len(w) == 1 && w[0] == "pwd" {
		return s.pwd(), nil
	}
	if len(w) > 0 && w[0] == "cd" {
		prefix, _, _ := strings.Cut(s.pwd(), ":")
		url := prefix + ":/"
		if len(w) == 2 {
			url = s.fileURL(w[1])
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
		st, err := root.Stat(name)
		if err != nil {
			return "", fileError(url, err)
		}
		if !st.IsDir() {
			return "", fmt.Errorf("%s: Not a directory", url)
		}
		s.workingDirectory = v.Name + ":" + path.Clean("/"+name)
		return "", nil
	}
	return Operations(s.fileWords(w), c)
}
