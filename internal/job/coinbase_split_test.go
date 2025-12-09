package job

import "testing"

func TestSplitCoinbaseJunoSample(t *testing.T) {
	raw := "050000800a27a726f04dec4d0000000094840000010000000000000000000000000000000000000000000000000000000000000000ffffffff050394840000ffffffff01807c814a000000001976a91434baea3b306f95b5b5f1a455c4febfe52fc9022088ac000000"
	ex1 := "00000001"
	coinb1, coinb2, err := SplitCoinbase(raw, ex1, ExtraNonce2Size)
	if err != nil {
		t.Fatalf("split failed: %v", err)
	}

	expected1 := "050000800a27a726f04dec4d0000000094840000010000000000000000000000000000000000000000000000000000000000000000ffffffff1103948400"
	expected2 := "00ffffffff01807c814a000000001976a91434baea3b306f95b5b5f1a455c4febfe52fc9022088ac000000"

	if coinb1 != expected1 {
		t.Fatalf("coinb1 mismatch\n got: %s\nwant: %s", coinb1, expected1)
	}
	if coinb2 != expected2 {
		t.Fatalf("coinb2 mismatch\n got: %s\nwant: %s", coinb2, expected2)
	}
}
