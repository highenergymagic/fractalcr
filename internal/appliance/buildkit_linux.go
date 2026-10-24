package appliance

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/moby/buildkit/client"
)

// BuildkitService supervises the daemon; readiness comes from its typed gRPC
// worker inventory, never from executing buildctl or parsing its output.
func BuildkitService() error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()
	daemon := exec.Command("/usr/bin/buildkitd", "--root=/datastore/data0/buildkit",
		"--addr=unix:///run/buildkit/buildkitd.sock", "--containerd-worker=false",
		"--oci-worker=true", "--oci-worker-snapshotter=overlayfs", "--oci-worker-net=bridge",
		"--oci-worker-binary=/usr/libexec/fractal-buildkit-runc")
	daemon.Stdout, daemon.Stderr = os.Stdout, os.Stderr
	if err := daemon.Start(); err != nil {
		return err
	}
	done := make(chan struct{})
	var result error
	go func() { result = daemon.Wait(); close(done) }()
	defer func() {
		_ = daemon.Process.Signal(syscall.SIGTERM)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = daemon.Process.Kill()
			<-done
		}
	}()
	api, err := client.New(ctx, "unix:///run/buildkit/buildkitd.sock")
	if err != nil {
		return err
	}
	defer api.Close()
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	ready := false
	for !ready {
		select {
		case <-ctx.Done():
			return nil
		case <-done:
			return fmt.Errorf("build service exited before readiness: %v", result)
		case <-deadline.C:
			return fmt.Errorf("build service worker API did not become ready")
		case <-tick.C:
			probe, stop := context.WithTimeout(ctx, 2*time.Second)
			workers, e := api.ListWorkers(probe)
			stop()
			ready = e == nil && len(workers) > 0
		}
	}
	notifyReady()
	select {
	case <-ctx.Done():
		return nil
	case <-done:
		return fmt.Errorf("build service exited: %v", result)
	}
}

func ConfigurationService() error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()
	if err := Prepare(); err != nil {
		return err
	}
	defer os.Remove("/run/startup-config.json")
	notifyReady()
	<-ctx.Done()
	return nil
}
