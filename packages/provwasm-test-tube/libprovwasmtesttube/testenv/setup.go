package testenv

import (
	"encoding/json"
	"strings"
	"time"

	sdkmath "cosmossdk.io/math"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	cmttypes "github.com/cometbft/cometbft/types"
	"github.com/cosmos/cosmos-sdk/baseapp"
	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	"github.com/provenance-io/provenance/cmd/provenanced/config"
	nametypes "github.com/provenance-io/provenance/x/name/types"
	"github.com/spf13/pflag"

	// helpers

	// tendermint
	abci "github.com/cometbft/cometbft/abci/types"
	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"

	"cosmossdk.io/log"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/cosmos/cosmos-sdk/server"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"

	// wasmd

	// provenance
	"github.com/provenance-io/provenance/app"
)

type TestEnv struct {
	App                *app.App
	Ctx                sdk.Context
	ParamTypesRegistry ParamTypeRegistry
	ValPrivs           []*secp256k1.PrivKey
	Validator          []byte
	NodeHome           string
}

func GenesisStateWithValSet(appInstance *app.App) (app.GenesisState, secp256k1.PrivKey) {
	privVal := NewPV()
	pubKey, _ := privVal.GetPubKey()
	validator := cmttypes.NewValidator(pubKey, 1)
	valSet := cmttypes.NewValidatorSet([]*cmttypes.Validator{validator})

	// generate genesis account
	senderPrivKey := secp256k1.GenPrivKey()
	senderPrivKey.PubKey().Address()
	acc := authtypes.NewBaseAccountWithAddress(senderPrivKey.PubKey().Address().Bytes())

	//////////////////////
	balance := banktypes.Balance{
		Address: acc.GetAddress().String(),
		Coins:   sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdkmath.NewInt(100000000000000))),
	}
	genesisState := appInstance.DefaultGenesis()
	genAccs := []authtypes.GenesisAccount{acc}
	authGenesis := authtypes.NewGenesisState(authtypes.DefaultParams(), genAccs)
	genesisState[authtypes.ModuleName] = appInstance.AppCodec().MustMarshalJSON(authGenesis)

	rootName := nametypes.NewNameRecord("pb", sdk.AccAddress(validator.Address), false)
	scName := nametypes.NewNameRecord("sc.pb", sdk.AccAddress(validator.Address), false)
	nameRecords := nametypes.NameRecords{rootName, scName}
	nameGenesis := nametypes.NewGenesisState(nametypes.DefaultParams(), nameRecords)
	genesisState[nametypes.ModuleName] = appInstance.AppCodec().MustMarshalJSON(nameGenesis)

	genesisState, err := simtestutil.GenesisStateWithValSet(appInstance.AppCodec(), genesisState, valSet, []authtypes.GenesisAccount{acc}, balance)
	if err != nil {
		panic(err)
	}

	return genesisState, secp256k1.PrivKey{Key: privVal.PrivKey.Bytes()}
}

// DebugAppOptions is a stub implementing AppOptions
type DebugAppOptions struct{}

// Get implements AppOptions
func (ao DebugAppOptions) Get(o string) interface{} {
	if o == server.FlagTrace {
		return true
	}
	return nil
}

func SetupProvenanceApp(nodeHome string) *app.App {
	db := dbm.NewMemDB()

	app.SetConfig(true, false)

	provwasmFlags := pflag.NewFlagSet("provwasm-test-tube-flags", pflag.ContinueOnError)

	config.SetPioConfigFromFlags(provwasmFlags)

	baseAppOpts := []func(*baseapp.BaseApp){
		baseapp.SetChainID("testchain"),
	}

	appOpts := simtestutil.NewAppOptionsWithFlagHome(nodeHome)

	appInstance := app.New(
		log.NewNopLogger(),
		db,
		nil,
		true,
		appOpts,
		baseAppOpts...,
	)

	return appInstance
}

func InitChain(appInstance *app.App) (sdk.Context, secp256k1.PrivKey) {
	sdk.DefaultBondDenom = "nhash"
	genesisState, valPriv := GenesisStateWithValSet(appInstance)

	encCfg := appInstance.GetEncodingConfig()

	// Set up Wasm genesis state
	wasmGen := wasmtypes.GenesisState{
		Params: wasmtypes.Params{
			// Allow store code without gov
			CodeUploadAccess:             wasmtypes.AllowEverybody,
			InstantiateDefaultPermission: wasmtypes.AccessTypeEverybody,
		},
	}
	genesisState[wasmtypes.ModuleName] = encCfg.Marshaler.MustMarshalJSON(&wasmGen)

	stateBytes, err := json.MarshalIndent(genesisState, "", " ")

	requireNoErr(err)

	consensusParams := simtestutil.DefaultConsensusParams
	consensusParams.Block = &cmtproto.BlockParams{
		MaxBytes: 22020096,
		MaxGas:   -1,
	}

	// replace sdk.DefaultDenom with "nhash", a bit of a hack, needs improvement
	stateBytes = []byte(strings.Replace(string(stateBytes), "\"stake\"", "\"nhash\"", -1))

	_, err = appInstance.InitChain(
		&abci.RequestInitChain{
			ChainId:         "testnet",
			Validators:      []abci.ValidatorUpdate{},
			ConsensusParams: consensusParams,
			AppStateBytes:   stateBytes,
		},
	)
	requireNoErr(err)

	ctx := appInstance.NewUncachedContext(false, cmtproto.Header{Height: 0, ChainID: "testnet", Time: time.Now().UTC()})

	return ctx, valPriv
}

func (env *TestEnv) GetValidatorAddresses() []string {
	validators, err := env.App.StakingKeeper.GetAllValidators(env.Ctx)
	requireNoErr(err)

	var addresses []string
	for _, validator := range validators {
		addresses = append(addresses, validator.OperatorAddress)
	}

	return addresses
}

func (env *TestEnv) GetValidatorPrivateKey() []byte {
	return env.Validator
}

func (env *TestEnv) FundAccount(ctx sdk.Context, bankKeeper bankkeeper.Keeper, addr sdk.AccAddress, amounts sdk.Coins) error {
	if err := bankKeeper.MintCoins(ctx, minttypes.ModuleName, amounts); err != nil {
		return err
	}

	return bankKeeper.SendCoinsFromModuleToAccount(ctx, minttypes.ModuleName, addr, amounts)
}

func (env *TestEnv) SetupParamTypes() {
	pReg := env.ParamTypesRegistry

	pReg.RegisterParamSet(&minttypes.Params{})
}

func requireNoErr(err error) {
	if err != nil {
		panic(err)
	}
}
