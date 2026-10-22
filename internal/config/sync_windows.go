//go:build windows

package config

// Directory fsync is not supported through os.File.Sync on Windows.
// Windows is the configuration simulator; the appliance runs Linux.
func syncDir(path string) error { return nil }
