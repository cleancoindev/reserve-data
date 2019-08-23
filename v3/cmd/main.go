package main

import (
	"fmt"
	"log"
	"os"

	"github.com/urfave/cli"

	"github.com/KyberNetwork/reserve-data/cmd/configuration"
	"github.com/KyberNetwork/reserve-data/cmd/deployment"
	v1common "github.com/KyberNetwork/reserve-data/common"
	"github.com/KyberNetwork/reserve-data/common/profiler"
	"github.com/KyberNetwork/reserve-data/exchange"
	"github.com/KyberNetwork/reserve-data/exchange/binance"
	"github.com/KyberNetwork/reserve-data/exchange/huobi"
	"github.com/KyberNetwork/reserve-data/lib/httputil"
	"github.com/KyberNetwork/reserve-data/v3/http"
	"github.com/KyberNetwork/reserve-data/v3/storage/postgres"
)

const (
	defaultDB = "reserve_data"
)

func main() {
	app := cli.NewApp()
	app.Name = "HTTP gateway for reserve core"
	app.Action = run

	app.Flags = append(app.Flags, deployment.NewCliFlag())
	app.Flags = append(app.Flags, configuration.NewBinanceCliFlags()...)
	app.Flags = append(app.Flags, configuration.NewHuobiCliFlags()...)
	app.Flags = append(app.Flags, configuration.NewPostgreSQLFlags(defaultDB)...)
	app.Flags = append(app.Flags, httputil.NewHTTPCliFlags(httputil.V3ServicePort)...)
	app.Flags = append(app.Flags, configuration.NewExchangeCliFlag())
	app.Flags = append(app.Flags, profiler.NewCliFlags()...)

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func run(c *cli.Context) error {
	host := httputil.NewHTTPAddressFromContext(c)
	db, err := configuration.NewDBFromContext(c)
	if err != nil {
		return err
	}

	dpl, err := deployment.NewDeploymentFromContext(c)
	if err != nil {
		return err
	}

	enableExchanges, err := configuration.NewExchangesFromContext(c)
	if err != nil {
		return fmt.Errorf("failed to get enabled exchanges: %s", err)
	}

	bi := configuration.NewBinanceInterfaceFromContext(c)
	// dummy signer as live infos does not need to sign
	binanceSigner := binance.NewSigner("", "")
	binanceEndpoint := binance.NewBinanceEndpoint(binanceSigner, bi, dpl)
	hi := configuration.NewhuobiInterfaceFromContext(c)
	// dummy signer as live infos does not need to sign
	huobiSigner := huobi.NewSigner("", "")
	huobiEndpoint := huobi.NewHuobiEndpoint(huobiSigner, hi)

	liveExchanges, err := getLiveExchanges(enableExchanges, binanceEndpoint, huobiEndpoint)
	if err != nil {
		return fmt.Errorf("failed to initiate live exchanges: %s", err)
	}

	sr, err := postgres.NewStorage(db)
	if err != nil {
		return err
	}

	server := http.NewServer(sr, host, liveExchanges)
	if profiler.IsEnableProfilerFromContext(c) {
		server.EnableProfiler()
	}
	server.Run()
	return nil
}

func getLiveExchanges(enabledExchanges []v1common.ExchangeID, bi exchange.BinanceInterface, hi exchange.HuobiInterface) (map[v1common.ExchangeID]v1common.LiveExchange, error) {
	var (
		liveExchanges = make(map[v1common.ExchangeID]v1common.LiveExchange)
	)
	for _, exchangeID := range enabledExchanges {
		switch exchangeID {
		case v1common.Binance:
			binanceLive := exchange.NewBinanceLive(bi)
			liveExchanges[v1common.Binance] = binanceLive
		case v1common.Huobi:
			huobiLive := exchange.NewHuobiLive(hi)
			liveExchanges[v1common.Huobi] = huobiLive
		}
	}
	return liveExchanges, nil
}