package runstate

import (
	"context"
	"testing"
)

func TestInMemoryStorePutAndGet(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	in := Run{RunID: "r1", Goal: "g1", Status: StatusQueued}
	if err := store.Put(ctx, in); err != nil {
		t.Fatalf("put: %v", err)
	}

	got, ok, err := store.Get(ctx, "r1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !ok {
		t.Fatalf("expected run to exist")
	}
	if got.RunID != "r1" || got.Status != StatusQueued {
		t.Fatalf("unexpected run: %+v", got)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Fatalf("timestamps should be set")
	}
}
