package evmd

import (
	"crypto/sha256"
	"math/big"
	"os"
	"runtime"
	"sync"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"

	ethtypes "github.com/ethereum/go-ethereum/core/types"

	evmtypes "github.com/cosmos/evm/x/vm/types"
)

// parallelTxDecoder wraps the app's tx decoder. FinalizeBlock receives the
// block's entire tx list up front, so decode+signature-recovery — the
// dominant per-tx serial cost — is pre-computed across cores into a cache;
// the serial runTx loop then hits the cache instead of doing ecrecover
// inline. Decode+recovery is a pure function of the tx bytes (given the
// block's signer), so consensus semantics are unchanged: identical bytes →
// identical decoded tx, and ante still performs the authoritative
// signature check.
type parallelTxDecoder struct {
	inner sdk.TxDecoder
	cache sync.Map // [32]byte(sha256(txBz)) -> decodeResult
}

type decodeResult struct {
	tx  sdk.Tx
	err error
}

// Decode — sdk.TxDecoder. Returns the pre-decoded result when the block
// pre-pass already covered these bytes (consumed exactly once).
func (d *parallelTxDecoder) Decode(txBz []byte) (sdk.Tx, error) {
	k := sha256.Sum256(txBz)
	if v, ok := d.cache.LoadAndDelete(k); ok {
		r := v.(decodeResult)
		return r.tx, r.err
	}
	return d.inner(txBz)
}

// preDecode decodes txs concurrently into the cache and warms each eth
// transaction's sender cache (ethtypes.Sender memoizes on the decoded
// object; MakeSigner produces a value-identical signer for the same
// height/time, so the ante handler's VerifySender hits the warm cache and
// skips ecrecover). Single-consumer via LoadAndDelete keeps memory
// block-bounded; duplicate tx bytes decode once and re-decode fresh on the
// second lookup (same result).
func (d *parallelTxDecoder) preDecode(txs [][]byte, height int64, blockTime time.Time) {
	n := len(txs)
	if n < 8 {
		return // not worth the fan-out on small blocks
	}
	signer := ethtypes.MakeSigner(evmtypes.GetEthChainConfig(), big.NewInt(height), uint64(blockTime.Unix())) //#nosec G115 -- block time always > 0
	workers := runtime.GOMAXPROCS(0)
	if workers > n {
		workers = n
	}
	jobs := make(chan int, n)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				k := sha256.Sum256(txs[i])
				if _, dup := d.cache.Load(k); dup {
					continue
				}
				tx, err := d.inner(txs[i])
				if err == nil {
					for _, msg := range tx.GetMsgs() {
						if ethMsg, ok := msg.(*evmtypes.MsgEthereumTx); ok {
							// best-effort warm-up only — failures are
							// re-checked authoritatively by the ante handler
							_ = ethMsg.VerifySender(signer)
						}
					}
				}
				d.cache.Store(k, decodeResult{tx, err})
			}
		}()
	}
	for i := range txs {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
}

func init() {
	// sdk.GetConfig() resolves its registry key via os.Hostname() +
	// os.Executable() on EVERY call, guarded by a global mutex — that is a
	// sysctl syscall + lock on every AccAddress.String() inside account
	// lookups, executed per worker under BlockSTM. The default key is
	// already process-constant, so pinning the scope preserves the exact
	// same config-sharing semantics while skipping the per-call syscall.
	if os.Getenv(sdk.EnvConfigScope) == "" {
		_ = os.Setenv(sdk.EnvConfigScope, "evmd")
	}
}
