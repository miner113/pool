package job

import "testing"

func TestBuildMerkleBranchesSingleTx(t *testing.T) {
	txHashes := []string{"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}
	branches := BuildMerkleBranches(txHashes)
	expected := []string{"efcdab8967452301efcdab8967452301efcdab8967452301efcdab8967452301"}
	if len(branches) != len(expected) {
		t.Fatalf("branch len mismatch got %d want %d", len(branches), len(expected))
	}
	for i := range expected {
		if branches[i] != expected[i] {
			t.Fatalf("branch %d mismatch\n got: %s\nwant: %s", i, branches[i], expected[i])
		}
	}
}

func TestBuildMerkleBranchesEmpty(t *testing.T) {
	if branches := BuildMerkleBranches(nil); branches != nil {
		t.Fatalf("expected nil branches for empty input, got %v", branches)
	}
}
