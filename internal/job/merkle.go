package job

import (
	"crypto/sha256"
	"encoding/hex"
)

// BuildMerkleBranches returns the merkle branches for the coinbase (assumed
// to be the first leaf) given the list of transaction hashes (hex, big-endian)
// from the template. Branches are encoded as little-endian hex as expected by
// stratum miners.
func BuildMerkleBranches(txHashes []string) []string {
	if len(txHashes) == 0 {
		return nil
	}

	leaves := make([][]byte, 1, len(txHashes)+1)
	leaves[0] = nil // coinbase placeholder
	for _, h := range txHashes {
		b, err := hex.DecodeString(h)
		if err != nil {
			return nil
		}
		// store internal byte order (little-endian) for hashing
		reverseBytes(b)
		leaves = append(leaves, b)
	}

	idx := 0 // coinbase position
	var branches []string

	for len(leaves) > 1 {
		if len(leaves)%2 == 1 {
			leaves = append(leaves, leaves[len(leaves)-1])
		}
		siblingIdx := idx ^ 1
		sibling := leaves[siblingIdx]
		if sibling == nil {
			branches = append(branches, "")
		} else {
			branches = append(branches, hex.EncodeToString(sibling))
		}

		next := make([][]byte, 0, len(leaves)/2)
		for i := 0; i < len(leaves); i += 2 {
			a := leaves[i]
			b := leaves[i+1]
			if a == nil || b == nil {
				next = append(next, nil)
				continue
			}
			h := sha256.Sum256(append(a, b...))
			h = sha256.Sum256(h[:])
			buf := make([]byte, len(h))
			copy(buf, h[:])
			next = append(next, buf)
		}
		idx = idx / 2
		leaves = next
	}

	return branches
}

func reverseBytes(b []byte) {
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
}

// ComputeMerkleRoot returns the merkle root (little-endian hex) given the
// coinbase transaction hex and precomputed branches (little-endian hex).
func ComputeMerkleRoot(coinbaseHex string, branches []string) (string, error) {
	cb, err := hex.DecodeString(coinbaseHex)
	if err != nil {
		return "", err
	}
	coinHash := doubleSHA(cb)
	reverseBytes(coinHash)

	root := coinHash
	for _, br := range branches {
		if br == "" {
			root = doubleSHA(append(root, root...))
			continue
		}
		sib, err := hex.DecodeString(br)
		if err != nil {
			return "", err
		}
		root = doubleSHA(append(root, sib...))
	}

	// return little-endian hex
	reverseBytes(root)
	return hex.EncodeToString(root), nil
}

func doubleSHA(b []byte) []byte {
	h := sha256.Sum256(b)
	h = sha256.Sum256(h[:])
	buf := make([]byte, len(h))
	copy(buf, h[:])
	return buf
}
