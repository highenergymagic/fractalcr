package appliance

import (
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

const debugName = "fractal-platform-debug"
const debugImage = "debian:trixie-slim"

type debugContainer struct {
	ID     string
	Names  []string
	Labels map[string]string
}

var debugContainers = func() ([]debugContainer, error) {
	var entries []debugContainer
	err := engineJSON("GET", "/containers/json?all=1", nil, &entries)
	return entries, err
}
var deleteDebugContainer = func(id string) error {
	_, err := engineAction("DELETE", "/containers/"+id+"?force=1", nil, "")
	return err
}

func removeDebug() error {
	entries, err := debugContainers()
	if err != nil {
		return err
	}
	var owned *debugContainer
	for i := range entries {
		entry := &entries[i]
		matched := false
		for _, name := range entry.Names {
			if name == "/"+debugName {
				matched = true
			}
		}
		if !matched {
			continue
		}
		if owned != nil || entry.ID == "" || entry.Labels["com.fractalcr.debug"] != "true" {
			return fmt.Errorf("debug container name is occupied by another container")
		}
		owned = entry
	}
	if owned == nil {
		return nil
	}
	return deleteDebugContainer(owned.ID)
}

func debugDisable() (string, error) {
	if _, err := control(2, "debug-container"); err != nil {
		return "", err
	}
	if err := waitState(func(states map[string]Status) bool {
		s, ok := states["debug-container"]
		return ok && s.State == 0 && s.PID == 0
	}); err != nil {
		return "", err
	}
	// Also reconcile a container left behind by a killed/crashed worker.
	if err := removeDebug(); err != nil {
		return "", fmt.Errorf("debug service stopped, but container cleanup failed: %w", err)
	}
	return "Debug service disabled; container removed.", nil
}

func debugEnable() (string, error) {
	if _, err := engineRequest("GET", "/images/"+debugImage+"/json", nil); err != nil {
		if _, err = pullImage(debugImage); err != nil {
			return "", err
		}
	}
	if _, err := control(1, "debug-container"); err != nil {
		return "", err
	}
	return "Privileged platform debug service enabled; host root is mounted at /host.\nUse request platform debug service-container enter once the service is ready.", nil
}
func DebugService() (result error) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(signals)
	if err := removeDebug(); err != nil {
		return err
	}
	// Install cleanup before creation: an interrupted Docker client may still
	// have created the container. Ownership is checked again before removal.
	defer func() {
		if err := removeDebug(); err != nil {
			result = fmt.Errorf("debug container cleanup failed: %w", err)
		}
	}()
	_, err := engineAction("POST", "/containers/create?name="+debugName, map[string]any{
		"Image": debugImage, "Cmd": []string{"sleep", "infinity"}, "Labels": map[string]string{"com.fractalcr.debug": "true"},
		"HostConfig": map[string]any{"Runtime": "runc", "Privileged": true, "PidMode": "host", "NetworkMode": "host", "Binds": []string{"/:/host:ro"}},
	}, "")
	if err != nil {
		return err
	}
	if _, err = engineAction("POST", "/containers/"+debugName+"/start", nil, ""); err != nil {
		return err
	}
	if fd, e := strconv.Atoi(os.Getenv("FCR_READY_FD")); e == nil {
		f := os.NewFile(uintptr(fd), "ready")
		f.Write([]byte{'R'})
		f.Close()
		os.Unsetenv("FCR_READY_FD")
	}
	timer := time.NewTicker(2 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-signals:
			return nil
		case <-timer.C:
			var container struct{ State struct{ Running bool } }
			err := engineJSON("GET", "/containers/"+debugName+"/json", nil, &container)
			if err != nil || !container.State.Running {
				return fmt.Errorf("debug container exited")
			}
		}
	}
}
