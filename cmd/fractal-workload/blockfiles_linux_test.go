package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/moby/moby/client"
)

func TestGuestFilesystemAPIGuard(t *testing.T) {
	for _, tc := range []struct {
		driver, runtime  string
		running, blocked bool
	}{
		{"blockfile", "io.containerd.kata.v2", true, true},
		{"blockfile", "io.containerd.kata.v2", false, false},
		{"blockfile", "runc", true, false},
		{"overlay2", "io.containerd.kata.v2", true, false},
	} {
		t.Run(fmt.Sprintf("%s/%s/%v", tc.driver, tc.runtime, tc.running), func(t *testing.T) {
			exported := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.HasSuffix(r.URL.Path, "/info"):
					fmt.Fprintf(w, `{"Driver":%q}`, tc.driver)
				case strings.HasSuffix(r.URL.Path, "/containers/test/json"):
					fmt.Fprintf(w, `{"HostConfig":{"Runtime":%q},"State":{"Running":%v}}`, tc.runtime, tc.running)
				case strings.HasSuffix(r.URL.Path, "/containers/test/export"):
					exported = true
					_, _ = w.Write([]byte("fixture"))
				default:
					t.Errorf("unexpected API request %s", r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			var api *client.Client
			api, err := client.New(client.WithHost(server.URL), client.WithAPIVersion("1.47"), client.WithHTTPRequestHook(func(r *http.Request) error { return protectGuestFilesystem(r, api) }))
			if err != nil {
				t.Fatal(err)
			}
			defer api.Close()
			body, err := api.ContainerExport(context.Background(), "test", client.ContainerExportOptions{})
			if body != nil {
				body.Close()
			}
			if tc.blocked {
				if err == nil || !strings.Contains(err.Error(), "stop container test") || exported {
					t.Fatalf("exported=%v err=%v", exported, err)
				}
			} else if err != nil || !exported {
				t.Fatalf("exported=%v err=%v", exported, err)
			}
		})
	}
}
