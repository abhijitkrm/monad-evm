// Package comet is the CometBFT backend for the engine seam — it wraps the
// exact construction server/start.go has always done (node.NewNode +
// in-process local client), so running with Kind = "cometbft" is a
// zero-behavior-change path.
package comet

import (
	cmttypes "github.com/cometbft/cometbft/types"

	"github.com/cometbft/cometbft/node"
	"github.com/cometbft/cometbft/proxy"
	rpcclient "github.com/cometbft/cometbft/rpc/client"
	"github.com/cometbft/cometbft/rpc/client/local"

	"github.com/cosmos/cosmos-sdk/server"

	"github.com/cosmos/evm/engine"
)

// Kind is the registered backend name.
const Kind = "cometbft"

func init() { engine.Register(Kind, Start) }

type cometEngine struct {
	node   *node.Node
	client rpcclient.Client
}

var _ engine.Engine = (*cometEngine)(nil)

// Start builds and starts an in-process CometBFT node over the app.
func Start(opts engine.Options) (engine.Engine, error) {
	cmtApp := server.NewCometABCIWrapper(opts.App)
	n, err := node.NewNode(
		opts.Config,
		opts.PrivValidator,
		opts.NodeKey,
		proxy.NewLocalClientCreator(cmtApp),
		opts.GenDocProvider,
		opts.DBProvider,
		opts.MetricsProvider,
		opts.Logger,
	)
	if err != nil {
		return nil, err
	}
	if err := n.Start(); err != nil {
		return nil, err
	}
	return &cometEngine{node: n, client: local.New(n)}, nil
}

func (e *cometEngine) Client() rpcclient.Client     { return e.client }
func (e *cometEngine) EventBus() *cmttypes.EventBus { return e.node.EventBus() }
func (e *cometEngine) IsRunning() bool              { return e.node.IsRunning() }
func (e *cometEngine) Stop() error                  { return e.node.Stop() }
