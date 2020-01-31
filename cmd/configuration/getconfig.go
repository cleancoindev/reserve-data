package configuration

import (
	"encoding/json"
	"io/ioutil"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/urfave/cli"
	"go.uber.org/zap"

	"github.com/KyberNetwork/reserve-data/cmd/deployment"
	"github.com/KyberNetwork/reserve-data/common"
	"github.com/KyberNetwork/reserve-data/common/archive"
	"github.com/KyberNetwork/reserve-data/common/blockchain"
	"github.com/KyberNetwork/reserve-data/exchange/binance"
	"github.com/KyberNetwork/reserve-data/exchange/huobi"
	"github.com/KyberNetwork/reserve-data/reservesetting/storage"
	"github.com/KyberNetwork/reserve-data/world"
)

const (
	byzantiumChainType = "byzantium"
	homesteadChainType = "homestead"
)

// GetChainType return chain type
func GetChainType(dpl deployment.Deployment) string {
	switch dpl {
	case deployment.Production:
		return byzantiumChainType
	case deployment.Development:
		return homesteadChainType
	case deployment.Kovan:
		return homesteadChainType
	case deployment.Staging:
		return byzantiumChainType
	case deployment.Simulation, deployment.Analytic:
		return homesteadChainType
	case deployment.Ropsten:
		return byzantiumChainType
	default:
		return homesteadChainType
	}
}

// rawConfig include all configs read from files
type rawConfig struct {
	WorldEndpoints common.WorldEndpoints `json:"world_endpoints"`
	AWSConfig      archive.AWSConfig     `json:"aws_config"`

	PricingKeystore   string `json:"keystore_path"`
	PricingPassphrase string `json:"passphrase"`
	DepositKeystore   string `json:"keystore_deposit_path"`
	DepositPassphrase string `json:"passphrase_deposit"`

	BinanceKey    string `json:"binance_key"`
	BinanceSecret string `json:"binance_secret"`
	HoubiKey      string `json:"huobi_key"`
	HoubiSecret   string `json:"huobi_secret"`

	IntermediatorKeystore   string `json:"keystore_intermediator_path"`
	IntermediatorPassphrase string `json:"passphrase_intermediate_account"`
}

func loadConfigFromFile(path string, rcf *rawConfig) error {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, rcf)
}

// GetConfig return config for core
func GetConfig(
	cliCtx *cli.Context,
	dpl deployment.Deployment,
	nodeConf *EthereumNodeConfiguration,
	bi binance.Interface,
	hi huobi.Interface,
	contractAddressConf *common.ContractAddressConfiguration,
	dataFile string,
	configFile string,
	secretConfigFile string,
	settingStorage storage.Interface,
) (*Config, error) {
	l := zap.S()
	rcf := rawConfig{}
	if err := loadConfigFromFile(configFile, &rcf); err != nil {
		return nil, err
	}
	if err := loadConfigFromFile(secretConfigFile, &rcf); err != nil {
		return nil, err
	}

	chainType := GetChainType(dpl)

	//set client & endpoint
	client, err := rpc.Dial(nodeConf.Main)
	if err != nil {
		return nil, err
	}

	mainClient := ethclient.NewClient(client)
	bkClients := map[string]*ethclient.Client{}

	var callClients []*common.EthClient
	for _, ep := range nodeConf.Backup {
		var bkClient *ethclient.Client
		bkClient, err = ethclient.Dial(ep)
		if err != nil {
			l.Warnw("Cannot connect to rpc endpoint", "endpoint", ep, "err", err)
		} else {
			bkClients[ep] = bkClient
			callClients = append(callClients, &common.EthClient{
				Client: bkClient,
				URL:    ep,
			})
		}
	}

	bc := blockchain.NewBaseBlockchain(
		client, mainClient, map[string]*blockchain.Operator{},
		blockchain.NewBroadcaster(bkClients),
		chainType,
		blockchain.NewContractCaller(callClients),
	)

	s3archive := archive.NewS3Archive(rcf.AWSConfig)
	theWorld := world.NewTheWorld(rcf.WorldEndpoints)

	config := &Config{
		Blockchain:              bc,
		EthereumEndpoint:        nodeConf.Main,
		BackupEthereumEndpoints: nodeConf.Backup,
		Archive:                 s3archive,
		World:                   theWorld,
		ContractAddresses:       contractAddressConf,
		SettingStorage:          settingStorage,
	}

	l.Infow("configured endpoint", "endpoint", config.EthereumEndpoint, "backup", config.BackupEthereumEndpoints)
	if err = config.AddCoreConfig(cliCtx, rcf, dpl, bi, hi, contractAddressConf, dataFile, settingStorage); err != nil {
		return nil, err
	}
	return config, nil
}
