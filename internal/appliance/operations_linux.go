package appliance

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"fractalcr/internal/config"
)

type boundedOutput struct{ bytes.Buffer }

func (b *boundedOutput) Write(p []byte) (int, error) {
	n := len(p)
	if left := 60000 - b.Len(); left > 0 {
		if len(p) > left {
			p = p[:left]
		}
		b.Buffer.Write(p)
	}
	return n, nil
}

func run(program string, args ...string) (string, error) {
	return runTimeout(120*time.Second, program, args...)
}

func runTimeout(timeout time.Duration, program string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, program, args...)
	var out boundedOutput
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	s := strings.TrimSpace(out.String())
	if ctx.Err() != nil {
		return s, fmt.Errorf("operation timed out")
	}
	if err != nil {
		return "", fmt.Errorf("%s: %s (%v)", program, s, err)
	}
	return s, nil
}

func Operations(w []string, c config.Config) (string, error) {
	line := strings.Join(w, " ")
	switch line {
	case "show container-isolation detail":
		return run("/opt/kata/bin/kata-runtime", "--config", "/run/kata-containers/configuration.toml", "kata-env")
	case "request platform debug service-container enable":
		return debugEnable()
	case "request platform debug service-container disable":
		return debugDisable()
	case "show container-isolation capabilities":
		version, err := run("/opt/kata/bin/kata-runtime", "--version")
		if err != nil {
			return "", err
		}
		if err = kvmAvailable(); err != nil {
			return version + "\n" + err.Error(), nil
		}
		return version + "\nKVM available; Cloud Hypervisor and Mariner guest installed.", nil
	case "show interfaces", "show ip interface brief":
		return run("ip", "-brief", "address")
	case "show ip route":
		return run("ip", "route", "show")
	case "show containers":
		return run("docker", "ps", "--all", "--format", "table {{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}")
	case "show images":
		return run("docker", "images")
	case "show volumes":
		return run("docker", "volume", "ls")
	case "show storage":
		return run("btrfs", "filesystem", "show")
	case "show storage disks":
		return disks()
	case "show storage datastore data0":
		return run("btrfs", "filesystem", "usage", "/datastore/data0")
	case "show system resources":
		return resources()
	case "show capabilities":
		return "Management: transactional JSON configuration, local AAA, console/SSH EXEC, privilege authorization, persistent accounting, output filters\nContainers: Docker Engine, Compose, Buildx, BuildKit, Swarm; memory/PID/swap cgroup limits\nDatastore: data0, Btrfs single/raid1; explicit initialization of unused disks\nKata: 4.2.0 / Cloud Hypervisor / CBL-Mariner; requires KVM\nDebug service-container: privileged Debian container, enabled explicitly", nil
	}
	if len(w) == 4 && w[0] == "show" && w[1] == "container" && config.Name.MatchString(w[2]) {
		switch w[3] {
		case "detail":
			return run("docker", "inspect", w[2])
		case "logs":
			return run("docker", "logs", "--tail", "200", w[2])
		}
	}
	if len(w) == 4 && w[0] == "request" && w[1] == "container" && config.Name.MatchString(w[2]) {
		name, op := w[2], w[3]
		switch op {
		case "create":
			v, ok := c.Containers[name]
			if !ok {
				return "", fmt.Errorf("container is not configured")
			}
			if err := safeImage(v.Image); err != nil {
				return "", err
			}
			args := []string{"create", "--name", name, "--label", "com.fractalcr.managed=true", "--runtime", c.EffectiveRuntime(name)}
			if v.Restart != "" {
				args = append(args, "--restart", v.Restart)
			}
			if v.MemoryMiB > 0 {
				args = append(args, "--memory", fmt.Sprintf("%dm", v.MemoryMiB))
			}
			if v.Pids > 0 {
				args = append(args, "--pids-limit", fmt.Sprint(v.Pids))
			}
			if v.ReadOnly {
				args = append(args, "--read-only")
			}
			if v.NoNewPrivileges {
				args = append(args, "--security-opt", "no-new-privileges")
			}
			if v.DropAllCapabilities {
				args = append(args, "--cap-drop", "ALL")
			}
			for _, mount := range v.Volumes {
				option := "type=volume,source=" + mount.Name + ",target=" + mount.Target
				if mount.ReadOnly {
					option += ",readonly"
				}
				args = append(args, "--mount", option)
			}
			for _, k := range config.Keys(v.Environment) {
				args = append(args, "--env", k+"="+v.Environment[k])
			}
			for _, p := range v.Publish {
				args = append(args, "--publish", p)
			}
			args = append(args, v.Image)
			args = append(args, v.Command...)
			return run("docker", args...)
		case "start", "stop", "restart":
			return run("docker", op, name)
		case "remove":
			return run("docker", "rm", name)
		}
	}
	if len(w) == 4 && w[0] == "request" && w[1] == "image" && w[2] == "pull" {
		if err := safeImage(w[3]); err != nil {
			return "", err
		}
		return run("docker", "pull", w[3])
	}
	if len(w) == 4 && w[0] == "request" && w[1] == "volume" && config.Name.MatchString(w[2]) {
		if w[3] == "create" {
			return run("docker", "volume", "create", w[2])
		}
		if w[3] == "remove" {
			return run("docker", "volume", "rm", w[2])
		}
	}
	if len(w) == 7 && strings.Join(w[:3], " ") == "request storage datastore" && w[4] == "format" && w[5] == "confirm" && w[6] == w[3] {
		return formatDatastore(w[3], c)
	}
	if len(w) == 2 && w[0] == "ping" {
		if !config.Interface.MatchString(w[1]) || strings.HasPrefix(w[1], "-") {
			return "", fmt.Errorf("invalid ping destination")
		}
		return run("ping", "-c", "3", "-W", "2", w[1])
	}
	return "", fmt.Errorf("unknown command or invalid arguments; enter ? for help")
}

func disks() (string, error) {
	entries, err := os.ReadDir("/sys/class/block")
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("Disk             Sectors (512 bytes)  Read-only\n")
	for _, e := range entries {
		n := e.Name()
		if !config.Disk.MatchString(n) {
			continue
		}
		size, _ := os.ReadFile("/sys/class/block/" + n + "/size")
		ro, _ := os.ReadFile("/sys/class/block/" + n + "/ro")
		fmt.Fprintf(&b, "%-16s %-20s %s\n", n, strings.TrimSpace(string(size)), strings.TrimSpace(string(ro)))
	}
	return b.String(), nil
}

func resources() (string, error) {
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func formatDatastore(name string, c config.Config) (string, error) {
	if name != "data0" {
		return "", fmt.Errorf("only datastore data0 is supported")
	}
	d, ok := c.Datastores[name]
	if !ok {
		return "", fmt.Errorf("configure the datastore first")
	}
	if err := c.Validate(); err != nil {
		return "", err
	}
	states, err := control(0, "")
	if err != nil {
		return "", err
	}
	if states["datastore"].PID != 0 {
		return "", fmt.Errorf("stop datastore service before initialization")
	}
	existing, probeErr := exec.Command("blkid", "-t", "LABEL=FRDATA", "-o", "device").CombinedOutput()
	if probeErr == nil && len(bytes.TrimSpace(existing)) != 0 {
		return "", fmt.Errorf("an FRDATA datastore already exists; initialization would create an ambiguous label")
	}
	if probeErr != nil {
		exit, ok := probeErr.(*exec.ExitError)
		if !ok || exit.ExitCode() != 2 {
			return "", fmt.Errorf("cannot check existing datastores: %v", probeErr)
		}
	}
	mounts, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return "", err
	}
	args := []string{"-L", "FRDATA", "-d", d.Preset, "-m", d.Preset}
	seen := map[string]bool{}
	for _, disk := range d.Disks {
		path, err := filepath.EvalSymlinks("/dev/" + disk)
		if err != nil {
			return "", err
		}
		n := filepath.Base(path)
		if !config.Disk.MatchString(n) || !strings.HasPrefix(path, "/dev/") || seen[path] {
			return "", fmt.Errorf("invalid or duplicate whole disk")
		}
		seen[path] = true
		var st syscall.Stat_t
		if err = syscall.Stat(path, &st); err != nil || st.Mode&syscall.S_IFMT != syscall.S_IFBLK {
			return "", fmt.Errorf("%s is not a block device", path)
		}
		if _, err = os.Stat("/sys/class/block/" + n + "/partition"); !os.IsNotExist(err) {
			return "", fmt.Errorf("partitions cannot be initialized")
		}
		descendants, err := filepath.Glob("/sys/class/block/" + n + "*")
		if err != nil {
			return "", err
		}
		for _, device := range descendants {
			holders, err := os.ReadDir(device + "/holders")
			if err != nil || len(holders) > 0 {
				return "", fmt.Errorf("disk is in use")
			}
			for _, line := range strings.Split(string(mounts), "\n") {
				f := strings.Fields(line)
				if len(f) > 0 && f[0] == "/dev/"+filepath.Base(device) {
					return "", fmt.Errorf("disk or partition is mounted")
				}
			}
		}
		// No -f: mkfs refuses existing filesystems and partition tables.
		args = append(args, path)
	}
	out, err := run("mkfs.btrfs", args...)
	if err != nil {
		return "", err
	}
	if _, err = control(1, "datastore"); err != nil {
		return out, err
	}
	return "Datastore initialized. Services are starting.\n" + out, nil
}
