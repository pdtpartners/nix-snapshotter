package nix

import (
	"context"
	"testing"
)

func TestGetNixStorePath(t *testing.T) {
	ctx := context.Background()

	// Single arch ref
	ref := "nix:0/nix/store/dbc4mhv2fjbfx8pypx88qgp8nfp392az-nix-image-nginx.tar:latest"
	expected := "/nix/store/dbc4mhv2fjbfx8pypx88qgp8nfp392az-nix-image-nginx.tar"
	received := getNixStorePath(ctx, ref, "x86_64-linux")
	if received != expected {
		t.Fatalf("Expected %s, received %s", expected, received)
	}

	// Multi arch ref
	ref = "nix:0/multiarch/aarch64-linux/nix/store/zkw3cjabs8lc8bv4sgnm6x132gm956fc-nix-image-nginx.tar/x86_64-linux/nix/store/dbc4mhv2fjbfx8pypx88qgp8nfp392az-nix-image-nginx.tar:latest"

	expected = "/nix/store/dbc4mhv2fjbfx8pypx88qgp8nfp392az-nix-image-nginx.tar"
	received = getNixStorePath(ctx, ref, "x86_64-linux")
	if received != expected {
		t.Fatalf("Expected %s, received %s", expected, received)
	}

	expected = "/nix/store/zkw3cjabs8lc8bv4sgnm6x132gm956fc-nix-image-nginx.tar"
	received = getNixStorePath(ctx, ref, "aarch64-linux")
	if received != expected {
		t.Fatalf("Expected %s, received %s", expected, received)
	}
}
