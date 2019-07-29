package postgres

import (
	"database/sql"
	"log"

	ethereum "github.com/ethereum/go-ethereum/common"

	"github.com/KyberNetwork/reserve-data/v3/common"
)

func (s *Storage) GetTransferableAssets() ([]common.Asset, error) {
	transferable := true
	return s.getAssets(&transferable)
}

type tradingPairSymbolsDB struct {
	tradingPairDB
	BaseSymbol  string `db:"base_symbol"`
	QuoteSymbol string `db:"quote_symbol"`
}

func (s *Storage) GetTradingPairs(id uint64) ([]common.TradingPairSymbols, error) {
	var (
		tradingPairs []tradingPairSymbolsDB
		result       []common.TradingPairSymbols
	)
	if err := s.stmts.getTradingPairSymbols.Select(&tradingPairs, id); err != nil {
		return nil, err
	}
	for _, pair := range tradingPairs {
		result = append(result, common.TradingPairSymbols{
			TradingPair: pair.tradingPairDB.ToCommon(),
			BaseSymbol:  pair.BaseSymbol,
			QuoteSymbol: pair.QuoteSymbol,
		})
	}
	return result, nil
}

// TODO: rewrite this function to filter the exchange if in SQL query.
func (s *Storage) GetDepositAddresses(exchangeID uint64) (map[string]ethereum.Address, error) {
	var results = make(map[string]ethereum.Address)

	allAssets, err := s.GetAssets()
	if err != nil {
		return nil, err
	}

	for _, asset := range allAssets {
		for _, exchange := range asset.Exchanges {
			if exchange.ExchangeID == exchangeID {
				results[exchange.Symbol] = exchange.DepositAddress
			}
		}
	}

	return results, nil
}

func (s *Storage) GetMinNotional(exchangeID, baseID, quoteID uint64) (float64, error) {
	var minNotional float64
	log.Printf("getting min notional for exchange=%d base=%d quote=%d",
		exchangeID, baseID, quoteID)
	if err := s.stmts.getMinNotional.Get(&minNotional,
		exchangeID, baseID, quoteID); err == sql.ErrNoRows {
		return 0, common.ErrNotFound
	} else if err != nil {
		return 0, err
	}
	return minNotional, nil
}