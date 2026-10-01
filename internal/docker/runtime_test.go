package docker

import (
	"reflect"
	"testing"
)

func TestRuntimeFlagsPreserveHostGateway(t *testing.T) {
	output := "lidoo-odoo:18\t[\"HOST=db\"]\t[{\"Source\":\"/tmp/addons\",\"Destination\":\"/opt/addons\"}]\t{\"lidoo-net\":{}}\t[\"host.docker.internal:host-gateway\"]"
	flags, image, err := runtimeFlagsFromInspect("profile", output)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--network", "lidoo-net", "--add-host", "host.docker.internal:host-gateway", "--env", "HOST=db", "-v", "/tmp/addons:/opt/addons"}
	if image != "lidoo-odoo:18" || !reflect.DeepEqual(flags, want) {
		t.Fatalf("image=%q flags=%v", image, flags)
	}
}

func TestRuntimeFlagsAllowNoHostAliases(t *testing.T) {
	flags, _, err := runtimeFlagsFromInspect("profile", "image\t[]\t[]\t{}\tnull")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(flags, []string{"--network", networkName}) {
		t.Fatalf("flags=%v", flags)
	}
}

func TestRuntimeFlagsRejectInvalidHostAliases(t *testing.T) {
	_, _, err := runtimeFlagsFromInspect("profile", "image\t[]\t[]\t{}\t{}")
	if err == nil {
		t.Fatal("invalid host aliases accepted")
	}
}
