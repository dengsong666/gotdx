package gotdx

import "testing"

func TestMACFieldBitmapPresets(t *testing.T) {
	defaultBitmap := DefaultMACBoardMembersQuotesFieldBitmap()
	expectedDefault := [20]byte{0xff, 0xfc, 0xe1, 0xcc, 0x3f, 0x08, 0x03, 0x01}
	if defaultBitmap != expectedDefault {
		t.Fatalf("default bitmap mismatch: got %x want %x", defaultBitmap, expectedDefault)
	}

	common := MACFieldBitmap(MACPresetCommon)
	expectedCommon := [20]byte{0xff, 0xfc, 0xa1, 0xcc, 0x3f, 0x28, 0x03, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x03}
	if common != expectedCommon {
		t.Fatalf("common preset literal mismatch: got %x want %x", common, expectedCommon)
	}

	custom := MACFieldBitmap(MACPresetOHLC, MACFieldAHCode)
	expected := MACBoardMembersQuotesFieldBitmapFromBits(1, 2, 3, 4, 0x4a)
	if custom != expected {
		t.Fatalf("custom bitmap mismatch: got %x want %x", custom, expected)
	}

	debug := MACFieldBitmap(MACPresetDebug)
	for i, b := range debug {
		if b != 0xff {
			t.Fatalf("debug byte %d = %#x", i, b)
		}
	}
}

func TestMACFieldAliases(t *testing.T) {
	if MACFieldBid2Volume != MACFieldLimitUpCount {
		t.Fatal("bid2 volume alias should match limit up count")
	}
	if MACFieldAsk5Volume != MACFieldDownCount {
		t.Fatal("ask5 volume alias should match down count")
	}
	if MACFieldPreIPOV != MACFieldPreIOPV {
		t.Fatal("legacy pre IPOV alias should match pre IOPV")
	}
	if MACFieldConstantNegOne != MACFieldSafetyScore {
		t.Fatal("legacy safety-score alias should match latest field")
	}
}

func TestMACBoardMembersQuotesRequestBitmap(t *testing.T) {
	bitmap := MACFieldBitmap(MACFieldMainNetRatio, MACFieldChangeAt1000)
	requestBitmap := MACBoardMembersQuotesRequestBitmap(bitmap, MACFilterKCB, MACFilterST, MACFilterBJ)

	if requestBitmap[0x6c/8]&(1<<(0x6c%8)) == 0 {
		t.Fatalf("main net ratio bit missing: %x", requestBitmap)
	}
	if requestBitmap[0x90/8]&(1<<(0x90%8)) == 0 {
		t.Fatalf("change-at-1000 bit missing: %x", requestBitmap)
	}
	if requestBitmap[17] != MACFilterFlags(MACFilterKCB, MACFilterST, MACFilterBJ) {
		t.Fatalf("unexpected filter byte: %#x", requestBitmap[17])
	}
	if requestBitmap[19]&1 == 0 {
		t.Fatalf("extended control bit missing: %#x", requestBitmap[19])
	}
}
