package evmd

import (
	evmmempool "github.com/cosmos/evm/mempool"
	"github.com/cosmos/evm/server"
	evmtypes "github.com/cosmos/evm/x/vm/types"

	"cosmossdk.io/log/v2"

	"github.com/cosmos/cosmos-sdk/baseapp"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// configureEVMMempool sets up the EVM mempool and related handlers using viper configuration.
func (app *EVMD) configureEVMMempool(appOpts servertypes.AppOptions, logger log.Logger) error {
	if evmtypes.GetChainConfig() == nil {
		logger.Debug("evm chain config is not set, skipping mempool configuration")
		return nil
	}

	var (
		mpConfig = server.ResolveMempoolConfig(app.GetAnteHandler(), appOpts, logger)

		txEncoder       = evmmempool.NewTxEncoder(app.txConfig)
		evmRechecker    = evmmempool.NewTxRechecker(mpConfig.AnteHandler, txEncoder)
		cosmosRechecker = evmmempool.NewTxRechecker(mpConfig.AnteHandler, txEncoder)
		cosmosPoolMaxTx = server.GetCosmosPoolMaxTx(appOpts, logger)
		checkTxTimeout  = server.GetMempoolCheckTxTimeout(appOpts, logger)
	)

	if cosmosPoolMaxTx < 0 {
		logger.Debug("evm mempool is disabled, skipping configuration")
		return nil
	}

	if err := server.ValidateReapBounds(appOpts, mpConfig.BlockGasLimit); err != nil {
		return err
	}

	// create mempool — route ctx creation through SpecLock so a rechecker
	// context can't race the monadbft bridge's speculative-execution
	// rollback (RollbackToVersion deletes versions; LatestVersion can
	// briefly read ahead of the materialized IAVL trees).
	mempool := evmmempool.NewMempool(
		func(height int64, prove bool) (sdk.Context, error) {
			app.SpecLock().Lock()
			defer app.SpecLock().Unlock()
			return app.CreateQueryContext(height, prove)
		},
		logger,
		app.EVMKeeper,
		app.FeeMarketKeeper,
		app.txConfig,
		evmRechecker,
		cosmosRechecker,
		mpConfig,
		cosmosPoolMaxTx,
	)

	app.EVMMempool = mempool

	// create ABCI handlers
	proposalHandler := baseapp.NewDefaultProposalHandler(mempool, NewNoCheckProposalTxVerifier(app.BaseApp))

	insertTxHandler := mempool.NewInsertTxHandler(app.TxDecode)
	reapTxsHandler := mempool.NewReapTxsHandler()
	checkTxHandler := mempool.NewCheckTxHandler(app.TxDecode, checkTxTimeout)

	// set handlers and the mempool
	app.SetPrepareProposal(proposalHandler.PrepareProposalHandler())
	app.SetProcessProposal(proposalHandler.ProcessProposalHandler())
	app.SetInsertTxHandler(insertTxHandler)
	app.SetReapTxsHandler(reapTxsHandler)
	app.SetCheckTxHandler(checkTxHandler)

	app.SetMempool(mempool)

	app.SetPrepareCheckStater(func(_ sdk.Context) {
		if !mempool.HasEventBus() {
			mempool.NotifyNewBlock()
		}
	})

	return nil
}
