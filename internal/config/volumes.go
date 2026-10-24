package config

import (
	"fmt"
	"strconv"
	"strings"
)

type VolumeDefinition struct {
	Driver  string            `json:"driver,omitempty"`
	Options map[string]string `json:"options,omitempty"`
	Labels  map[string]string `json:"labels,omitempty"`
}

func (v VolumeDefinition) Validate() error {
	if v.Driver != "" && !DriverName.MatchString(v.Driver) {
		return fmt.Errorf("invalid volume driver")
	}
	for _, m := range []map[string]string{v.Options, v.Labels} {
		for k, value := range m {
			if k == "" || strings.ContainsAny(k, "\x00\r\n=") || strings.ContainsAny(value, "\x00\r\n") {
				return fmt.Errorf("invalid volume option")
			}
		}
	}
	return nil
}

func (v VolumeDefinition) API(name string) map[string]any {
	labels := map[string]string{}
	for k, value := range v.Labels {
		labels[k] = value
	}
	labels["com.fractalcr.managed"] = "true"
	return map[string]any{"Name": name, "Driver": v.Driver, "DriverOpts": v.Options, "Labels": labels}
}

func (v VolumeDefinition) RenderCLI(name string, line func(string, ...any)) {
	line("!\nvolume %s", name)
	if v.Driver != "" {
		line(" driver %s", strconv.Quote(v.Driver))
	}
	for _, item := range []struct {
		key    string
		values map[string]string
	}{{"option", v.Options}, {"label", v.Labels}} {
		for _, k := range Keys(item.values) {
			line(" %s %s %s", item.key, strconv.Quote(k), strconv.Quote(item.values[k]))
		}
	}
	line(" exit")
}
