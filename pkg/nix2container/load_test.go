package nix2container

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/containerd/containerd/v2/core/transfer"
)

func TestSavedImageNameWaitsForLateEvent(t *testing.T) {
	saved := newSavedImageName()
	go func() {
		// Deliver the event after wait has started, as containerd's progress
		// goroutine can after Transfer has returned.
		time.Sleep(50 * time.Millisecond)
		saved.progress(transfer.Progress{Event: "saving"})
		saved.progress(transfer.Progress{Event: "saved", Name: "nix:0/nix/store/abc-image.tar:latest"})
	}()

	name, err := saved.wait(context.Background(), 5*time.Second)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if name != "nix:0/nix/store/abc-image.tar:latest" {
		t.Fatalf("name = %q", name)
	}
}

func TestSavedImageNameKeepsFirstSavedEvent(t *testing.T) {
	saved := newSavedImageName()
	saved.progress(transfer.Progress{Event: "saved", Name: "first"})
	saved.progress(transfer.Progress{Event: "saved", Name: "second"})

	name, err := saved.wait(context.Background(), time.Second)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if name != "first" {
		t.Fatalf("name = %q, want first", name)
	}
}

func TestSavedImageNameTimesOutWithoutSavedEvent(t *testing.T) {
	saved := newSavedImageName()
	saved.progress(transfer.Progress{Event: "saving", Name: "ignored"})

	name, err := saved.wait(context.Background(), 20*time.Millisecond)
	if err == nil {
		t.Fatalf("wait returned %q without a saved event", name)
	}
}

func TestSavedImageNameStopsOnCancel(t *testing.T) {
	saved := newSavedImageName()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := saved.wait(ctx, time.Second); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}
