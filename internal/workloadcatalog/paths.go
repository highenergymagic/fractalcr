package workloadcatalog

import (
	"encoding/csv"
	"fmt"
	"path"
	"strings"
)

// FilePath resolves the appliance's writable file volume. It is a namespace
// translation, not an authorization boundary; workload access is privileged.
func FilePath(value string) (string, error) {
	if !strings.HasPrefix(value, "data0:") {
		return value, nil
	}
	name := strings.TrimPrefix(value, "data0:")
	if strings.ContainsAny(name, "\x00\r\n") {
		return "", fmt.Errorf("Invalid data0: filename")
	}
	name = strings.TrimPrefix(name, "/")
	clean := path.Clean(name)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("Filename escapes data0:")
	}
	return path.Join("/datastore/data0/files", clean), nil
}

// PathArguments walks the linked command graph. Only declared file operands
// are translated; arbitrary positional arguments and flag values are opaque.
func PathArguments(root Command, args []string) ([]string, error) {
	out := append([]string(nil), args...)
	cmd := &root
	var names []string
	position := 0
	flags := true
	for i := 0; i < len(out); i++ {
		word := out[i]
		if flags && word == "--" {
			flags = false
			continue
		}
		if flags && strings.HasPrefix(word, "-") && word != "-" {
			key, value, assigned := strings.Cut(word, "=")
			var flag *Flag
			attachedPrefix := ""
			for j := range cmd.Flags {
				f := &cmd.Flags[j]
				if key == "--"+f.Name || f.Short != "" && key == "-"+f.Short {
					flag = f
					break
				}
			}
			if flag == nil && !strings.HasPrefix(word, "--") {
				for n := 1; n < len(key); n++ {
					var part *Flag
					for j := range cmd.Flags {
						if cmd.Flags[j].Short == key[n:n+1] {
							part = &cmd.Flags[j]
							break
						}
					}
					if part == nil {
						break
					}
					if part.Boolean {
						continue
					}
					flag = part
					if n+1 < len(key) {
						attachedPrefix = key[:n+1]
						if assigned {
							value = key[n+1:] + "=" + value
							assigned = false
						} else {
							value = key[n+1:]
						}
					}
					break
				}
			}
			if flag == nil || flag.Boolean {
				continue
			}
			if !assigned && attachedPrefix == "" {
				i++
				if i == len(out) {
					break
				}
				value = out[i]
			}
			mapped, err := flagPath(names, flag.Name, value)
			if err != nil {
				return nil, err
			}
			if assigned {
				out[i] = key + "=" + mapped
			} else if attachedPrefix != "" {
				out[i] = attachedPrefix + mapped
			} else {
				out[i] = mapped
			}
			continue
		}
		if len(cmd.Children) > 0 && position == 0 {
			var child *Command
			for j := range cmd.Children {
				c := &cmd.Children[j]
				for _, n := range append([]string{c.Name}, c.Aliases...) {
					if n == word {
						child = c
						break
					}
				}
				if child != nil {
					break
				}
			}
			if child != nil {
				cmd = child
				names = append(names, child.Name)
				continue
			}
		}
		if positionalFile(names, position) {
			mapped, err := FilePath(word)
			if err != nil {
				return nil, err
			}
			out[i] = mapped
		}
		position++
		if !cmd.Interspersed {
			flags = false
		}
	}
	return out, nil
}

func positionalFile(names []string, index int) bool {
	if len(names) == 0 {
		return false
	}
	last := names[len(names)-1]
	full := strings.Join(names, " ")
	if full == "context import" || full == "context export" || full == "plugin create" {
		return index == 1
	}
	if full == "buildx dap attach" || full == "compose bridge transformations create" || full == "debug dump-llb" {
		return index == 0
	}
	switch last {
	case "build", "import":
		return index == 0
	case "cp":
		return index < 2
	case "create":
		return len(names) > 1 && (names[len(names)-2] == "secret" || names[len(names)-2] == "config") && index == 1
	}
	return false
}

func flagPath(commands []string, name, value string) (string, error) {
	switch name {
	case "file", "compose-file", "env-file", "env-from-file", "label-file", "project-directory", "project-dir", "templates", "input", "iidfile", "cidfile", "metadata-file", "source-policy-file", "ref-file", "trace", "cache-metrics-file", "tlscacert", "tlscert", "tlskey", "tlsdir", "config", "buildkitd-config", "filename", "ca-cert", "ca-key", "lock-image-digests", "checkpoint-dir", "root":
		return FilePath(value)
	case "workdir":
		if len(commands) > 0 && commands[0] == "compose" {
			return FilePath(value)
		}
	case "volume":
		if strings.HasPrefix(value, "data0:") {
			source, suffix, ok := strings.Cut(strings.TrimPrefix(value, "data0:"), ":")
			if ok {
				mapped, err := FilePath("data0:" + source)
				return mapped + ":" + suffix, err
			}
		}
	case "local", "oci-layout", "build-context":
		key, v, ok := strings.Cut(value, "=")
		if ok {
			p, err := FilePath(v)
			return key + "=" + p, err
		}
	case "ssh":
		id, entries, ok := strings.Cut(value, "=")
		if ok {
			fields := strings.Split(entries, ",")
			for i, field := range fields {
				mapped, err := FilePath(field)
				if err != nil {
					return "", err
				}
				fields[i] = mapped
			}
			return id + "=" + strings.Join(fields, ","), nil
		}
	case "output", "cache-from", "cache-to", "export-cache", "import-cache", "secret", "mount", "policy", "docker", "registry-auth-tlscontext":
		// "secret" on a service is an object reference, not a local file.
		if name == "secret" && len(commands) > 0 && commands[0] == "service" {
			return value, nil
		}
		if strings.HasPrefix(value, "data0:") && name == "output" {
			return FilePath(value)
		}
		fields, err := csv.NewReader(strings.NewReader(value)).Read()
		if err != nil {
			return value, nil
		} // The linked client's parser reports malformed records.
		for _, field := range fields {
			if name == "secret" && field == "type=env" || name == "mount" && field == "type=volume" {
				return value, nil
			}
		}
		changed := false
		for i, field := range fields {
			key, v, ok := strings.Cut(field, "=")
			certificate := (name == "docker" || name == "registry-auth-tlscontext") && (key == "ca" || key == "cert" || key == "key")
			if !ok || (key != "src" && key != "source" && key != "dest" && key != "destination" && !(name == "policy" && key == "filename") && !certificate) || !strings.HasPrefix(v, "data0:") {
				continue
			}
			if name == "mount" && key != "src" && key != "source" {
				continue
			}
			mapped, err := FilePath(v)
			if err != nil {
				return "", err
			}
			fields[i] = key + "=" + mapped
			changed = true
		}
		if changed {
			var b strings.Builder
			w := csv.NewWriter(&b)
			_ = w.Write(fields)
			w.Flush()
			return strings.TrimSuffix(b.String(), "\n"), w.Error()
		}
	}
	return value, nil
}
