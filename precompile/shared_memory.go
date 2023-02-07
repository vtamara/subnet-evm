// (c) 2023, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package precompile

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/ava-labs/avalanchego/chains/atomic"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow"
	"github.com/ava-labs/avalanchego/vms/components/avax"
	"github.com/ava-labs/avalanchego/vms/components/verify"
	"github.com/ava-labs/avalanchego/vms/secp256k1fx"
	"github.com/ava-labs/subnet-evm/accounts/abi"
	"github.com/ava-labs/subnet-evm/utils/codec"
	"github.com/ava-labs/subnet-evm/vmerrs"

	"github.com/ethereum/go-ethereum/common"
)

// XXX
const (
	ExportAVAXGasCost            uint64 = writeGasCostPerSlot + readGasCostPerSlot
	ExportUTXOGasCost            uint64 = writeGasCostPerSlot + readGasCostPerSlot
	ImportAVAXGasCost            uint64 = writeGasCostPerSlot + readGasCostPerSlot
	ImportUTXOGasCost            uint64 = writeGasCostPerSlot + readGasCostPerSlot
	GetNativeTokenAssetIDGasCost uint64 = 100 // Based off of sha256

	// SharedMemoryRawABI contains the raw ABI of SharedMemory contract.
	SharedMemoryRawABI = "[{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"uint64\",\"name\":\"amount\",\"type\":\"uint64\"},{\"indexed\":true,\"internalType\":\"bytes32\",\"name\":\"destinationChainID\",\"type\":\"bytes32\"},{\"indexed\":false,\"internalType\":\"uint64\",\"name\":\"locktime\",\"type\":\"uint64\"},{\"indexed\":false,\"internalType\":\"uint64\",\"name\":\"threshold\",\"type\":\"uint64\"},{\"indexed\":false,\"internalType\":\"address[]\",\"name\":\"addrs\",\"type\":\"address[]\"}],\"name\":\"ExportAVAX\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"uint64\",\"name\":\"amount\",\"type\":\"uint64\"},{\"indexed\":true,\"internalType\":\"bytes32\",\"name\":\"destinationChainID\",\"type\":\"bytes32\"},{\"indexed\":true,\"internalType\":\"bytes32\",\"name\":\"assetID\",\"type\":\"bytes32\"},{\"indexed\":false,\"internalType\":\"uint64\",\"name\":\"locktime\",\"type\":\"uint64\"},{\"indexed\":false,\"internalType\":\"uint64\",\"name\":\"threshold\",\"type\":\"uint64\"},{\"indexed\":false,\"internalType\":\"address[]\",\"name\":\"addrs\",\"type\":\"address[]\"}],\"name\":\"ExportUTXO\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"uint64\",\"name\":\"amount\",\"type\":\"uint64\"},{\"indexed\":true,\"internalType\":\"bytes32\",\"name\":\"sourceChainID\",\"type\":\"bytes32\"},{\"indexed\":false,\"internalType\":\"uint64\",\"name\":\"locktime\",\"type\":\"uint64\"},{\"indexed\":false,\"internalType\":\"uint64\",\"name\":\"threshold\",\"type\":\"uint64\"},{\"indexed\":false,\"internalType\":\"address[]\",\"name\":\"addrs\",\"type\":\"address[]\"}],\"name\":\"ImportAVAX\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"uint64\",\"name\":\"amount\",\"type\":\"uint64\"},{\"indexed\":true,\"internalType\":\"bytes32\",\"name\":\"sourceChainID\",\"type\":\"bytes32\"},{\"indexed\":true,\"internalType\":\"bytes32\",\"name\":\"assetID\",\"type\":\"bytes32\"},{\"indexed\":false,\"internalType\":\"uint64\",\"name\":\"locktime\",\"type\":\"uint64\"},{\"indexed\":false,\"internalType\":\"uint64\",\"name\":\"threshold\",\"type\":\"uint64\"},{\"indexed\":false,\"internalType\":\"address[]\",\"name\":\"addrs\",\"type\":\"address[]\"}],\"name\":\"ImportUTXO\",\"type\":\"event\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"destinationChainID\",\"type\":\"bytes32\"},{\"internalType\":\"uint64\",\"name\":\"locktime\",\"type\":\"uint64\"},{\"internalType\":\"uint64\",\"name\":\"threshold\",\"type\":\"uint64\"},{\"internalType\":\"address[]\",\"name\":\"addrs\",\"type\":\"address[]\"}],\"name\":\"exportAVAX\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint64\",\"name\":\"amount\",\"type\":\"uint64\"},{\"internalType\":\"bytes32\",\"name\":\"desinationChainID\",\"type\":\"bytes32\"},{\"internalType\":\"uint64\",\"name\":\"locktime\",\"type\":\"uint64\"},{\"internalType\":\"uint64\",\"name\":\"threshold\",\"type\":\"uint64\"},{\"internalType\":\"address[]\",\"name\":\"addrs\",\"type\":\"address[]\"}],\"name\":\"exportUTXO\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"caller\",\"type\":\"address\"}],\"name\":\"getNativeTokenAssetID\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"assetID\",\"type\":\"bytes32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"sourceChain\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"utxoID\",\"type\":\"bytes32\"}],\"name\":\"importAVAX\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"sourceChain\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"utxoID\",\"type\":\"bytes32\"}],\"name\":\"importUTXO\",\"outputs\":[{\"internalType\":\"uint64\",\"name\":\"amount\",\"type\":\"uint64\"},{\"internalType\":\"uint64\",\"name\":\"locktime\",\"type\":\"uint64\"},{\"internalType\":\"uint64\",\"name\":\"threshold\",\"type\":\"uint64\"},{\"internalType\":\"address[]\",\"name\":\"addrs\",\"type\":\"address[]\"}],\"stateMutability\":\"nonpayable\",\"type\":\"function\"}]"
)

// Singleton StatefulPrecompiledContract and signatures.
var (
	_ StatefulPrecompileConfig = &SharedMemoryConfig{}

	SharedMemoryABI abi.ABI // will be initialized by init function

	SharedMemoryPrecompile StatefulPrecompiledContract // will be initialized by init function
)

// SharedMemoryConfig implements the StatefulPrecompileConfig
// interface while adding in the SharedMemory specific precompile address.
type SharedMemoryConfig struct {
	UpgradeableConfig
}

type ExportAVAXInput struct {
	DestinationChainID [32]byte
	Locktime           uint64
	Threshold          uint64
	Addrs              []common.Address
}

type ExportUTXOInput struct {
	Amount            uint64
	DesinationChainID [32]byte
	Locktime          uint64
	Threshold         uint64
	Addrs             []common.Address
}

type ImportAVAXInput struct {
	SourceChain [32]byte
	UtxoID      [32]byte
}

type ImportUTXOInput struct {
	SourceChain [32]byte
	UtxoID      [32]byte
}

type ImportUTXOOutput struct {
	Amount    uint64
	Locktime  uint64
	Threshold uint64
	Addrs     []common.Address
}

func init() {
	parsed, err := abi.JSON(strings.NewReader(SharedMemoryRawABI))
	if err != nil {
		panic(err)
	}
	SharedMemoryABI = parsed

	SharedMemoryPrecompile = createSharedMemoryPrecompile(SharedMemoryAddress)
}

// NewSharedMemoryConfig returns a config for a network upgrade at [blockTimestamp] that enables
// SharedMemory .
func NewSharedMemoryConfig(blockTimestamp *big.Int) *SharedMemoryConfig {
	return &SharedMemoryConfig{
		UpgradeableConfig: UpgradeableConfig{BlockTimestamp: blockTimestamp},
	}
}

// NewDisableSharedMemoryConfig returns config for a network upgrade at [blockTimestamp]
// that disables SharedMemory.
func NewDisableSharedMemoryConfig(blockTimestamp *big.Int) *SharedMemoryConfig {
	return &SharedMemoryConfig{
		UpgradeableConfig: UpgradeableConfig{
			BlockTimestamp: blockTimestamp,
			Disable:        true,
		},
	}
}

// Equal returns true if [s] is a [*SharedMemoryConfig] and it has been configured identical to [c].
func (c *SharedMemoryConfig) Equal(s StatefulPrecompileConfig) bool {
	// typecast before comparison
	other, ok := (s).(*SharedMemoryConfig)
	if !ok {
		return false
	}

	equals := c.UpgradeableConfig.Equal(&other.UpgradeableConfig)
	return equals
}

// String returns a string representation of the SharedMemoryConfig.
func (c *SharedMemoryConfig) String() string {
	bytes, _ := json.Marshal(c)
	return string(bytes)
}

// Address returns the address of the SharedMemory precompile. Addresses reside under the precompile/params.go
func (c *SharedMemoryConfig) Address() common.Address {
	return SharedMemoryAddress
}

// Configure configures [state] with the initial configuration.
func (c *SharedMemoryConfig) Configure(_ ChainConfig, state StateDB, _ BlockContext) {
	// TODO: the configure function could be used to set up the atomic trie.
}

// Contract returns the singleton stateful precompiled contract to be used for SharedMemory.
func (c *SharedMemoryConfig) Contract() StatefulPrecompiledContract {
	return SharedMemoryPrecompile
}

// Verify tries to verify SharedMemoryConfig and returns an error accordingly.
func (c *SharedMemoryConfig) Verify() error {
	return nil
}

// Predicate optionally returns a function to enforce as a predicate for a transaction to be valid
// if the access list of the transaction includes a tuple that references the precompile address.
// Returns nil here to indicate that this precompile does not enforce a predicate.
func (c *SharedMemoryConfig) Predicate() PredicateFunc {
	return SharedMemoryPredicate
}

type AtomicPredicate struct {
	// Which chain to consume the funds from
	SourceChain ids.ID `serialize:"true" json:"sourceChain"`
	// UTXOs consumable in this transaction
	ImportedUTXOs []*avax.UTXO `serialize:"true" json:"importedInputs"`
}

// SharedMemoryPredicate verifies that the UTXOs specified by [predicateBytes] are present in shared memory, valid, and
// have not been consumed yet.
// SharedMemoryPredicate is called before the transaction execution, so it is up to the precompile implementation to validate
// that the caller has permission to consume the UTXO and how consuming the UTXO works.
func SharedMemoryPredicate(chainContext *snow.Context, predicateBytes []byte) error {
	atomicPredicate := new(AtomicPredicate)
	version, err := codec.Codec.Unmarshal(predicateBytes, atomicPredicate)
	if err != nil {
		return fmt.Errorf("failed to unmarshal shared memory predicate: %w", err)
	}
	if version != 0 {
		return fmt.Errorf("invalid version for shared memory predicate: %d", version)
	}

	utxoIDs := make([][]byte, len(atomicPredicate.ImportedUTXOs))
	for i, in := range atomicPredicate.ImportedUTXOs {
		inputID := in.UTXOID.InputID()
		utxoIDs[i] = inputID[:]
	}
	// allUTXOBytes is guaranteed to be the same length as utxoIDs
	allUTXOBytes, err := chainContext.SharedMemory.Get(atomicPredicate.SourceChain, utxoIDs)
	if err != nil {
		return fmt.Errorf("failed to fetch import UTXOs from %s due to: %w", atomicPredicate.SourceChain, err)
	}

	// TODO: check that the UTXO has not been marked as consumed within the statedb
	// XXX make sure this handles the async acceptor here

	for i, specifiedUTXO := range atomicPredicate.ImportedUTXOs {
		specifiedUTXOBytes, err := codec.Codec.Marshal(0, specifiedUTXO)
		if err != nil {
			return fmt.Errorf("failed to marshal specified UTXO: %s", specifiedUTXO.ID)
		}
		utxoBytes := allUTXOBytes[i]

		if !bytes.Equal(specifiedUTXOBytes, utxoBytes) {
			return fmt.Errorf("UTXO %s mismatching byte representation: 0x%x expected: 0x%x", specifiedUTXO.ID, specifiedUTXOBytes, utxoBytes)
		}

		// Validate that the specified UTXO is valid to be imported to the EVM
		transferOut, ok := specifiedUTXO.Out.(*secp256k1fx.TransferOutput)
		if !ok {
			return fmt.Errorf("UTXO %s invalid UTXO output type: %T", specifiedUTXO.ID, specifiedUTXO.Out)
		}
		// Do not allow a transfer amount of 0 - no VM should export an atomic UTXO with an amount of 0, so this
		// should never happen, but just in case another VM implements this incorrectly, we add an extra check here
		if transferOut.Amt == 0 {
			return fmt.Errorf("UTXO %s cannot specify an amount of 0", specifiedUTXO.ID)
		}
		// When import is called within the EVM, there will only be one caller address, so we require a threshold of 1
		// since a UTXO with a threshold of 2 will never be valid to import.
		if transferOut.Threshold != 1 {
			return fmt.Errorf("UTXO %s specified invalid threshold", specifiedUTXO.ID)
		}
	}

	return nil
}

type ExportAVAXEvent struct {
	Amount    uint64
	Locktime  uint64
	Threshold uint64
	Addrs     []common.Address
}

// OnAccept optionally returns a function to perform on any log with the precompile address.
func (c *SharedMemoryConfig) OnAccept() OnAcceptFunc {
	return ApplySharedMemoryLogs
}

// ApplySharedMemoryLogs processes the log data and returns a corresponding map atomic memory operation
func ApplySharedMemoryLogs(snowCtx *snow.Context, txHash common.Hash, logIndex int, topics []common.Hash, logData []byte) (ids.ID, *atomic.Requests, error) { // TODO update to return atomic operations, database batch to be applied atomically, and an error
	if len(topics) == 0 {
		return ids.ID{}, nil, errors.New("SharedMemory does not handle logs with 0 topics")
	}
	event, err := SharedMemoryABI.EventByID(topics[0]) // First topic is the event ID
	if err != nil {
		return ids.ID{}, nil, fmt.Errorf("shared memory accept: %w", err)
	}

	// TODO: separate this out better
	switch {
	case event.Name == "ExportAVAX":
		ev := &ExportAVAXEvent{}
		err = SharedMemoryABI.UnpackInputIntoInterface(ev, "ExportAVAX", logData)
		if err != nil {
			return ids.ID{}, nil, fmt.Errorf("failed to unpack exportAVAX event data: %w", err)
		}
		addrs := make([]ids.ShortID, 0, len(ev.Addrs))
		for _, addr := range ev.Addrs {
			addrs = append(addrs, ids.ShortID(addr))
		}
		utxo := &avax.UTXO{
			// Derive unique UTXOID from txHash and log index
			UTXOID: avax.UTXOID{
				TxID:        ids.ID(txHash),
				OutputIndex: uint32(logIndex),
			},
			Asset: avax.Asset{ID: snowCtx.AVAXAssetID},
			Out: &secp256k1fx.TransferOutput{
				Amt: ev.Amount,
				OutputOwners: secp256k1fx.OutputOwners{
					Locktime:  ev.Locktime,
					Threshold: uint32(ev.Threshold), // TODO make the actual type uint32 to correspond to this
					Addrs:     addrs,
				},
			},
		}

		utxoBytes, err := codec.Codec.Marshal(0, utxo) // XXX
		if err != nil {
			return ids.ID{}, nil, err
		}
		utxoID := utxo.InputID()
		elem := &atomic.Element{
			Key:   utxoID[:],
			Value: utxoBytes,
		}
		if out, ok := utxo.Out.(avax.Addressable); ok {
			elem.Traits = out.Addresses()
		}

		return ids.ID(topics[1]), &atomic.Requests{ // TODO unpack the topics instead of manually extracting desintationChainID here
			PutRequests: []*atomic.Element{elem},
		}, nil
	default:
		return ids.ID{}, nil, fmt.Errorf("shared memory accept unexpected log: %q", event.Name)
	}
}

// UnpackExportAVAXInput attempts to unpack [input] into the arguments for the ExportAVAXInput{}
// assumes that [input] does not include selector (omits first 4 func signature bytes)
func UnpackExportAVAXInput(input []byte) (ExportAVAXInput, error) {
	inputStruct := ExportAVAXInput{}
	err := SharedMemoryABI.UnpackInputIntoInterface(&inputStruct, "exportAVAX", input)

	return inputStruct, err
}

// PackExportAVAX packs [inputStruct] of type ExportAVAXInput into the appropriate arguments for exportAVAX.
func PackExportAVAX(destinationChainID ids.ID, locktime uint64, threshold uint64, addrs []common.Address) ([]byte, error) {
	return SharedMemoryABI.Pack("exportAVAX", destinationChainID, locktime, threshold, addrs)
}

func exportAVAX(accessibleState PrecompileAccessibleState, caller common.Address, addr common.Address, input []byte, suppliedGas uint64, readOnly bool) (ret []byte, remainingGas uint64, err error) {
	if remainingGas, err = deductGas(suppliedGas, ExportAVAXGasCost); err != nil {
		return nil, 0, err
	}
	if readOnly {
		return nil, remainingGas, vmerrs.ErrWriteProtection
	}
	inputStruct, err := UnpackExportAVAXInput(input)
	if err != nil {
		return nil, remainingGas, err
	}
	chainCtx := accessibleState.GetSnowContext()
	if err := verify.SameSubnet(context.TODO(), chainCtx, ids.ID(inputStruct.DestinationChainID)); err != nil {
		return nil, remainingGas, err
	}

	balance := accessibleState.GetStateDB().GetBalance(SharedMemoryAddress)
	accessibleState.GetStateDB().SubBalance(SharedMemoryAddress, balance)
	convertedBalance := balance.Div(balance, big.NewInt(1000000000))

	topics, data, err := SharedMemoryABI.PackEvent(
		"ExportAVAX",
		convertedBalance.Uint64(), // XXX validate this value
		inputStruct.DestinationChainID,
		inputStruct.Locktime,
		inputStruct.Threshold,
		inputStruct.Addrs,
	)
	if err != nil {
		return nil, remainingGas, err
	}
	accessibleState.GetStateDB().AddLog(SharedMemoryAddress, topics, data, accessibleState.GetBlockContext().Number().Uint64())

	// TODO: add atomic trie handling if we are going to keep it inside of the storage trie

	// Return an empty output and the remaining gas
	return []byte{}, remainingGas, nil
}

// UnpackExportUTXOInput attempts to unpack [input] into the arguments for the ExportUTXOInput{}
// assumes that [input] does not include selector (omits first 4 func signature bytes)
func UnpackExportUTXOInput(input []byte) (ExportUTXOInput, error) {
	inputStruct := ExportUTXOInput{}
	err := SharedMemoryABI.UnpackInputIntoInterface(&inputStruct, "exportUTXO", input)

	return inputStruct, err
}

// PackExportUTXO packs [inputStruct] of type ExportUTXOInput into the appropriate arguments for exportUTXO.
func PackExportUTXO(inputStruct ExportUTXOInput) ([]byte, error) {
	return SharedMemoryABI.Pack("exportUTXO", inputStruct.Amount, inputStruct.DesinationChainID, inputStruct.Locktime, inputStruct.Threshold, inputStruct.Addrs)
}

func exportUTXO(accessibleState PrecompileAccessibleState, caller common.Address, addr common.Address, input []byte, suppliedGas uint64, readOnly bool) (ret []byte, remainingGas uint64, err error) {
	if remainingGas, err = deductGas(suppliedGas, ExportUTXOGasCost); err != nil {
		return nil, 0, err
	}
	if readOnly {
		return nil, remainingGas, vmerrs.ErrWriteProtection
	}
	// attempts to unpack [input] into the arguments to the ExportUTXOInput.
	// Assumes that [input] does not include selector
	// You can use unpacked [inputStruct] variable in your code
	inputStruct, err := UnpackExportUTXOInput(input)
	if err != nil {
		return nil, remainingGas, err
	}

	chainCtx := accessibleState.GetSnowContext()
	// TODO fix typo DesinationChainID -> DestinationChainID in original solidity interface and re-generate
	if err := verify.SameSubnet(context.TODO(), chainCtx, ids.ID(inputStruct.DesinationChainID)); err != nil {
		return nil, remainingGas, err
	}
	assetID := CalculateANTAssetID(common.Hash(chainCtx.ChainID), caller)

	topics, data, err := SharedMemoryABI.PackEvent(
		"ExportUTXO",
		inputStruct.Amount,
		inputStruct.DesinationChainID,
		assetID,
		inputStruct.Locktime,
		inputStruct.Threshold,
		inputStruct.Addrs,
	)
	if err != nil {
		return nil, remainingGas, err
	}
	accessibleState.GetStateDB().AddLog(SharedMemoryAddress, topics, data, accessibleState.GetBlockContext().Number().Uint64())

	// TODO: add atomic trie handling if we are going to keep it inside of the storage trie

	// Return an empty output and the remaining gas
	return []byte{}, remainingGas, nil
}

// UnpackGetNativeTokenAssetIDInput attempts to unpack [input] into the common.Address type argument
// assumes that [input] does not include selector (omits first 4 func signature bytes)
func UnpackGetNativeTokenAssetIDInput(input []byte) (common.Address, error) {
	res, err := SharedMemoryABI.UnpackInput("getNativeTokenAssetID", input)
	if err != nil {
		return common.Address{}, err
	}
	unpacked := *abi.ConvertType(res[0], new(common.Address)).(*common.Address)
	return unpacked, nil
}

// PackGetNativeTokenAssetID packs [caller] of type common.Address into the appropriate arguments for getNativeTokenAssetID.
// the packed bytes include selector (first 4 func signature bytes).
// This function is mostly used for tests.
func PackGetNativeTokenAssetID(caller common.Address) ([]byte, error) {
	return SharedMemoryABI.Pack("getNativeTokenAssetID", caller)
}

// PackGetNativeTokenAssetIDOutput attempts to pack given assetID of type [32]byte
// to conform the ABI outputs.
func PackGetNativeTokenAssetIDOutput(assetID [32]byte) ([]byte, error) {
	return SharedMemoryABI.PackOutput("getNativeTokenAssetID", assetID)
}

func getNativeTokenAssetID(accessibleState PrecompileAccessibleState, caller common.Address, addr common.Address, input []byte, suppliedGas uint64, readOnly bool) (ret []byte, remainingGas uint64, err error) {
	if remainingGas, err = deductGas(suppliedGas, GetNativeTokenAssetIDGasCost); err != nil {
		return nil, 0, err
	}
	// attempts to unpack [input] into the arguments to the GetNativeTokenAssetIDInput.
	// Assumes that [input] does not include selector
	// You can use unpacked [inputStruct] variable in your code
	address, err := UnpackGetNativeTokenAssetIDInput(input)
	if err != nil {
		return nil, remainingGas, err
	}

	assetID := CalculateANTAssetID(common.Hash(accessibleState.GetSnowContext().ChainID), address)
	packedOutput, err := PackGetNativeTokenAssetIDOutput(assetID)
	if err != nil {
		return nil, remainingGas, err
	}

	// Return the packed output and the remaining gas
	return packedOutput, remainingGas, nil
}

// UnpackImportAVAXInput attempts to unpack [input] into the arguments for the ImportAVAXInput{}
// assumes that [input] does not include selector (omits first 4 func signature bytes)
func UnpackImportAVAXInput(input []byte) (ImportAVAXInput, error) {
	inputStruct := ImportAVAXInput{}
	err := SharedMemoryABI.UnpackInputIntoInterface(&inputStruct, "importAVAX", input)

	return inputStruct, err
}

// PackImportAVAX packs [inputStruct] of type ImportAVAXInput into the appropriate arguments for importAVAX.
func PackImportAVAX(inputStruct ImportAVAXInput) ([]byte, error) {
	return SharedMemoryABI.Pack("importAVAX", inputStruct.SourceChain, inputStruct.UtxoID)
}

func importAVAX(accessibleState PrecompileAccessibleState, caller common.Address, addr common.Address, input []byte, suppliedGas uint64, readOnly bool) (ret []byte, remainingGas uint64, err error) {
	if remainingGas, err = deductGas(suppliedGas, ImportAVAXGasCost); err != nil {
		return nil, 0, err
	}
	if readOnly {
		return nil, remainingGas, vmerrs.ErrWriteProtection
	}
	// attempts to unpack [input] into the arguments to the ImportAVAXInput.
	// Assumes that [input] does not include selector
	// You can use unpacked [inputStruct] variable in your code
	inputStruct, err := UnpackImportAVAXInput(input)
	if err != nil {
		return nil, remainingGas, err
	}

	predicateBytes, exists := accessibleState.GetStateDB().GetPredicateStorageSlots(SharedMemoryAddress)
	if !exists {
		return nil, remainingGas, fmt.Errorf("no predicate available to import, caller %s, input: 0x%x, suppliedGas: %d", caller, input, suppliedGas)
	}

	atomicPredicate := new(AtomicPredicate)
	_, err = codec.Codec.Unmarshal(predicateBytes, atomicPredicate)
	// Note: this should never happen since this should be unmarshalled within the predicate verification
	if err != nil {
		return nil, remainingGas, fmt.Errorf("failed to unmarshal shared memory predicate: %w", err)
	}

	if atomicPredicate.SourceChain != inputStruct.SourceChain {
		return nil, remainingGas, fmt.Errorf("predicate source chain %s does not match specified source chain: %s", atomicPredicate.SourceChain, inputStruct.SourceChain)
	}

	// CUSTOM CODE STARTS HERE
	_ = inputStruct // CUSTOM CODE OPERATES ON INPUT
	// this function does not return an output, leave this one as is
	packedOutput := []byte{}

	// Return the packed output and the remaining gas
	return packedOutput, remainingGas, nil
}

// UnpackImportUTXOInput attempts to unpack [input] into the arguments for the ImportUTXOInput{}
// assumes that [input] does not include selector (omits first 4 func signature bytes)
func UnpackImportUTXOInput(input []byte) (ImportUTXOInput, error) {
	inputStruct := ImportUTXOInput{}
	err := SharedMemoryABI.UnpackInputIntoInterface(&inputStruct, "importUTXO", input)

	return inputStruct, err
}

// PackImportUTXO packs [inputStruct] of type ImportUTXOInput into the appropriate arguments for importUTXO.
func PackImportUTXO(inputStruct ImportUTXOInput) ([]byte, error) {
	return SharedMemoryABI.Pack("importUTXO", inputStruct.SourceChain, inputStruct.UtxoID)
}

// PackImportUTXOOutput attempts to pack given [outputStruct] of type ImportUTXOOutput
// to conform the ABI outputs.
func PackImportUTXOOutput(outputStruct ImportUTXOOutput) ([]byte, error) {
	return SharedMemoryABI.PackOutput("importUTXO",
		outputStruct.Amount,
		outputStruct.Locktime,
		outputStruct.Threshold,
		outputStruct.Addrs,
	)
}

func importUTXO(accessibleState PrecompileAccessibleState, caller common.Address, addr common.Address, input []byte, suppliedGas uint64, readOnly bool) (ret []byte, remainingGas uint64, err error) {
	if remainingGas, err = deductGas(suppliedGas, ImportUTXOGasCost); err != nil {
		return nil, 0, err
	}
	if readOnly {
		return nil, remainingGas, vmerrs.ErrWriteProtection
	}
	// attempts to unpack [input] into the arguments to the ImportUTXOInput.
	// Assumes that [input] does not include selector
	// You can use unpacked [inputStruct] variable in your code
	inputStruct, err := UnpackImportUTXOInput(input)
	if err != nil {
		return nil, remainingGas, err
	}

	// CUSTOM CODE STARTS HERE
	_ = inputStruct             // CUSTOM CODE OPERATES ON INPUT
	var output ImportUTXOOutput // CUSTOM CODE FOR AN OUTPUT
	packedOutput, err := PackImportUTXOOutput(output)
	if err != nil {
		return nil, remainingGas, err
	}

	// Return the packed output and the remaining gas
	return packedOutput, remainingGas, nil
}

// createSharedMemoryPrecompile returns a StatefulPrecompiledContract with getters and setters for the precompile.

func createSharedMemoryPrecompile(precompileAddr common.Address) StatefulPrecompiledContract {
	var functions []*statefulPrecompileFunction

	methodExportAVAX, ok := SharedMemoryABI.Methods["exportAVAX"]
	if !ok {
		panic("given method does not exist in the ABI")
	}
	functions = append(functions, newStatefulPrecompileFunction(methodExportAVAX.ID, exportAVAX))

	methodExportUTXO, ok := SharedMemoryABI.Methods["exportUTXO"]
	if !ok {
		panic("given method does not exist in the ABI")
	}
	functions = append(functions, newStatefulPrecompileFunction(methodExportUTXO.ID, exportUTXO))

	methodGetNativeTokenAssetID, ok := SharedMemoryABI.Methods["getNativeTokenAssetID"]
	if !ok {
		panic("given method does not exist in the ABI")
	}
	functions = append(functions, newStatefulPrecompileFunction(methodGetNativeTokenAssetID.ID, getNativeTokenAssetID))

	methodImportAVAX, ok := SharedMemoryABI.Methods["importAVAX"]
	if !ok {
		panic("given method does not exist in the ABI")
	}
	functions = append(functions, newStatefulPrecompileFunction(methodImportAVAX.ID, importAVAX))

	methodImportUTXO, ok := SharedMemoryABI.Methods["importUTXO"]
	if !ok {
		panic("given method does not exist in the ABI")
	}
	functions = append(functions, newStatefulPrecompileFunction(methodImportUTXO.ID, importUTXO))

	// Construct the contract with no fallback function.
	contract := newStatefulPrecompileWithFunctionSelectors(nil, functions)
	return contract
}
