package storage

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"

	"github.com/boltdb/bolt"

	"github.com/KyberNetwork/reserve-data/boltutil"
	"github.com/KyberNetwork/reserve-data/common"
)

const (
	intermediateTx        string = "intermediate_tx"
	pendingIntermediateTx string = "pending_intermediate_tx"
	tradeHistory          string = "trade_history"
	maxGetTradeHistory    uint64 = 3 * 86400000
)

//BoltStorage strage object for using huobi
//including boltdb
type BoltStorage struct {
	mu sync.RWMutex
	db *bolt.DB
}

// Deprecated: Using postgres instead of bolt
//NewBoltStorage return new storage instance
func NewBoltStorage(path string) (*BoltStorage, error) {
	// init instance
	var err error
	var db *bolt.DB
	db, err = bolt.Open(path, 0600, nil)
	if err != nil {
		return nil, err
	}
	// init buckets
	err = db.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists([]byte(intermediateTx)); err != nil {
			return err
		}
		if _, err := tx.CreateBucketIfNotExists([]byte(pendingIntermediateTx)); err != nil {
			return err
		}
		if _, err := tx.CreateBucketIfNotExists([]byte(tradeHistory)); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	storage := &BoltStorage{sync.RWMutex{}, db}
	return storage, nil
}

//GetPendingIntermediateTXs return pending transaction for first deposit phase
func (bs *BoltStorage) GetPendingIntermediateTXs() (map[common.ActivityID]common.TXEntry, error) {
	result := make(map[common.ActivityID]common.TXEntry)
	var err error
	err = bs.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(pendingIntermediateTx))
		c := b.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			actID := common.ActivityID{}
			record := common.TXEntry{}
			if err = json.Unmarshal(k, &actID); err != nil {
				return err
			}
			if err = json.Unmarshal(v, &record); err != nil {
				return err
			}
			result[actID] = record
		}
		return nil
	})
	return result, err
}

//StorePendingIntermediateTx store pending transaction
func (bs *BoltStorage) StorePendingIntermediateTx(id common.ActivityID, data common.TXEntry) error {
	var err error
	err = bs.db.Update(func(tx *bolt.Tx) error {
		var dataJSON []byte
		b := tx.Bucket([]byte(pendingIntermediateTx))
		dataJSON, uErr := json.Marshal(data)
		if uErr != nil {
			return err
		}
		idJSON, uErr := json.Marshal(id)
		if uErr != nil {
			return uErr
		}
		return b.Put(idJSON, dataJSON)
	})
	return err
}

//StoreIntermediateTx store intermediate transaction and remove it from pending bucket
func (bs *BoltStorage) StoreIntermediateTx(id common.ActivityID, data common.TXEntry) error {
	err := bs.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(intermediateTx))
		dataJSON, uErr := json.Marshal(data)
		if uErr != nil {
			return uErr
		}
		idByte := id.ToBytes()
		if uErr = b.Put(idByte[:], dataJSON); uErr != nil {
			return uErr
		}

		// remove pending intermediate tx
		pendingBucket := tx.Bucket([]byte(pendingIntermediateTx))
		idJSON, uErr := json.Marshal(id)
		if uErr != nil {
			return uErr
		}
		return pendingBucket.Delete(idJSON)
	})
	return err
}

func isTheSame(a []byte, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for ix, v := range a {
		if b[ix] != v {
			return false
		}
	}
	return true
}

//GetIntermedatorTx get intermediate transaction
func (bs *BoltStorage) GetIntermedatorTx(id common.ActivityID) (common.TXEntry, error) {
	var (
		tx2 common.TXEntry
		err error
	)
	err = bs.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(intermediateTx))
		c := b.Cursor()
		idBytes := id.ToBytes()
		k, v := c.Seek(idBytes[:])
		if isTheSame(k, idBytes[:]) {
			return json.Unmarshal(v, &tx2)
		}
		return errors.New("can not find 2nd transaction tx for the deposit, please try later")
	})
	return tx2, err
}

//StoreTradeHistory store trade history
func (bs *BoltStorage) StoreTradeHistory(data common.ExchangeTradeHistory) error {
	err := bs.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(tradeHistory))
		for pair, pairHistory := range data {
			pairBk, uErr := b.CreateBucketIfNotExists(boltutil.Uint64ToBytes(pair))
			if uErr != nil {
				return uErr
			}
			for _, history := range pairHistory {
				idBytes := []byte(fmt.Sprintf("%s%s", strconv.FormatUint(history.Timestamp, 10), history.ID))
				dataJSON, uErr := json.Marshal(history)
				if uErr != nil {
					return uErr
				}
				uErr = pairBk.Put(idBytes, dataJSON)
				if uErr != nil {
					return uErr
				}
			}
		}
		return nil
	})
	return err
}

//GetTradeHistory get trade history
func (bs *BoltStorage) GetTradeHistory(fromTime, toTime uint64) (common.ExchangeTradeHistory, error) {
	result := common.ExchangeTradeHistory{}
	var err error
	if toTime-fromTime > maxGetTradeHistory {
		return result, errors.New("time range is too broad, it must be smaller or equal to 3 days (miliseconds)")
	}
	min := []byte(strconv.FormatUint(fromTime, 10))
	max := []byte(strconv.FormatUint(toTime, 10))
	err = bs.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(tradeHistory))
		c := b.Cursor()
		exchangeHistory := common.ExchangeTradeHistory{}
		for key, value := c.First(); key != nil && value == nil; key, value = c.Next() {
			pairBk := b.Bucket(key)
			pairsHistory := []common.TradeHistory{}
			pairCursor := pairBk.Cursor()
			for pairKey, history := pairCursor.Seek(min); pairKey != nil && bytes.Compare(pairKey, max) <= 0; pairKey, history = pairCursor.Next() {
				pairHistory := common.TradeHistory{}
				if uErr := json.Unmarshal(history, &pairHistory); uErr != nil {
					return uErr
				}
				pairsHistory = append(pairsHistory, pairHistory)
			}
			exchangeHistory[boltutil.BytesToUint64(key)] = pairsHistory
		}
		result = exchangeHistory
		return nil
	})
	return result, err
}
