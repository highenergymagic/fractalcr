package appliance

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"fractalcr/internal/config"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var engineClient = &http.Client{Timeout: 120 * time.Second, Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
	return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "unix", "/run/docker.sock")
}}}

var engineVersionCache struct {
	sync.Mutex
	client  *http.Client
	version string
}

// engineStatusError retains status for idempotent API operations without
// exposing daemon response bodies or host paths to the management console.
type engineStatusError struct {
	Status  int
	Message string
}

func (e *engineStatusError) Error() string { return e.Message }

func engineVersion(ctx context.Context) string {
	engineVersionCache.Lock()
	defer engineVersionCache.Unlock()
	if engineVersionCache.client == engineClient && engineVersionCache.version != "" {
		return engineVersionCache.version
	}
	version := "1.47"
	request, err := http.NewRequestWithContext(ctx, "HEAD", "http://engine/_ping", nil)
	if err == nil {
		response, e := engineClient.Do(request)
		if e == nil {
			response.Body.Close()
			raw := response.Header.Get("API-Version")
			major, minor, ok := strings.Cut(raw, ".")
			n, e := strconv.Atoi(minor)
			if response.StatusCode == 200 && ok && major == "1" && e == nil && n > 0 && n < 47 {
				version = raw
			}
		}
	}
	engineVersionCache.client, engineVersionCache.version = engineClient, version
	return version
}

func engineRequest(method, endpoint string, body any) ([]byte, error) {
	var input io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		input = bytes.NewReader(data)
	}
	return engineBody(method, endpoint, input, body != nil)
}

func engineBody(method, endpoint string, input io.Reader, jsonBody bool) ([]byte, error) {
	return engineBodyContext(context.Background(), method, endpoint, input, jsonBody)
}
func engineBodyContext(ctx context.Context, method, endpoint string, input io.Reader, jsonBody bool) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, "http://engine/v"+engineVersion(ctx)+endpoint, input)
	if err != nil {
		return nil, fmt.Errorf("Invalid container service request")
	}
	if jsonBody {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := engineClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Container service is unavailable")
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 8*1024*1024+1))
	if err != nil {
		return nil, fmt.Errorf("Container service response was interrupted")
	}
	if len(data) > 8*1024*1024 {
		return nil, fmt.Errorf("Container service response exceeds display limit")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		// Backend messages can contain host paths and client implementation details.
		reason := http.StatusText(response.StatusCode)
		switch response.StatusCode {
		case 404:
			reason = "Object not found"
		case 409:
			reason = "Object is in use or conflicts with existing state"
		case 400:
			reason = "Invalid object configuration"
		case 500:
			reason = "Container service could not complete the operation"
		}
		return nil, &engineStatusError{Status: response.StatusCode, Message: reason}
	}
	return data, nil
}
func engineJSON(method, endpoint string, body, out any) error {
	data, err := engineRequest(method, endpoint, body)
	if err != nil {
		return err
	}
	if out != nil && len(data) > 0 {
		if json.Unmarshal(data, out) != nil {
			return fmt.Errorf("Invalid container service response")
		}
	}
	return nil
}
func engineAction(method, endpoint string, body any, message string) (string, error) {
	if err := engineJSON(method, endpoint, body, nil); err != nil {
		return "", err
	}
	return message, nil
}
func displayText(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return ' '
		}
		return r
	}, s)
}

func engineInventory(kind string) (string, error) {
	var b strings.Builder
	switch kind {
	case "containers":
		var entries []struct {
			Names                []string
			Image, State, Status string
			Ports                []struct {
				IP, Type                string
				PrivatePort, PublicPort int
			}
		}
		if err := engineJSON("GET", "/containers/json?all=1", nil, &entries); err != nil {
			return "", err
		}
		sort.Slice(entries, func(i, j int) bool { return strings.Join(entries[i].Names, "") < strings.Join(entries[j].Names, "") })
		b.WriteString("Container                Image                              State       Ports\n")
		for _, e := range entries {
			ports := []string{}
			for _, p := range e.Ports {
				if p.PublicPort > 0 {
					ports = append(ports, fmt.Sprintf("%d->%d/%s", p.PublicPort, p.PrivatePort, p.Type))
				}
			}
			fmt.Fprintf(&b, "%-24s %-34s %-11s %s\n", displayText(strings.TrimPrefix(strings.Join(e.Names, ","), "/")), displayText(e.Image), displayText(e.State), strings.Join(ports, ", "))
		}
	case "images":
		var entries []struct {
			ID       string
			RepoTags []string
			Size     int64
		}
		if err := engineJSON("GET", "/images/json", nil, &entries); err != nil {
			return "", err
		}
		b.WriteString("Image                                                        Size (bytes)\n")
		for _, e := range entries {
			name := strings.Join(e.RepoTags, ", ")
			if name == "" {
				name = e.ID
			}
			fmt.Fprintf(&b, "%-60s %12d\n", displayText(name), e.Size)
		}
	case "volumes":
		var data struct {
			Volumes []struct{ Name, Driver, Scope string }
		}
		if err := engineJSON("GET", "/volumes", nil, &data); err != nil {
			return "", err
		}
		b.WriteString("Volume                           Driver           Scope\n")
		for _, e := range data.Volumes {
			fmt.Fprintf(&b, "%-32s %-16s %s\n", displayText(e.Name), displayText(e.Driver), displayText(e.Scope))
		}
	case "networks":
		var entries []struct{ Name, Driver, Scope string }
		if err := engineJSON("GET", "/networks", nil, &entries); err != nil {
			return "", err
		}
		b.WriteString("Network                          Driver           Scope\n")
		for _, e := range entries {
			fmt.Fprintf(&b, "%-32s %-16s %s\n", displayText(e.Name), displayText(e.Driver), displayText(e.Scope))
		}
	}
	return b.String(), nil
}
func containerDetail(name string) (string, error) {
	var c struct {
		Name, ID string
		State    struct {
			Status        string
			Pid, ExitCode int
		}
		Config struct {
			Image string
			Cmd   []string
		}
		HostConfig struct {
			Runtime        string
			Memory         int64
			ReadonlyRootfs bool
		}
		NetworkSettings struct {
			Networks map[string]struct{ IPAddress string }
		}
	}
	if err := engineJSON("GET", "/containers/"+url.PathEscape(name)+"/json", nil, &c); err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Container %s\n  Image: %s\n  State: %s\n  Runtime: %s\n  Memory limit: %d bytes\n  Read-only root: %t\n  Exit code: %d\n", displayText(strings.TrimPrefix(c.Name, "/")), displayText(c.Config.Image), displayText(c.State.Status), displayText(c.HostConfig.Runtime), c.HostConfig.Memory, c.HostConfig.ReadonlyRootfs, c.State.ExitCode)
	for _, name := range config.Keys(c.NetworkSettings.Networks) {
		fmt.Fprintf(&b, "  Network %s, address %s\n", displayText(name), displayText(c.NetworkSettings.Networks[name].IPAddress))
	}
	return b.String(), nil
}
func containerLogs(name string) (string, error) {
	return containerLogsContext(context.Background(), name)
}
func containerLogsContext(ctx context.Context, name string) (string, error) {
	data, err := engineBodyContext(ctx, "GET", "/containers/"+url.PathEscape(name)+"/logs?stdout=1&stderr=1&tail=200", nil, false)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for len(data) >= 8 && data[0] <= 2 && data[1] == 0 && data[2] == 0 && data[3] == 0 {
		size := int(binary.BigEndian.Uint32(data[4:8]))
		if size > len(data)-8 {
			return "", fmt.Errorf("Incomplete container log response")
		}
		b.Write(data[8 : 8+size])
		data = data[8+size:]
	}
	b.Write(data)
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || r >= 32 && r != 127 {
			return r
		}
		return -1
	}, b.String()), nil
}
func createContainer(name string, c config.Config) (string, error) {
	body, err := c.ContainerRequest(name)
	if err != nil {
		return "", err
	}
	return engineAction("POST", "/containers/create?name="+url.QueryEscape(name), body, "Container "+name+" created.")
}
func pullImage(name string) (string, error) {
	if err := safeImage(name); err != nil {
		return "", err
	}
	data, err := engineRequest("POST", "/images/create?fromImage="+url.QueryEscape(name), nil)
	if err != nil {
		return "", err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	for {
		var event struct{ Error string }
		err = decoder.Decode(&event)
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("Invalid image transfer response")
		}
		if event.Error != "" {
			return "", fmt.Errorf("Image transfer failed")
		}
	}
	return "Image " + name + " installed.", nil
}

func engineJSONContext(ctx context.Context, method, endpoint string, body, out any) error {
	var input io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		input = bytes.NewReader(data)
	}
	data, err := engineBodyContext(ctx, method, endpoint, input, body != nil)
	if err != nil {
		return err
	}
	if out != nil && len(data) > 0 {
		return json.Unmarshal(data, out)
	}
	return nil
}
