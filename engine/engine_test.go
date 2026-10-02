package engine

import (
	"errors"
	"testing"

	rpcclient "github.com/cometbft/cometbft/rpc/client"
	cmttypes "github.com/cometbft/cometbft/types"
)

type fakeEngine struct{ stopped bool }

func (f *fakeEngine) Client() rpcclient.Client     { return nil }
func (f *fakeEngine) EventBus() *cmttypes.EventBus { return nil }
func (f *fakeEngine) IsRunning() bool              { return !f.stopped }
func (f *fakeEngine) Stop() error                  { f.stopped = true; return nil }

func TestRegistry(t *testing.T) {
	fe := &fakeEngine{}
	Register("fake", func(Options) (Engine, error) { return fe, nil })

	eng, err := Start("fake", Options{})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if eng != fe {
		t.Fatal("Start did not return the registered engine")
	}
	if !eng.IsRunning() {
		t.Fatal("engine should be running")
	}
	if err := eng.Stop(); err != nil || eng.IsRunning() {
		t.Fatalf("Stop: err=%v running=%v", err, eng.IsRunning())
	}

	if _, err := Start("nope", Options{}); err == nil {
		t.Fatal("expected error for unregistered engine kind")
	}
}

func TestRegisterDuplicatePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on duplicate registration")
		}
	}()
	noop := func(Options) (Engine, error) { return nil, errors.New("unused") }
	Register("dup", noop)
	Register("dup", noop)
}
