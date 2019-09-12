package http

import (
	ethereum "github.com/ethereum/go-ethereum/common"
	"github.com/gin-gonic/gin"

	"github.com/KyberNetwork/reserve-data/common"
	"github.com/KyberNetwork/reserve-data/http/httputil"
)

const (
	pricingOPAddressName      = "pricing_operator"
	depositOPAddressName      = "deposit_operator"
	intermediateOPAddressName = "intermediate_operator"
	wrapper                   = "wrapper"
	internalNetwork           = "internal_network"
	network                   = "network"
)

// GetAddresses get address config from core
func (s *Server) GetAddresses(c *gin.Context) {
	var (
		addresses = make(map[string]ethereum.Address)
	)
	addresses[pricingOPAddressName] = s.blockchain.GetPricingOPAddress()
	addresses[depositOPAddressName] = s.blockchain.GetDepositOPAddress()
	addresses[intermediateOPAddressName] = s.blockchain.GetIntermediatorOPAddress()
	addresses[wrapper] = s.blockchain.GetWrapperAddress()
	addresses[internalNetwork] = s.blockchain.GetInternalNetworkAddress()
	addresses[network] = s.blockchain.GetNetworkAddress()
	result := common.NewAddressesResponse(addresses)
	httputil.ResponseSuccess(c, httputil.WithData(result))
}