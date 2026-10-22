package config

import (
	"fmt"
	"os"
)

func openConfig(path string) (*os.File, error) {
	st, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("not a regular configuration file")
	}
	return os.Open(path)
}
