package config

import (
	"fmt"
	"github.com/docker/go-connections/nat"
)

// ContainerRequest is shared by the live operation and offline configuration
// export, so the exported plan cannot omit options accepted by the CLI.
func (c Config) ContainerRequest(name string) (map[string]any, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	v, ok := c.Containers[name]
	if !ok || v.Image == "" {
		return nil, fmt.Errorf("container requires a configured image")
	}
	host := map[string]any{"Runtime": c.EffectiveRuntime(name), "ReadonlyRootfs": v.ReadOnly}
	if v.Network != "" {
		host["NetworkMode"] = v.Network
	}
	if v.Restart != "" {
		host["RestartPolicy"] = map[string]any{"Name": v.Restart}
	}
	if v.MemoryMiB > 0 {
		host["Memory"] = int64(v.MemoryMiB) * 1024 * 1024
	}
	if v.Pids > 0 {
		host["PidsLimit"] = v.Pids
	}
	if v.NoNewPrivileges {
		host["SecurityOpt"] = []string{"no-new-privileges"}
	}
	if v.DropAllCapabilities {
		host["CapDrop"] = []string{"ALL"}
	}
	mounts := []map[string]any{}
	for _, m := range v.Volumes {
		mounts = append(mounts, map[string]any{"Type": "volume", "Source": m.Name, "Target": m.Target, "ReadOnly": m.ReadOnly})
	}
	host["Mounts"] = mounts
	env := []string{}
	for _, key := range Keys(v.Environment) {
		env = append(env, key+"="+v.Environment[key])
	}
	exposed, bindings, err := nat.ParsePortSpecs(v.Publish)
	if err != nil {
		return nil, err
	}
	host["PortBindings"] = bindings
	body := map[string]any{"Image": v.Image, "Cmd": v.Command, "Env": env, "HostConfig": host, "ExposedPorts": exposed}
	v.ContainerOptions.ApplyAPI(body, host)
	if v.LogDriver == "" && len(v.LogOptions) > 0 {
		driver := c.Engine.LogDriver
		if driver == "" {
			driver = "local"
		}
		host["LogConfig"] = map[string]any{"Type": driver, "Config": v.LogOptions}
	}
	return body, nil
}
