package main

import (
	"fractalcr/internal/workloadcatalog"
	"github.com/docker/cli/cli/command"
	"reflect"
	"testing"
)

func TestApplianceFileArguments(t *testing.T) {
	client, err := command.NewDockerCli()
	if err != nil {
		t.Fatal(err)
	}
	model := catalog(engineRoot(client))
	for _, tc := range []struct{ args, want []string }{
		{[]string{"compose", "-f", "data0:app.yaml", "config"}, []string{"compose", "-f", "/datastore/data0/files/app.yaml", "config"}},
		{[]string{"compose", "--env-file=data0:vars", "up", "web", "--detach"}, []string{"compose", "--env-file=/datastore/data0/files/vars", "up", "web", "--detach"}},
		{[]string{"build", "-fdata0:Dockerfile", "--output", "type=local,dest=data0:result", "data0:context"}, []string{"build", "-f/datastore/data0/files/Dockerfile", "--output", "type=local,dest=/datastore/data0/files/result", "/datastore/data0/files/context"}},
		{[]string{"run", "--env", "FILE=data0:literal", "image", "--file", "data0:literal"}, []string{"run", "--env", "FILE=data0:literal", "image", "--file", "data0:literal"}},
		{[]string{"exec", "app", "echo", "--output=data0:literal"}, []string{"exec", "app", "echo", "--output=data0:literal"}},
		{[]string{"cp", "app:/file", "data0:copy"}, []string{"cp", "app:/file", "/datastore/data0/files/copy"}},
		{[]string{"context", "import", "lab", "data0:lab.context"}, []string{"context", "import", "lab", "/datastore/data0/files/lab.context"}},
		{[]string{"context", "export", "lab", "data0:lab.context"}, []string{"context", "export", "lab", "/datastore/data0/files/lab.context"}},
		{[]string{"plugin", "create", "local/driver", "data0:plugin"}, []string{"plugin", "create", "local/driver", "/datastore/data0/files/plugin"}},
		{[]string{"build", "--ssh", "default=data0:key1,data0:key2", "."}, []string{"build", "--ssh", "default=/datastore/data0/files/key1,/datastore/data0/files/key2", "."}},
		{[]string{"context", "create", "lab", "--docker", "host=tcp://host:2376,ca=data0:ca.pem"}, []string{"context", "create", "lab", "--docker", "host=tcp://host:2376,ca=/datastore/data0/files/ca.pem"}},
		{[]string{"run", "-itvdata0:input:/input:ro", "image"}, []string{"run", "-itv/datastore/data0/files/input:/input:ro", "image"}},
		{[]string{"run", "-itv=data0:input:/input:ro", "image"}, []string{"run", "-itv=/datastore/data0/files/input:/input:ro", "image"}},
		{[]string{"secret", "create", "key", "data0:key"}, []string{"secret", "create", "key", "/datastore/data0/files/key"}},
		{[]string{"run", "--mount", "type=bind,src=data0:input,target=/input", "image"}, []string{"run", "--mount", "type=bind,src=/datastore/data0/files/input,target=/input", "image"}},
		{[]string{"build", "--secret", "type=env,id=key,src=data0:literal", "."}, []string{"build", "--secret", "type=env,id=key,src=data0:literal", "."}},
	} {
		got, err := workloadcatalog.PathArguments(model, tc.args)
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%v: got %v, %v; want %v", tc.args, got, err, tc.want)
		}
	}
	if _, err := workloadcatalog.PathArguments(model, []string{"compose", "--file", "data0:../config/startup-config.json", "config"}); err == nil {
		t.Fatal("accepted volume escape")
	}
}
