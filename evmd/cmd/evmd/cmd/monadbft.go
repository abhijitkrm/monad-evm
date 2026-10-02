package cmd

// Registers the MonadBFT consensus backend with the engine factory —
// `evmd start --engine=monadbft` (or `engine = "monadbft"` in config)
// runs pipelined MonadBFT in-process instead of CometBFT. The backend
// itself lives in the monadbft-go bridge module; blank-importing it here
// runs its init() → engine.Register(bridge.Kind, bridge.Start).
import _ "github.com/abhijitkrm/monadbft-go/bridge"
