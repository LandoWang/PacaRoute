package pacaroute

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"
)

// A request waiting for another request's credential exchange on the same lease
// gate must return as soon as its own context is cancelled.
func TestLeaseGateHonorsCancellation(t *testing.T) {
	store := storeForTest(t)
	auth := NewAuth(store, "")
	if err := auth.vault.write(auth.vault.githubPath("copilot"), credential{Version: 1, AccessToken: "audit-synthetic", CredentialID: "audit"}); err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	var enteredOnce sync.Once
	auth.http.values = map[string]*http.Client{"system:": syntheticAuthClient(func(r *http.Request) string {
		enteredOnce.Do(func() { close(entered) })
		<-release
		return fmt.Sprintf(`{"token":"audit-synthetic-exchange","expires_at":%d,"endpoints":{"api":"https://api.githubcopilot.com"}}`, time.Now().Add(time.Hour).Unix())
	})}
	config := store.Snapshot()
	source, _ := config.Source("copilot")
	first := make(chan error, 1)
	go func() { _, err := auth.Lease(context.Background(), config, source, false); first <- err }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("exchange did not start")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	second := make(chan error, 1)
	started := make(chan struct{})
	go func() { close(started); _, err := auth.Lease(ctx, config, source, false); second <- err }()
	<-started
	cancel()
	returned := false
	select {
	case <-second:
		returned = true
	case <-time.After(250 * time.Millisecond):
		t.Error("canceled lease waiter still blocked behind unrelated request's exchange after 250ms")
	}
	unblock()
	select {
	case err := <-first:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("first lease stuck")
	}
	if !returned {
		select {
		case <-second:
		case <-time.After(3 * time.Second):
			t.Fatal("lease waiter stuck after gate release")
		}
	}
}
