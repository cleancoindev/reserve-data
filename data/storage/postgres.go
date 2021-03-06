package storage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"github.com/KyberNetwork/reserve-data/common"
	commonv3 "github.com/KyberNetwork/reserve-data/reservesetting/common"
)

const (
	schema = `
DO
$$
    BEGIN
        IF NOT EXISTS(SELECT 1 FROM pg_type WHERE typname = 'fetch_data_type') THEN
            CREATE TYPE fetch_data_type AS ENUM ('price', 'rate',
                'auth_data','gold', 'btc', 'usd');
        END IF;
    END
$$;

CREATE TABLE IF NOT EXISTS "fetch_data" 
(
	id SERIAL PRIMARY KEY,
	created TIMESTAMPTZ NOT NULL,
	data JSON NOT NULL,
	type fetch_data_type NOT NULL
);
CREATE INDEX IF NOT EXISTS "fetch_data_created_index" ON "fetch_data" (created);

CREATE TABLE IF NOT EXISTS "activity"
(
	id SERIAL PRIMARY KEY,
	timepoint BIGINT NOT NULL,
	eid TEXT NOT NULL,
	created TIMESTAMPTZ NOT NULL,
	is_pending BOOL NOT NULL,
	data JSONB NOT NULL
);
CREATE INDEX IF NOT EXISTS "activity_idx" ON "activity" (timepoint, eid);
CREATE INDEX IF NOT EXISTS "pending_idx" ON "activity" (is_pending) WHERE is_pending IS TRUE;
`
	fetchDataTable = "fetch_data" // data fetch from exchange and blockchain
	activityTable  = "activity"
	// data type constant

)

//go:generate enumer -type=fetchDataType -linecomment -json=true -sql=true
type fetchDataType int

const (
	priceDataType fetchDataType = iota // price
	rateDataType                       // rate
	authDataType                       // auth_data
	goldDataType                       // gold
	btcDataType                        // btc
	usdDataType                        // usd
)

// PostgresStorage struct
type PostgresStorage struct {
	db *sqlx.DB
	l  *zap.SugaredLogger
}

// NewPostgresStorage return new db instance
func NewPostgresStorage(db *sqlx.DB) (*PostgresStorage, error) {
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("failed to intialize database schema err=%s", err.Error())
	}

	s := &PostgresStorage{
		db: db,
		l:  zap.S(),
	}

	return s, nil
}

func getDataType(data interface{}) fetchDataType {
	switch data.(type) {
	case common.AuthDataSnapshot, *common.AuthDataSnapshot:
		return authDataType
	case common.BTCData, *common.BTCData:
		return btcDataType
	case common.GoldData, *common.GoldData:
		return goldDataType
	case common.USDData, *common.USDData:
		return usdDataType
	case common.AllPriceEntry, *common.AllPriceEntry:
		return priceDataType
	case common.AllRateEntry, *common.AllRateEntry:
		return rateDataType
	}
	panic(fmt.Sprintf("unexpected data type %+v", data))
}

func (ps *PostgresStorage) storeFetchData(data interface{}, timepoint uint64) error {
	query := fmt.Sprintf(`INSERT INTO "%s" (created, data, type) VALUES ($1, $2, $3)`, fetchDataTable)
	timestamp := common.MillisToTime(timepoint)
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return err
	}
	dataType := getDataType(data)
	if _, err := ps.db.Exec(query, timestamp, dataJSON, dataType); err != nil {
		return err
	}
	return nil
}

func (ps *PostgresStorage) currentVersion(dataType fetchDataType, timepoint uint64) (common.Version, error) {
	var (
		v  common.Version
		id int64
	)
	timestamp := common.MillisToTime(timepoint)
	query := fmt.Sprintf(`SELECT id FROM "%s" WHERE created <= $1 and type = $2 ORDER BY created DESC LIMIT 1`, fetchDataTable)
	if err := ps.db.Get(&id, query, timestamp, dataType); err != nil {
		if err == sql.ErrNoRows {
			return v, fmt.Errorf("there is no version at timestamp: %d", timepoint)
		}
		return v, err
	}
	v = common.Version(id)
	return v, nil
}

func (ps *PostgresStorage) getData(o interface{}, v common.Version) error {
	var (
		data []byte
	)
	dataType := getDataType(o)
	query := fmt.Sprintf(`SELECT data FROM "%s" WHERE id = $1 AND type = $2`, fetchDataTable)
	if err := ps.db.Get(&data, query, v, dataType); err != nil {
		return err
	}
	return json.Unmarshal(data, o)
}

// StorePrice store price
func (ps *PostgresStorage) StorePrice(priceEntry common.AllPriceEntry, timepoint uint64) error {
	return ps.storeFetchData(priceEntry, timepoint)
}

// CurrentPriceVersion return current price version
func (ps *PostgresStorage) CurrentPriceVersion(timepoint uint64) (common.Version, error) {
	return ps.currentVersion(priceDataType, timepoint)
}

// GetAllPrices return all prices currently save in db
func (ps *PostgresStorage) GetAllPrices(v common.Version) (common.AllPriceEntry, error) {
	var (
		allPrices common.AllPriceEntry
	)
	err := ps.getData(&allPrices, v)
	return allPrices, err
}

// GetOnePrice return one price
func (ps *PostgresStorage) GetOnePrice(pairID uint64, v common.Version) (common.OnePrice, error) {
	allPrices, err := ps.GetAllPrices(v)
	if err != nil {
		return common.OnePrice{}, err
	}
	onePrice, exist := allPrices.Data[pairID]
	if exist {
		return onePrice, nil
	}
	return common.OnePrice{}, errors.New("pair id does not exist")
}

// StoreAuthSnapshot store authdata
func (ps *PostgresStorage) StoreAuthSnapshot(authData *common.AuthDataSnapshot, timepoint uint64) error {
	return ps.storeFetchData(authData, timepoint)
}

// CurrentAuthDataVersion return current auth data version
func (ps *PostgresStorage) CurrentAuthDataVersion(timepoint uint64) (common.Version, error) {
	return ps.currentVersion(authDataType, timepoint)
}

// GetAuthData return auth data
func (ps *PostgresStorage) GetAuthData(v common.Version) (common.AuthDataSnapshot, error) {
	var (
		authData common.AuthDataSnapshot
	)
	err := ps.getData(&authData, v)
	return authData, err
}

// ExportExpiredAuthData export data to store on s3 storage
func (ps *PostgresStorage) ExportExpiredAuthData(timepoint uint64, filePath string) (uint64, error) {

	// create export file
	outFile, err := os.Create(filePath)
	if err != nil {
		return 0, err
	}
	defer func() {
		if cErr := outFile.Close(); cErr != nil {
			ps.l.Errorf("Close file error: %s", cErr.Error())
		}
	}()

	// Get expire data
	timepointExpireData := timepoint - authDataExpiredDuration
	timestampExpire := common.MillisToTime(timepointExpireData)
	query := fmt.Sprintf(`SELECT data FROM "%s" WHERE type = $1 AND created < $2`, fetchDataTable)
	rows, err := ps.db.Query(query, authDataType, timestampExpire)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}
	defer func() {
		if cle := rows.Close(); cle != nil {
			ps.l.Errorf("close result error %v", cle)
		}
	}()

	var count uint64
	for rows.Next() {
		var data []byte
		err = rows.Scan(&data)
		if err != nil {
			return 0, err
		}
		_, _ = outFile.Write(data)
		_, err = outFile.Write([]byte{'\n'})
		if err != nil {
			return 0, err
		}
		count++
	}
	return count, nil
}

// PruneExpiredAuthData remove expire auth data from database
func (ps *PostgresStorage) PruneExpiredAuthData(timepoint uint64) (uint64, error) {
	var (
		count uint64
	)
	// Get expire data
	timepointExpireData := timepoint - authDataExpiredDuration
	timestampExpire := common.MillisToTime(timepointExpireData)
	query := fmt.Sprintf(`WITH deleted AS 
	(DELETE FROM "%s" WHERE type = $1 AND created < $2 RETURNING *) SELECT count(*) FROM deleted`, fetchDataTable)
	if err := ps.db.Get(&count, query, authDataType, timestampExpire); err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}
	return count, nil
}

// StoreRate store rate
func (ps *PostgresStorage) StoreRate(allRateEntry common.AllRateEntry, timepoint uint64) error {
	return ps.storeFetchData(allRateEntry, timepoint)
}

// CurrentRateVersion return current rate version
func (ps *PostgresStorage) CurrentRateVersion(timepoint uint64) (common.Version, error) {
	return ps.currentVersion(rateDataType, timepoint)
}

// GetRate return rate at a specific version
func (ps *PostgresStorage) GetRate(v common.Version) (common.AllRateEntry, error) {
	var (
		rate common.AllRateEntry
	)
	err := ps.getData(&rate, v)
	return rate, err
}

//GetRates return rate from time to time
func (ps *PostgresStorage) GetRates(fromTime, toTime uint64) ([]common.AllRateEntry, error) {
	var (
		rates []common.AllRateEntry
		data  [][]byte
	)
	query := fmt.Sprintf(`SELECT data FROM "%s" WHERE type = $1 AND created >= $2 AND created <= $3`, fetchDataTable)
	from := common.MillisToTime(fromTime)
	to := common.MillisToTime(toTime)
	if err := ps.db.Select(&data, query, rateDataType, from, to); err != nil {
		return []common.AllRateEntry{}, err
	}
	for _, dataByte := range data {
		var rate common.AllRateEntry
		if err := json.Unmarshal(dataByte, &rate); err != nil {
			return []common.AllRateEntry{}, err
		}
		rates = append(rates, rate)
	}
	return rates, nil
}

// GetAllRecords return all activities records from database
func (ps *PostgresStorage) GetAllRecords(fromTime, toTime uint64) ([]common.ActivityRecord, error) {
	var (
		activities []common.ActivityRecord
		data       [][]byte
	)
	query := fmt.Sprintf(`SELECT data FROM "%s" WHERE created >= $1 AND created <= $2`, activityTable)
	from := common.MillisToTime(fromTime)
	to := common.MillisToTime(toTime)
	if err := ps.db.Select(&data, query, from, to); err != nil {
		return nil, err
	}
	for _, dataByte := range data {
		var activity common.ActivityRecord
		if err := json.Unmarshal(dataByte, &activity); err != nil {
			return nil, err
		}
		activities = append(activities, activity)
	}
	return activities, nil
}

// GetPendingActivities return all pending activities
func (ps *PostgresStorage) GetPendingActivities() ([]common.ActivityRecord, error) {
	var (
		pendingActivities []common.ActivityRecord
		data              [][]byte
	)
	query := fmt.Sprintf(`SELECT data FROM "%s" WHERE is_pending IS TRUE`, activityTable)
	if err := ps.db.Select(&data, query); err != nil {
		return []common.ActivityRecord{}, err
	}
	for _, dataByte := range data {
		var activity common.ActivityRecord
		if err := json.Unmarshal(dataByte, &activity); err != nil {
			return []common.ActivityRecord{}, err
		}
		pendingActivities = append(pendingActivities, activity)
	}
	return pendingActivities, nil
}

// UpdateActivity update activity to finished if it is finished
func (ps *PostgresStorage) UpdateActivity(id common.ActivityID, act common.ActivityRecord) error {
	var (
		data []byte
	)
	// get activity from db
	getQuery := fmt.Sprintf(`SELECT data FROM "%s" WHERE timepoint = $1 AND eid = $2`, activityTable)
	if err := ps.db.Get(&data, getQuery, id.Timepoint, id.EID); err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return err
	}
	// check if activity is not pending anymore update it
	updateQuery := fmt.Sprintf(`UPDATE "%s" SET is_pending = $1, data = $2 WHERE timepoint = $3 AND eid = $4`, activityTable)
	dataBytes, err := json.Marshal(act)
	if err != nil {
		return err
	}
	if !act.IsPending() {
		if _, err := ps.db.Exec(updateQuery, false, dataBytes, id.Timepoint, id.EID); err != nil {
			return err
		}
	}
	return nil
}

// GetActivity return activity record by id
func (ps *PostgresStorage) GetActivity(id common.ActivityID) (common.ActivityRecord, error) {
	var (
		activityRecord common.ActivityRecord
		data           []byte
	)
	query := fmt.Sprintf(`SELECT data FROM "%s" WHERE timepoint = $1 AND eid = $2`, activityTable)
	if err := ps.db.Get(&data, query, id.Timepoint, id.EID); err != nil {
		return common.ActivityRecord{}, err
	}
	if err := json.Unmarshal(data, &activityRecord); err != nil {
		return common.ActivityRecord{}, err
	}
	return activityRecord, nil
}

// PendingSetRate return pending set rate activity
func (ps *PostgresStorage) PendingSetRate(minedNonce uint64) (*common.ActivityRecord, uint64, error) {
	pendings, err := ps.GetPendingActivities()
	if err != nil {
		return nil, 0, err
	}
	return getFirstAndCountPendingSetrate(pendings, minedNonce)
}

// HasPendingDeposit return true if there is any pending deposit for a token
func (ps *PostgresStorage) HasPendingDeposit(token commonv3.Asset, exchange common.Exchange) (bool, error) {
	var (
		pendingActivity common.ActivityRecord
		data            [][]byte
	)
	query := fmt.Sprintf(`SELECT data FROM "%s" WHERE is_pending IS TRUE AND data->>'Action' = $1 AND data ->> 'Destination' = $2`, activityTable)
	if err := ps.db.Select(&data, query, common.ActionDeposit, exchange.ID().String()); err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	for _, activity := range data {
		if err := json.Unmarshal(activity, &pendingActivity); err != nil {
			return false, err
		}
		if pendingActivity.Params.Asset == token.ID {
			return true, nil
		}
	}
	return false, nil
}

// Record save activity
func (ps *PostgresStorage) Record(action string, id common.ActivityID, destination string,
	params common.ActivityParams, result common.ActivityResult,
	estatus string, mstatus string, timepoint uint64) error {
	record := common.NewActivityRecord(
		action,
		id,
		destination,
		params,
		result,
		estatus,
		mstatus,
		common.Timestamp(strconv.FormatUint(timepoint, 10)),
	)
	query := fmt.Sprintf(`INSERT INTO "%s" (created, data, is_pending, timepoint, eid) VALUES($1, $2, $3, $4, $5)`, activityTable)
	timestamp := common.MillisToTime(timepoint)
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if _, err := ps.db.Exec(query, timestamp, data, true, id.Timepoint, id.EID); err != nil {
		return err
	}
	return nil
}

// StoreGoldInfo store gold info into database
func (ps *PostgresStorage) StoreGoldInfo(goldData common.GoldData) error {
	timepoint := goldData.Timestamp
	return ps.storeFetchData(goldData, timepoint)
}

// StoreBTCInfo store btc info into database
func (ps *PostgresStorage) StoreBTCInfo(btcData common.BTCData) error {
	timepoint := btcData.Timestamp
	return ps.storeFetchData(btcData, timepoint)
}

// StoreUSDInfo store btc info into database
func (ps *PostgresStorage) StoreUSDInfo(usdData common.USDData) error {
	timepoint := usdData.Timestamp
	return ps.storeFetchData(usdData, timepoint)
}

// GetGoldInfo return gold info
func (ps *PostgresStorage) GetGoldInfo(v common.Version) (common.GoldData, error) {
	var (
		goldData common.GoldData
	)
	err := ps.getData(&goldData, v)
	return goldData, err
}

// GetBTCInfo return BTC info
func (ps *PostgresStorage) GetBTCInfo(v common.Version) (common.BTCData, error) {
	var (
		btcData common.BTCData
	)
	err := ps.getData(&btcData, v)
	return btcData, err
}

// GetUSDInfo return USD info
func (ps *PostgresStorage) GetUSDInfo(v common.Version) (common.USDData, error) {
	var (
		usdData common.USDData
	)
	err := ps.getData(&usdData, v)
	return usdData, err
}

// CurrentGoldInfoVersion return btc info version
func (ps *PostgresStorage) CurrentGoldInfoVersion(timepoint uint64) (common.Version, error) {
	return ps.currentVersion(goldDataType, timepoint)
}

// CurrentBTCInfoVersion return current btc info version
func (ps *PostgresStorage) CurrentBTCInfoVersion(timepoint uint64) (common.Version, error) {
	return ps.currentVersion(btcDataType, timepoint)
}

// CurrentUSDInfoVersion return current btc info version
func (ps *PostgresStorage) CurrentUSDInfoVersion(timepoint uint64) (common.Version, error) {
	return ps.currentVersion(usdDataType, timepoint)
}
