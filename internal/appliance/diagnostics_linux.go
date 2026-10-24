package appliance

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// Diagnostics exercises real Engine objects and the embedded container probe.
// The probe is a workload executable, never a host shell or command adapter.
func Diagnostics() (result error) {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()
	var info struct {
		Driver, CgroupVersion string
		SwapLimit             bool
	}
	if err := engineJSONContext(ctx, "GET", "/info", nil, &info); err != nil {
		return err
	}
	expectedDriver := "overlay2"
	cfg, err := loadRunning()
	if err != nil {
		return err
	}
	if cfg.Engine.StorageBackend == "blockfile" {
		expectedDriver = "blockfile"
	}
	if info.Driver != expectedDriver || info.CgroupVersion != "2" || !info.SwapLimit {
		return fmt.Errorf("storage or cgroup prerequisites failed")
	}
	root, err := os.OpenFile("/.fractal-root-write-test", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err == nil {
		root.Close()
		os.Remove("/.fractal-root-write-test")
		return fmt.Errorf("system root is writable")
	}
	if !errors.Is(err, syscall.EROFS) {
		return fmt.Errorf("system root read-only check failed")
	}
	temp, err := os.CreateTemp("/run", "fractal-write-test-")
	if err != nil {
		return err
	}
	temp.Close()
	os.Remove(temp.Name())
	fmt.Println("HOST_ROOT_READONLY_OK")
	probe, err := os.ReadFile("/usr/share/fractal/probe.tar")
	if err != nil {
		return err
	}
	name := fmt.Sprintf("fractal/probe:%x", sha256.Sum256(probe))
	if err = engineJSONContext(ctx, "GET", "/images/"+name+"/json", nil, nil); err != nil {
		query := url.Values{"fromSrc": {"-"}, "repo": {"fractal/probe"}, "tag": {fmt.Sprintf("%x", sha256.Sum256(probe))}, "changes": {`ENTRYPOINT ["/probe"]`}}
		stream, e := engineBodyContext(ctx, "POST", "/images/create?"+query.Encode(), bytes.NewReader(probe), false)
		if e != nil {
			return e
		}
		decoder := json.NewDecoder(bytes.NewReader(stream))
		for {
			var event struct{ Error string }
			e = decoder.Decode(&event)
			if e == io.EOF {
				break
			}
			if e != nil {
				return e
			}
			if event.Error != "" {
				return fmt.Errorf("embedded probe image import failed")
			}
		}
	}
	var created []string
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 60*time.Second)
		defer stop()
		for _, id := range created {
			e := removeDiagnosticContainer(cleanup, id, 10*time.Second, 200*time.Millisecond,
				func(ctx context.Context, id string) error {
					return engineJSONContext(ctx, "DELETE", "/containers/"+id+"?force=1", nil, nil)
				})
			if e != nil {
				result = errors.Join(result, fmt.Errorf("diagnostic container cleanup failed: %w", e))
			}
		}
	}()
	create := func(command, network string, swap, publish bool) (string, error) {
		host := map[string]any{"NetworkMode": network, "ReadonlyRootfs": true, "CapDrop": []string{"ALL"}, "SecurityOpt": []string{"no-new-privileges"}, "Memory": 32 * 1024 * 1024, "PidsLimit": 32}
		body := map[string]any{"Image": name, "HostConfig": host, "Labels": map[string]string{"com.fractalcr.diagnostics": "true"}}
		if command != "" {
			body["Cmd"] = []string{command}
		} else {
			host["Binds"] = []string{"/datastore/data0/selftest:/state"}
		}
		if swap {
			host["Runtime"] = "runc"
			host["MemorySwap"] = 48 * 1024 * 1024
		}
		if publish {
			host["PortBindings"] = map[string]any{"8080/tcp": []map[string]string{{"HostIp": "127.0.0.1", "HostPort": ""}}}
			body["ExposedPorts"] = map[string]any{"8080/tcp": map[string]any{}}
		}
		var container struct{ ID string }
		if e := engineJSONContext(ctx, "POST", "/containers/create", body, &container); e != nil {
			return "", e
		}
		if container.ID == "" {
			return "", fmt.Errorf("diagnostic container identifier missing")
		}
		created = append(created, container.ID)
		if e := engineJSONContext(ctx, "POST", "/containers/"+container.ID+"/start", nil, nil); e != nil {
			return "", e
		}
		return container.ID, nil
	}
	for _, test := range []struct {
		command, network string
		swap             bool
	}{{"swap-limit", "none", true}, {"", "none", false}, {"network", "bridge", false}} {
		id, e := create(test.command, test.network, test.swap, false)
		if e != nil {
			return e
		}
		var status struct {
			StatusCode *int64
			Error      *struct{ Message string }
		}
		if e = engineJSONContext(ctx, "POST", "/containers/"+id+"/wait?condition=not-running", nil, &status); e != nil {
			return e
		}
		if status.StatusCode == nil || *status.StatusCode != 0 || status.Error != nil {
			return fmt.Errorf("container diagnostic %s failed", test.command)
		}
		logs, e := containerLogsContext(ctx, id)
		if e != nil {
			return e
		}
		fmt.Println(logs)
	}
	id, err := create("serve", "bridge", false, true)
	if err != nil {
		return err
	}
	var container struct {
		NetworkSettings struct {
			Ports map[string][]struct{ HostIp, HostPort string }
		}
	}
	if err = engineJSONContext(ctx, "GET", "/containers/"+id+"/json", nil, &container); err != nil {
		return err
	}
	ports := container.NetworkSettings.Ports["8080/tcp"]
	if len(ports) != 1 || ports[0].HostIp != "127.0.0.1" {
		return fmt.Errorf("diagnostic port binding missing")
	}
	client := &http.Client{Timeout: time.Second, Transport: &http.Transport{Proxy: nil}}
	defer client.CloseIdleConnections()
	for retry := 0; retry < 30; retry++ {
		request, e := http.NewRequestWithContext(ctx, "GET", "http://127.0.0.1:"+ports[0].HostPort+"/", nil)
		if e != nil {
			return e
		}
		response, e := client.Do(request)
		if e == nil {
			data, e := io.ReadAll(io.LimitReader(response.Body, 1024))
			response.Body.Close()
			if e == nil && bytes.Contains(data, []byte("FCR_NETWORK_OK")) {
				fmt.Println("CONTAINER_PORT_PUBLISH_OK")
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	return fmt.Errorf("container port publishing diagnostic failed")
}

// A timed-out DELETE can already have removed the object. Retry that same
// owned identifier, accepting not-found as completion. Never extend the
// caller's cleanup budget or select unrelated containers by name/label.
func removeDiagnosticContainer(ctx context.Context, id string, attemptTimeout, delay time.Duration, remove func(context.Context, string) error) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		attempt, cancel := context.WithTimeout(ctx, attemptTimeout)
		err := remove(attempt, id)
		cancel()
		if err == nil {
			return nil
		}
		var status *engineStatusError
		if errors.As(err, &status) {
			if status.Status == http.StatusNotFound {
				return nil
			}
			if status.Status < 500 && status.Status != http.StatusConflict && status.Status != http.StatusTooManyRequests {
				return err
			}
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return errors.Join(err, ctx.Err())
		case <-timer.C:
		}
	}
}
