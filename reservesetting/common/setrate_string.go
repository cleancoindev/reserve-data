// Code generated by "stringer -type=SetRate -linecomment"; DO NOT EDIT.

package common

import "strconv"

func _() {
	// An "invalid array index" compiler error signifies that the constant values have changed.
	// Re-run the stringer command to generate them again.
	var x [1]struct{}
	_ = x[SetRateNotSet-0]
	_ = x[ExchangeFeed-1]
	_ = x[GoldFeed-2]
	_ = x[BTCFeed-3]
	_ = x[USDFeed-4]
}

const _SetRate_name = "not_setexchange_feedgold_feedbtc_feedusd_feed"

var _SetRate_index = [...]uint8{0, 7, 20, 29, 37, 45}

func (i SetRate) String() string {
	if i < 0 || i >= SetRate(len(_SetRate_index)-1) {
		return "SetRate(" + strconv.FormatInt(int64(i), 10) + ")"
	}
	return _SetRate_name[_SetRate_index[i]:_SetRate_index[i+1]]
}
