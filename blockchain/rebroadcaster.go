package blockchain

import (
	"context"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

// Rebroadcaster takes a signed tx and try to broadcast it to all
// nodes that it manages as fast as possible. It returns a map of
// failures and a bool indicating that the tx is broadcasted to
// at least 1 node
type Rebroadcaster struct {
	clients map[string]*ethclient.Client
}

func (self Rebroadcaster) broadcast(
	ctx context.Context,
	id string, client *ethclient.Client, tx *types.Transaction,
	wg *sync.WaitGroup, failures *sync.Map) {
	defer wg.Done()
	err := client.SendTransaction(ctx, tx)
	if err != nil {
		failures.Store(id, err)
	}
}

func (self Rebroadcaster) Broadcast(tx *types.Transaction) (map[string]error, bool) {
	failures := sync.Map{}
	wg := sync.WaitGroup{}
	for id, client := range self.clients {
		wg.Add(1)
		timeout, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		self.broadcast(timeout, id, client, tx, &wg, &failures)
		defer cancel()
	}
	wg.Wait()
	result := map[string]error{}
	failures.Range(func(key, value interface{}) bool {
		result[key.(string)] = value.(error)
		return true
	})
	return result, len(result) != len(self.clients) && len(self.clients) > 0
}

func NewRebroadcaster(clients map[string]*ethclient.Client) *Rebroadcaster {
	return &Rebroadcaster{
		clients: clients,
	}
}