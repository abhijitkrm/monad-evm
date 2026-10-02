// Package engine is the consensus-engine seam for the Cosmos EVM server
// stack.
//
// Consumers of the running engine (json-rpc backend, indexer, mempool,
// gRPC services) already go through the CometBFT rpcclient.Client interface
// and the EventBus pub/sub — those are the surfaces every engine must
// provide. An Engine implementation starts the in-process consensus layer
// for an ABCI app and exposes both.
//
// Backends self-register via Register so this package never imports a
// concrete engine: cosmos-evm's server only blank-imports the backends it
// ships (engine/comet), and external engines (e.g. monadbft) register from
// their own module.
package engine

import (
	"fmt"
	"sort"

	cmtcfg "github.com/cometbft/cometbft/config"
	cmtlog "github.com/cometbft/cometbft/libs/log"
	"github.com/cometbft/cometbft/node"
	"github.com/cometbft/cometbft/p2p"
	rpcclient "github.com/cometbft/cometbft/rpc/client"
	cmttypes "github.com/cometbft/cometbft/types"

	srvtypes "github.com/cosmos/cosmos-sdk/server/types"
)

// Engine is a running in-process consensus engine serving an ABCI app.
type Engine interface {
	// Client returns the in-process engine RPC client. CometBFT result
	// types (coretypes.ResultBlock, ResultBlockResults, ResultEvent…) are
	// the transitional protocol every consumer already speaks; engines
	// synthesize them.
	Client() rpcclient.Client

	// EventBus feeds subscribers such as the app mempool's
	// NotifyNewBlock path. Engines without a native comet bus must publish
	// the same cmttypes events (EventDataNewBlock/Header/Tx) on a bus they
	// own so subscribers work unchanged.
	EventBus() *cmttypes.EventBus

	IsRunning() bool
	Stop() error
}

// Options carries everything server/start.go hands to engine construction.
type Options struct {
	Config          *cmtcfg.Config
	PrivValidator   cmttypes.PrivValidator
	NodeKey         *p2p.NodeKey
	App             srvtypes.Application
	GenDocProvider  node.GenesisDocProvider
	DBProvider      cmtcfg.DBProvider
	MetricsProvider node.MetricsProvider
	Logger          cmtlog.Logger
}

// Starter constructs and starts an Engine. Implementations register
// themselves under a kind name (e.g. "cometbft").
type Starter func(Options) (Engine, error)

var backends = map[string]Starter{}

// Register installs a backend. It panics on duplicate kinds — registration
// happens at init time, so a duplicate is a build-time integration bug.
func Register(kind string, s Starter) {
	if _, dup := backends[kind]; dup {
		panic(fmt.Sprintf("engine backend %q registered twice", kind))
	}
	backends[kind] = s
}

// Registered returns the sorted list of known backend kinds.
func Registered() []string {
	kinds := make([]string, 0, len(backends))
	for k := range backends {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	return kinds
}

// Start launches the backend named by kind.
func Start(kind string, opts Options) (Engine, error) {
	s, ok := backends[kind]
	if !ok {
		return nil, fmt.Errorf("unknown consensus engine %q (registered: %v)", kind, Registered())
	}
	return s(opts)
}
