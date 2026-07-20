package gotdx

// MACSortType 表示 MAC 板块成分排序字段。
type MACSortType uint16

const (
	MACSortCode              MACSortType = 0x00
	MACSortName              MACSortType = 0x01
	MACSortPrice             MACSortType = 0x06
	MACSortVol               MACSortType = 0x07
	MACSortAmount            MACSortType = 0x08
	MACSortTotalAmount       MACSortType = 0x0a
	MACSortLastVolume        MACSortType = 0x0b
	MACSortChange            MACSortType = 0x0c
	MACSortIndustry          MACSortType = 0x0d
	MACSortChangePct         MACSortType = 0x0e
	MACSortAmplitudePct      MACSortType = 0x0f
	MACSortShortTurnoverPct  MACSortType = 0xcc
	MACSortVolSpeedPct       MACSortType = 0xd0
	MACSortChange5DPct       MACSortType = 0xd1
	MACSortChange10DPct      MACSortType = 0xd2
	MACSortMainNetAmount     MACSortType = 0xd4
	MACSortMainNetRatio      MACSortType = 0xd7
	MACSortAuctionLimitBuy   MACSortType = 0x102
	MACSortAuctionLimitSell  MACSortType = 0x103
	MACSortLimitUpTime       MACSortType = 0x108
	MACSortLimitDownTime     MACSortType = 0x109
	MACSortDrawdownPct       MACSortType = 0x11e
	MACSortAttackPct         MACSortType = 0x11f
	MACSortYTDPct            MACSortType = 0x121
	MACSortConsecutiveUpDays MACSortType = 0x138
	MACSortChange3DPct       MACSortType = 0x139
	MACSortDividendYieldRate MACSortType = 0x13a
	MACSortChange20DPct      MACSortType = 0x172
	MACSortChange60DPct      MACSortType = 0x173
	MACSortPrevChangePct     MACSortType = 0x17e
	MACSortMTDPct            MACSortType = 0x19c
	MACSortChange1YPct       MACSortType = 0x19d
	MACSortPETTM             MACSortType = 0x3ee
	MACSortPEStatic          MACSortType = 0x3ef
)

// MACSortOrder 表示 MAC 板块成分排序方向。
type MACSortOrder uint8

const (
	MACSortOrderNone MACSortOrder = 0
	MACSortOrderDesc MACSortOrder = 1
	MACSortOrderAsc  MACSortOrder = 2
)

// MACFilterType 表示 MAC 板块成分排除条件。
type MACFilterType uint8

const (
	MACFilterNew          MACFilterType = 1
	MACFilterKCB          MACFilterType = 2
	MACFilterST           MACFilterType = 4
	MACFilterGEM          MACFilterType = 8
	MACFilterHKConnect    MACFilterType = 16
	MACFilterBJ           MACFilterType = 32
	MACFilterApproval     MACFilterType = 64
	MACFilterRegistration MACFilterType = 128
)

// MACFilterFlags 将多个 MAC 排除条件合并成协议位图字节。
func MACFilterFlags(filters ...MACFilterType) uint8 {
	var flags uint8
	for _, filter := range filters {
		flags |= uint8(filter)
	}
	return flags
}

const macBoardMembersQuotesControlExtended byte = 1

// MACBoardMembersQuotesRequestBitmap 将字段位图转换为 0x122C 请求位图。
func MACBoardMembersQuotesRequestBitmap(fieldBitmap [20]byte, filters ...MACFilterType) [20]byte {
	fieldBitmap[17] = MACFilterFlags(filters...)
	fieldBitmap[19] |= macBoardMembersQuotesControlExtended
	return fieldBitmap
}
