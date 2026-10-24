package buildclient

import (
	"fractalcr/internal/workloadcatalog"
	"github.com/urfave/cli/v3"
	"reflect"
	"testing"
)

func TestBuildKitAppliancePaths(t *testing.T) {
	model := commandCatalog(&cli.Command{Name: "buildkit", Commands: []*cli.Command{buildCommand}})
	args := []string{"build", "--local", "context=data0:source", "--local", "dockerfile=data0:source", "--output", "type=oci,dest=data0:result.tar", "--opt", "build-arg:FILE=data0:literal"}
	want := []string{"build", "--local", "context=/datastore/data0/files/source", "--local", "dockerfile=/datastore/data0/files/source", "--output", "type=oci,dest=/datastore/data0/files/result.tar", "--opt", "build-arg:FILE=data0:literal"}
	got, err := workloadcatalog.PathArguments(model, args)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, %v; want %v", got, err, want)
	}
}

func TestInheritedBuildKitFileFlags(t *testing.T) {
	model := commandCatalog(&cli.Command{Name: "buildkit", Flags: []cli.Flag{&cli.StringFlag{Name: "tlscacert"}}, Commands: []*cli.Command{buildCommand}})
	got, err := workloadcatalog.PathArguments(model, []string{"build", "--tlscacert", "data0:ca.pem"})
	if err != nil || !reflect.DeepEqual(got, []string{"build", "--tlscacert", "/datastore/data0/files/ca.pem"}) {
		t.Fatal(got, err)
	}
}
