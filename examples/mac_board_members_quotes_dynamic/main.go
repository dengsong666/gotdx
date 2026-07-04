package main

import (
	"log"

	"github.com/bensema/gotdx"
	"github.com/bensema/gotdx/examples/internal/exampleutil"
)

func main() {
	client := exampleutil.NewMACClient()
	defer client.Disconnect()

	fieldBitmap := gotdx.MACFieldBitmap(
		gotdx.MACPresetCommon,
		gotdx.MACFieldMainNetRatio,
		gotdx.MACFieldChangeUpType,
	)
	reply, err := client.MACBoardMembersQuotesDynamicWithFilter(
		"880761",
		10,
		uint16(gotdx.MACSortChangePct),
		uint8(gotdx.MACSortOrderDesc),
		fieldBitmap,
		gotdx.MACFilterST,
	)
	if err != nil {
		log.Fatalln(err)
	}

	log.Printf("mac_member_quote_dynamic total=%d count=%d fields=%d bitmap=%x",
		reply.Total, reply.Count, len(reply.ActiveFields), reply.FieldBitmap)

	for _, item := range reply.Stocks[:min(5, len(reply.Stocks))] {
		log.Printf("symbol=%s name=%s close=%v pre_close=%v turnover=%v pe_ttm=%v main_net_ratio=%v change_up_type=%v",
			item.Symbol,
			item.Name,
			item.Values["close"],
			item.Values["pre_close"],
			item.Values["turnover"],
			item.Values["pe_ttm"],
			item.Values["main_net_ratio"],
			item.Values["change_up_type"],
		)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
