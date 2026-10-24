package appliance

import (
	"compress/gzip"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
)

const blockfileSize = int64(8 << 30)

func emptyEngineForStorageChange() error {
	var containers []json.RawMessage
	if err := engineJSON("GET", "/containers/json?all=1", nil, &containers); err != nil {
		return fmt.Errorf("storage backend changes require a running, empty container engine: %w", err)
	}
	var images []json.RawMessage
	if err := engineJSON("GET", "/images/json?all=1", nil, &images); err != nil {
		return err
	}
	var usage struct{ BuildCache []json.RawMessage }
	if err := engineJSON("GET", "/system/df", nil, &usage); err != nil {
		return err
	}
	if len(containers) != 0 || len(images) != 0 || len(usage.BuildCache) != 0 {
		return fmt.Errorf("remove or export containers, images and build cache before changing storage backend")
	}
	return nil
}

// restoreBlockSeed writes only stored extents, preserving holes and bounding
// every write before it can consume datastore space. The seed is image-owned.
func restoreBlockSeed(dst *os.File, src io.Reader) error {
	z, err := gzip.NewReader(src)
	if err != nil {
		return err
	}
	defer z.Close()
	var header [16]byte
	if _, err := io.ReadFull(z, header[:]); err != nil {
		return err
	}
	if string(header[:8]) != "FCRBLK01" || binary.LittleEndian.Uint64(header[8:]) != uint64(blockfileSize) {
		return fmt.Errorf("invalid block storage seed")
	}
	if err := dst.Truncate(blockfileSize); err != nil {
		return err
	}
	var last, total uint64
	for {
		if _, err := io.ReadFull(z, header[:]); err != nil {
			return err
		}
		offset, length := binary.LittleEndian.Uint64(header[:8]), binary.LittleEndian.Uint64(header[8:])
		if offset == 0 && length == 0 {
			var extra [1]byte
			if n, err := z.Read(extra[:]); n != 0 || err != io.EOF {
				return fmt.Errorf("invalid block seed trailer")
			}
			return dst.Sync()
		}
		if offset < last || offset >= uint64(blockfileSize) || length == 0 || length > uint64(blockfileSize)-offset || length > 1<<30-total {
			return fmt.Errorf("invalid block seed extent")
		}
		if _, err := dst.Seek(int64(offset), io.SeekStart); err != nil {
			return err
		}
		if _, err := io.CopyN(dst, z, int64(length)); err != nil {
			return err
		}
		last, total = offset+length, total+length
	}
}

func ensureBlockSeed() error {
	const target = "/datastore/data0/containerd-scratch.ext4"
	if st, err := os.Lstat(target); err == nil {
		if !st.Mode().IsRegular() || st.Size() != blockfileSize {
			return fmt.Errorf("invalid existing block storage seed")
		}
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	src, err := os.Open("/usr/share/fractal/blockfile-seed.gz")
	if err != nil {
		return err
	}
	defer src.Close()
	tmp, err := os.CreateTemp(filepath.Dir(target), ".blockfile-seed-")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	if err := restoreBlockSeed(tmp, src); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// Link publishes without replacing a concurrently installed seed.
	if err := os.Link(tmp.Name(), target); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(target))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func ContainerdService() error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()
	c, err := loadRunning()
	if err != nil {
		return err
	}
	if c.Engine.StorageBackend != "blockfile" {
		notifyReady()
		<-ctx.Done()
		return nil
	}
	if err := ensureBlockSeed(); err != nil {
		return err
	}
	daemon := exec.Command("/usr/bin/containerd", "--config=/run/containerd.toml")
	daemon.Stdout, daemon.Stderr = os.Stdout, os.Stderr
	if err := daemon.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- daemon.Wait() }()
	defer func() {
		_ = daemon.Process.Signal(syscall.SIGTERM)
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			_ = daemon.Process.Kill()
			<-done
		}
	}()
	conn, err := grpc.NewClient("unix:///run/fractal-containerd/containerd.sock", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	defer conn.Close()
	api := grpc_health_v1.NewHealthClient(conn)
	deadline := time.NewTimer(60 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-done:
			done <- err
			return fmt.Errorf("container storage service exited: %v", err)
		case <-deadline.C:
			return fmt.Errorf("container storage API readiness timed out")
		case <-tick.C:
			probe, stop := context.WithTimeout(ctx, time.Second)
			response, err := api.Check(probe, &grpc_health_v1.HealthCheckRequest{})
			stop()
			if err == nil && response.Status == grpc_health_v1.HealthCheckResponse_SERVING {
				notifyReady()
				select {
				case <-ctx.Done():
					return nil
				case err := <-done:
					done <- err
					return fmt.Errorf("container storage service exited: %v", err)
				}
			}
		}
	}
}
