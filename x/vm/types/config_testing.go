//
// The config package provides a convenient way to modify x/evm params and values.
// Its primary purpose is to be used during application initialization.

//go:build test

package types

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"

	"github.com/ethereum/go-ethereum/core/vm"
	geth "github.com/ethereum/go-ethereum/params"
)

// testChainConfig is the chain configuration used in the EVM to defined which
// opcodes are active based on Ethereum upgrades.
var testChainConfig *ChainConfig

// testChainConfigMu protects concurrent access to testChainConfig
var testChainConfigMu sync.RWMutex

// eipExtMu serializes EVM-table mutation; eipExtGen counts ResetTestConfig
// calls so a real reset forces re-application of the recorded extensions.
var (
	eipExtMu            sync.Mutex
	eipExtGen           uint64
	eipExtAppliedAtGen  uint64
	appliedDefaultEIPs  []int64
	appliedActivatorIDs []int
)

// Configure applies the changes to the virtual machine configuration.
func (ec *EVMConfigurator) Configure() error {
	// If Configure method has been already used in the object, return
	// an error to avoid overriding configuration.
	if ec.sealed {
		return fmt.Errorf("error configuring EVMConfigurator: already sealed and cannot be modified")
	}

	if err := setTestingEVMCoinInfo(ec.evmCoinInfo); err != nil {
		return err
	}

	if err := applyTestEIPExtensions(ec.extendedDefaultExtraEIPs, ec.extendedEIPs); err != nil {
		return err
	}

	// After applying modifications, the configurator is sealed. This way, it is not possible
	// to call the configure method twice.
	ec.sealed = true

	return nil
}

// applyTestEIPExtensions extends the process-global EVM tables idempotently:
// test processes hosting multiple in-process apps (multi-node devnets)
// Configure once per app over the same globals, and geth's activator table
// panics on duplicate activation. An identical re-application is a no-op;
// ResetTestConfig bumps the generation to force re-application.
func applyTestEIPExtensions(defaultExtra []int64, activators map[int]func(*vm.JumpTable)) error {
	eipExtMu.Lock()
	defer eipExtMu.Unlock()
	if eipExtAppliedAtGen == eipExtGen &&
		slices.Equal(appliedDefaultEIPs, defaultExtra) &&
		slices.Equal(appliedActivatorIDs, sortedKeys(activators)) {
		return nil
	}
	if err := extendDefaultExtraEIPs(defaultExtra); err != nil {
		return err
	}
	if err := vm.ExtendActivators(activators); err != nil {
		return err
	}
	appliedDefaultEIPs = slices.Clone(defaultExtra)
	appliedActivatorIDs = sortedKeys(activators)
	eipExtAppliedAtGen = eipExtGen
	return nil
}

func sortedKeys(m map[int]func(*vm.JumpTable)) []int {
	keys := slices.Collect(maps.Keys(m))
	slices.Sort(keys)
	return keys
}

func (ec *EVMConfigurator) ResetTestConfig() {
	vm.ResetActivators()
	resetEVMCoinInfo()
	eipExtMu.Lock()
	eipExtGen++
	eipExtMu.Unlock()
	testChainConfigMu.Lock()
	testChainConfig = nil
	testChainConfigMu.Unlock()
}

func setTestChainConfig(cc *ChainConfig) error {
	testChainConfigMu.Lock()
	defer testChainConfigMu.Unlock()

	if testChainConfig != nil {
		return errors.New("chainConfig already set. Cannot set again the chainConfig. Call the configurators ResetTestConfig method before configuring a new chain.")
	}
	config := DefaultChainConfig(0)
	if cc != nil {
		config = cc
	}
	if err := config.Validate(); err != nil {
		return err
	}
	testChainConfig = config
	return nil
}

// SetChainConfig allows to set the `chainConfig` variable modifying the
// default values. The method is private because it should only be called once
// in the EVMConfigurator.
func SetChainConfig(cc *ChainConfig) error {
	testChainConfigMu.Lock()
	defer testChainConfigMu.Unlock()

	if chainConfig != nil && chainConfig.ChainId != DefaultEVMChainID {
		return errors.New("chainConfig already set. Cannot set again the chainConfig")
	}
	config := DefaultChainConfig(0)
	if cc != nil {
		config = cc
	}
	if err := config.Validate(); err != nil {
		return err
	}
	testChainConfig = config

	return nil
}

// GetEthChainConfig returns the `chainConfig` used in the EVM (geth type).
func GetEthChainConfig() *geth.ChainConfig {
	testChainConfigMu.RLock()
	defer testChainConfigMu.RUnlock()
	return testChainConfig.EthereumConfig(nil)
}

// GetChainConfig returns the `chainConfig`.
func GetChainConfig() *ChainConfig {
	testChainConfigMu.RLock()
	defer testChainConfigMu.RUnlock()
	return testChainConfig
}
