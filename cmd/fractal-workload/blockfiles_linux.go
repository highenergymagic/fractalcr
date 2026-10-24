package main

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/moby/moby/client"
)

// Apply this to the client's API requests, including Compose, rather than
// command spellings. The host must not mount a running VM's raw filesystem.
func guestFilesystemTarget(req *http.Request) string {
	parts := strings.Split(strings.Trim(req.URL.Path, "/"), "/")
	if len(parts) > 0 && strings.HasPrefix(parts[0], "v1.") {
		parts = parts[1:]
	}
	name := ""
	if len(parts) == 3 && parts[0] == "containers" && (parts[2] == "archive" || parts[2] == "export") {
		name = parts[1]
	}
	if len(parts) == 1 && parts[0] == "commit" {
		name = req.URL.Query().Get("container")
	}
	return name
}

func protectGuestFilesystem(req *http.Request, api client.APIClient) error {
	name := guestFilesystemTarget(req)
	if name == "" {
		return nil
	}
	info, err := api.Info(req.Context(), client.InfoOptions{})
	if err != nil {
		return err
	}
	if info.Info.Driver != "blockfile" {
		return nil
	}
	result, err := api.ContainerInspect(req.Context(), name, client.ContainerInspectOptions{})
	if err != nil {
		return err
	}
	c := result.Container
	if c.HostConfig != nil && strings.Contains(c.HostConfig.Runtime, "kata") && c.State != nil && (c.State.Running || c.State.Restarting || c.State.Paused) {
		return fmt.Errorf("stop container %s before copying, exporting or committing its block-backed filesystem", name)
	}
	return nil
}
