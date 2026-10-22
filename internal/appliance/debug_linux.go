package appliance

import (
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const debugName = "fractal-platform-debug"
const debugImage = "debian:trixie-slim"

var debugRun = func(args ...string) (string, error) {
	return runTimeout(30*time.Second, "docker", args...)
}

func removeDebug() error {
	owner, err := debugRun("ps", "--all", "--filter", "name=^/"+debugName+"$", "--format", `{{.ID}} {{.Label "com.fractalcr.debug"}}`)
	if err != nil {
		return err
	}
	if strings.TrimSpace(owner) == "" {
		return nil
	}
	fields := strings.Fields(owner)
	if len(fields) != 2 || fields[1] != "true" {
		return fmt.Errorf("debug container name is occupied by another container")
	}
	_, err = debugRun("rm", "-f", fields[0])
	return err
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
	if _, err := run("docker", "image", "inspect", debugImage); err != nil {
		if _, err = run("docker", "pull", debugImage); err != nil {
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
	_, err := run("docker", "run", "-d", "--name", debugName, "--label", "com.fractalcr.debug=true",
		"--runtime", "runc", "--privileged", "--pid", "host", "--network", "host", "--volume", "/:/host:ro", debugImage, "sleep", "infinity")
	if err != nil {
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
			state, err := run("docker", "inspect", "--format", "{{.State.Running}}", debugName)
			if err != nil || strings.TrimSpace(state) != "true" {
				return fmt.Errorf("debug container exited")
			}
		}
	}
}
