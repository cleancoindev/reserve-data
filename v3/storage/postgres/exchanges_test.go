package postgres

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/KyberNetwork/reserve-data/common"
	"github.com/KyberNetwork/reserve-data/common/testutil"
	commonv3 "github.com/KyberNetwork/reserve-data/v3/common"
	"github.com/KyberNetwork/reserve-data/v3/storage"
)

func TestExchangeStorage(t *testing.T) {
	db, tearDown := testutil.MustNewDevelopmentDB()
	defer func() {
		assert.NoError(t, tearDown())
	}()

	s, err := NewStorage(db)
	require.NoError(t, err)

	// expect that exchange are initialized
	exchanges, err := s.GetExchanges()
	require.NoError(t, err)
	assert.Len(t, exchanges, len(common.ValidExchangeNames))

	for _, exchange := range exchanges {
		switch exchange.ID {
		case uint64(common.StableExchange):
			assert.NotZero(t, exchange.TradingFeeMaker)
			assert.NotZero(t, exchange.TradingFeeTaker)
			assert.False(t, exchange.Disable)
		default:
			assert.Zero(t, exchange.TradingFeeMaker)
			assert.Zero(t, exchange.TradingFeeTaker)
			assert.True(t, exchange.Disable)
		}
	}

	// exchange should not be allowed to enable if trading fees are not all set
	err = s.UpdateExchange(uint64(common.Huobi),
		storage.UpdateExchangeOpts{
			TradingFeeTaker: commonv3.FloatPointer(0.02),
			Disable:         commonv3.BoolPointer(false),
		})
	assert.Error(t, err)
	assert.Equal(t, commonv3.ErrExchangeFeeMissing, err)

	err = s.UpdateExchange(uint64(common.Huobi), storage.UpdateExchangeOpts{
		TradingFeeTaker: commonv3.FloatPointer(0.01),
		TradingFeeMaker: commonv3.FloatPointer(0.02),
	},
	)
	require.NoError(t, err)

	exchanges, err = s.GetExchanges()
	require.NoError(t, err)

	for _, exchange := range exchanges {
		switch exchange.ID {
		case uint64(common.Huobi):
			assert.Equal(t, 0.01, exchange.TradingFeeMaker)
			assert.Equal(t, 0.02, exchange.TradingFeeTaker)
			assert.True(t, exchange.Disable)
		case uint64(common.Binance):
			assert.Zero(t, exchange.TradingFeeMaker)
			assert.Zero(t, exchange.TradingFeeTaker)
			assert.True(t, exchange.Disable)
		}
	}

	err = s.UpdateExchange(uint64(common.Huobi),
		storage.UpdateExchangeOpts{
			TradingFeeMaker: commonv3.FloatPointer(0.01),
			TradingFeeTaker: commonv3.FloatPointer(0.02),
			Disable:         commonv3.BoolPointer(false),
		})
	require.NoError(t, err)

	exchanges, err = s.GetExchanges()
	require.NoError(t, err)
	for _, exchange := range exchanges {
		if exchange.ID == uint64(common.Huobi) {
			assert.Equal(t, 0.01, exchange.TradingFeeMaker)
			assert.Equal(t, 0.02, exchange.TradingFeeTaker)
			assert.False(t, exchange.Disable)
		}
	}

	huobiExchange, err := s.GetExchange(uint64(common.Huobi))
	require.NoError(t, err)
	assert.Equal(t, 0.01, huobiExchange.TradingFeeMaker)
	assert.Equal(t, 0.02, huobiExchange.TradingFeeTaker)
	assert.False(t, huobiExchange.Disable)
}
