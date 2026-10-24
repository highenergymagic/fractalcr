package appliance

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

type vmFactoryState struct {
	Enabled, Ready bool
	Created        time.Time
	MemoryMiB      uint32 `json:"memory_mib"`
}

func queryVMFactory(method, path string) (state vmFactoryState, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", "/run/fractal-vm-factory.sock")
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport}
	req, err := http.NewRequestWithContext(ctx, method, "http://factory"+path, nil)
	if err != nil {
		return state, err
	}
	response, err := client.Do(req)
	if err != nil {
		return state, fmt.Errorf("VM factory service is unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return state, fmt.Errorf("enable VM factory in container-isolation configuration first")
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 8192)).Decode(&state); err != nil {
		return state, err
	}
	return state, nil
}

func prepareVMFactory() (string, error) {
	state, err := queryVMFactory("POST", "/prepare")
	if err != nil {
		return "", err
	}
	if !state.Enabled || !state.Ready {
		return "", fmt.Errorf("VM factory is not ready")
	}
	return fmt.Sprintf("CLH VM factory ready, %d MiB template, created %s.", state.MemoryMiB, state.Created.Format(time.RFC3339)), nil
}
