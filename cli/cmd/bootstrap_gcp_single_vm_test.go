// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import "testing"

func TestBootstrapGcpSingleVMCommandIsSeparate(t *testing.T) {
	root := GetRootCmd()

	regular, _, err := root.Find([]string{"beta", "bootstrap-gcp"})
	if err != nil {
		t.Fatal(err)
	}

	single, _, err := root.Find([]string{"beta", "bootstrap-gcp-single-vm"})
	if err != nil {
		t.Fatal(err)
	}

	if regular == single || single.Name() != "bootstrap-gcp-single-vm" {
		t.Fatalf("single VM command not registered separately")
	}

	if single.Flags().Lookup("install-local") == nil || single.Flags().Lookup("machine-type") == nil {
		t.Fatalf("single VM flags missing")
	}

	if single.Flags().Lookup("install-version") == nil {
		t.Fatal("single VM install-version flag missing")
	}
	if flag := single.Flags().Lookup("storage-engine"); flag == nil || flag.DefValue != "rook-ceph" {
		t.Fatal("single VM storage-engine flag missing or has wrong default")
	}
	if single.Flags().Lookup("install-config-template") == nil {
		t.Fatal("single VM install-config-template flag missing")
	}
	if single.Flags().Lookup("config") == nil {
		t.Fatal("single VM config flag missing")
	}
	if single.Flags().Lookup("vault") == nil || single.Flags().Lookup("priv-key") == nil {
		t.Fatal("single VM vault flags missing")
	}

	local, _, err := root.Find([]string{"beta", "bootstrap-local"})
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"workspace-hosting-base-domain", "public-ip", "gateway-ip", "public-gateway-ip", "ssh-proxy-ip"} {
		if local.Flags().Lookup(name) != nil {
			t.Fatalf("single VM setting %q must be supplied through --install-config", name)
		}
	}
}

func TestValidateLocalStorageEngine(t *testing.T) {
	for _, engine := range []string{"rook-ceph", "local"} {
		if err := validateLocalStorageEngine(engine); err != nil {
			t.Fatalf("engine %q: %v", engine, err)
		}
	}
	if err := validateLocalStorageEngine("unknown"); err == nil {
		t.Fatal("expected unsupported storage engine error")
	}
}
