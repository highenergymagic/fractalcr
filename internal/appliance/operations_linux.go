package appliance

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"fractalcr/internal/config"
)

func Operations(w []string, c config.Config) (string, error) {
	if len(w) > 0 {
		switch w[0] {
		case "copy":
			return copyFile(w, c)
		case "dir", "delete", "move", "rename", "mkdir", "rmdir":
			if len(w) == 3 && w[0] == "delete" && w[1] == "/force" {
				w = []string{"delete", w[2]}
			}
			return fileOperation(w)
		}
	}
	line := strings.Join(w, " ")
	if line == "request system container-isolation kata-containers vm-factory create" {
		return prepareVMFactory()
	}
	if len(w) >= 5 && w[1] == "storage" && w[2] == "datastore" {
		if w[3] != "data0" {
			return "", fmt.Errorf("datastore %s is unavailable", w[3])
		}
		if w[0] == "show" && len(w) == 5 {
			if w[4] == "snapshots" {
				return snapshotOperation("list", "")
			}
			if w[4] == "scrub" || w[4] == "balance" {
				return maintenanceOperation(w[4], "status")
			}
		}
		if w[0] == "request" {
			if len(w) == 6 && (w[4] == "scrub" || w[4] == "balance") {
				return maintenanceOperation(w[4], w[5])
			}
			if len(w) == 7 && w[4] == "snapshot" && w[5] == "create" {
				return snapshotOperation("create", w[6])
			}
			if len(w) == 9 && w[4] == "snapshot" && w[5] == "delete" && w[7] == "confirm" && w[8] == w[6] {
				return snapshotOperation("delete", w[6])
			}
		}
	}
	switch line {
	case "show file systems":
		return fileOperation(w)
	case "show container-isolation detail":
		return isolationStatus(c), nil
	case "request platform debug service-container enable":
		return debugEnable()
	case "request platform debug service-container disable":
		return debugDisable()
	case "show container-isolation capabilities":
		return isolationStatus(c), nil
	case "show interfaces", "show ip interface brief":
		return showInterfaces(line == "show ip interface brief", c)
	case "show ip route":
		return showRoutes()
	case "show ipv6 route":
		return showRoutesFamily(syscall.AF_INET6)
	case "show ipv6 interface brief":
		var out strings.Builder
		for _, port := range append(platformPorts(), virtualDisplayPorts()...) {
			addresses, err := port.Addrs()
			if err != nil {
				continue
			}
			fmt.Fprintf(&out, "%s\n", port.Name)
			for _, address := range addresses {
				if strings.Contains(address.String(), ":") {
					fmt.Fprintf(&out, "  %s\n", address.String())
				}
			}
		}
		return out.String(), nil
	case "show containers":
		return engineInventory("containers")
	case "show images":
		return engineInventory("images")
	case "show volumes":
		return engineInventory("volumes")
	case "show storage":
		return storageStatus(c)
	case "show storage disks":
		return disks()
	case "show storage datastore data0":
		return storageStatus(c)
	case "show system resources":
		return resources()
	case "show clock":
		return time.Now().UTC().Format("15:04:05.000 MST Mon Jan 2 2006"), nil
	case "show ntp status":
		return ntpStatus()
	case "show ntp associations":
		return ntpAssociations()
	case "show networks":
		return engineInventory("networks")
	}
	if len(w) >= 3 && w[1] == "network" {
		return networkOperation(w, c)
	}
	if len(w) == 4 && w[0] == "show" && w[1] == "container" && config.Name.MatchString(w[2]) {
		switch w[3] {
		case "detail":
			return containerDetail(w[2])
		case "logs":
			return containerLogs(w[2])
		}
	}
	if len(w) == 4 && w[0] == "request" && w[1] == "container" && config.Name.MatchString(w[2]) {
		name, op := w[2], w[3]
		switch op {
		case "create":
			return createContainer(name, c)
		case "start", "stop", "restart":
			return engineAction("POST", "/containers/"+name+"/"+op, nil, "Container "+name+": "+op+" completed.")
		case "remove":
			return engineAction("DELETE", "/containers/"+name, nil, "Container "+name+" removed.")
		}
	}
	if len(w) == 4 && w[0] == "request" && w[1] == "image" && w[2] == "pull" {
		if err := safeImage(w[3]); err != nil {
			return "", err
		}
		return pullImage(w[3])
	}
	if len(w) == 4 && w[0] == "request" && w[1] == "volume" && config.Name.MatchString(w[2]) {
		if w[3] == "create" {
			return engineAction("POST", "/volumes/create", c.Volumes[w[2]].API(w[2]), "Volume "+w[2]+" created.")
		}
		if w[3] == "remove" {
			return engineAction("DELETE", "/volumes/"+w[2], nil, "Volume "+w[2]+" removed.")
		}
	}
	if len(w) == 4 && w[0] == "show" && w[1] == "volume" && w[3] == "detail" && config.Name.MatchString(w[2]) {
		var volume struct {
			Name, Driver, Scope string
			Labels, Options     map[string]string
		}
		if err := engineJSON("GET", "/volumes/"+w[2], nil, &volume); err != nil {
			return "", err
		}
		text := fmt.Sprintf("Volume %s\n  Driver: %s\n  Scope: %s\n", displayText(volume.Name), displayText(volume.Driver), displayText(volume.Scope))
		for _, key := range config.Keys(volume.Labels) {
			text += fmt.Sprintf("  Label %s: %s\n", displayText(key), displayText(volume.Labels[key]))
		}
		for _, key := range config.Keys(volume.Options) {
			text += fmt.Sprintf("  Option %s: %s\n", displayText(key), displayText(volume.Options[key]))
		}
		return text, nil
	}
	if len(w) == 7 && strings.Join(w[:3], " ") == "request storage datastore" && w[4] == "format" && w[5] == "confirm" && w[6] == w[3] {
		return formatDatastore(w[3], c)
	}
	if len(w) == 2 && w[0] == "ping" {
		if !config.Interface.MatchString(w[1]) || strings.HasPrefix(w[1], "-") {
			return "", fmt.Errorf("invalid ping destination")
		}
		return pingAddress(w[1])
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

func resources() (string, error) { return resourceStatus() }

func formatDatastore(name string, c config.Config) (string, error) {
	if name != "data0" {
		return "", fmt.Errorf("only datastore data0 is supported")
	}
	d, ok := c.Datastores[name]
	if !ok {
		return "", fmt.Errorf("configure the datastore first")
	}
	if len(d.Disks) == 0 || (d.Preset == "raid1" && len(d.Disks) < 2) {
		return "", fmt.Errorf("%s: insufficient disks for %s", name, d.Preset)
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
	if err := storageRequest(1, "", nil); err != nil {
		return "", err
	}
	mounts, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return "", err
	}
	members := []string{}
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
		members = append(members, n)
	}
	err = storageRequest(2, d.Preset, members)
	if err != nil {
		return "", err
	}
	if _, err = control(1, "datastore"); err != nil {
		return "", err
	}
	return "Datastore initialized. Services are starting.", nil
}
