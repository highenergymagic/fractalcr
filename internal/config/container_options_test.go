package config

import (
	"reflect"
	"testing"
	"time"
)

func TestContainerNamespaceAndCPUValidation(t *testing.T) {
	for _, v := range []ContainerOptions{{PIDMode: "shareable"}, {UTSMode: "private"}, {UTSMode: "container:other"}, {CPUSet: "9-2"}, {CPUSet: "999999999999999999999"}} {
		if v.Validate() == nil {
			t.Fatalf("accepted invalid options: %+v", v)
		}
	}
	for _, v := range []ContainerOptions{{PIDMode: "container:other"}, {IPCMode: "shareable"}, {UTSMode: "host"}, {CPUSet: "0,2-8"}} {
		if err := v.Validate(); err != nil {
			t.Fatal(v, err)
		}
	}
}

func TestContainerOptionsEngineSchema(t *testing.T) {
	v := ContainerOptions{NanoCPUs: 1250000000, CPUShares: 512, MemoryReservationMiB: 128, MemorySwapMiB: -1, ShmMiB: 64, User: "1000", Entrypoint: []string{"/app"}, CapAdd: []string{"NET_BIND_SERVICE"}, CapDrop: []string{"NET_RAW"}, LogDriver: "local", LogOptions: map[string]string{"max-size": "10m"}, Devices: map[string]Device{"/dev/test": {Source: "/dev/null", Permissions: "rw"}}, Ulimits: map[string]Ulimit{"nofile": {Soft: 1024, Hard: 2048}}, Health: Healthcheck{Command: []string{"/check"}, Interval: "30s", Retries: 3}}
	if err := v.Validate(); err != nil {
		t.Fatal(err)
	}
	body, host := map[string]any{}, map[string]any{"CapDrop": []string{"ALL"}}
	v.ApplyAPI(body, host)
	if host["NanoCpus"] != int64(1250000000) || host["MemorySwap"] != int64(-1) || host["MemoryReservation"] != int64(128*1024*1024) || body["User"] != "1000" {
		t.Fatal(body, host)
	}
	if !reflect.DeepEqual(host["CapDrop"], []string{"ALL", "NET_RAW"}) {
		t.Fatal(host)
	}
	health := body["Healthcheck"].(map[string]any)
	if !reflect.DeepEqual(health["Test"], []string{"CMD", "/check"}) || health["Interval"] != int64(30*time.Second) {
		t.Fatal(health)
	}
	devices := host["Devices"].([]map[string]string)
	if devices[0]["PathOnHost"] != "/dev/null" || devices[0]["PathInContainer"] != "/dev/test" {
		t.Fatal(devices)
	}
}
