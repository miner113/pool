package job

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
)

// BuildHeaderJuno constructs the Juno/Zcash-style header:
// version | prevhash | merkleroot | blockcommitment | time | bits | nonce(32) | solution (with CompactSize length)
// All multibyte fields are little-endian in the header; prev/merkle/blockcommitment are byte-reversed from RPC hex.
func BuildHeaderJuno(version int64, prevHashHex string, merkleRootHex string, blockCommitHex string, nTime uint32, bitsHex string, nonceHex string, solution []byte) ([]byte, error) {
	// version + prev + merkle + blockcommit + time + bits + nonce + solution
	// solution prefixed by compact size
	header := make([]byte, 0, 4+32+32+32+4+4+32+len(solution)+9)

	v := make([]byte, 4)
	binary.LittleEndian.PutUint32(v, uint32(version))
	header = append(header, v...)

	prev, err := hex.DecodeString(prevHashHex)
	if err != nil || len(prev) != 32 {
		return nil, fmt.Errorf("prevhash decode")
	}
	reverseBytes(prev)
	header = append(header, prev...)

	mr, err := hex.DecodeString(merkleRootHex)
	if err != nil || len(mr) != 32 {
		return nil, fmt.Errorf("merkleroot decode")
	}
	reverseBytes(mr) // Convert from RPC big-endian to header little-endian
	header = append(header, mr...)

	bc, err := hex.DecodeString(blockCommitHex)
	if err != nil || len(bc) != 32 {
		return nil, fmt.Errorf("blockcommit decode")
	}
	reverseBytes(bc)
	header = append(header, bc...)

	t := make([]byte, 4)
	binary.LittleEndian.PutUint32(t, nTime)
	header = append(header, t...)

	bits, err := hex.DecodeString(bitsHex)
	if err != nil || len(bits) != 4 {
		return nil, fmt.Errorf("bits decode")
	}
	reverseBytes(bits)
	header = append(header, bits...)

	nonce, err := hex.DecodeString(nonceHex)
	if err != nil || len(nonce) != 32 {
		return nil, fmt.Errorf("nonce decode")
	}
	header = append(header, nonce...)

	header = append(header, writeVarInt(uint64(len(solution)))...)
	header = append(header, solution...)

	return header, nil
}

// BuildBlockJuno assembles the full block hex string from the header components and tx list.
// txs should NOT include the coinbase; coinbaseHex is inserted first.
func BuildBlockJuno(version int64, prevHashHex, merkleRootLE, blockCommitHex string, nTime uint32, bitsHex string, nonceHex string, solution []byte, coinbaseHex string, txs []string) (string, error) {
	header, err := BuildHeaderJuno(version, prevHashHex, merkleRootLE, blockCommitHex, nTime, bitsHex, nonceHex, solution)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString(hex.EncodeToString(header))
	// tx count
	count := 1 + len(txs)
	b.WriteString(hex.EncodeToString(writeVarInt(uint64(count))))
	b.WriteString(coinbaseHex)
	for _, tx := range txs {
		b.WriteString(tx)
	}
	return b.String(), nil
}

// HashHeader computes double-SHA256 of the header.
func HashHeader(header []byte) []byte {
	return doubleSHA(header)
}

// TargetFromBits converts compact bits hex to full target.
func TargetFromBits(bitsHex string) (*big.Int, error) {
	bits, err := hex.DecodeString(bitsHex)
	if err != nil {
		return nil, fmt.Errorf("decode bits: %w", err)
	}
	if len(bits) != 4 {
		return nil, fmt.Errorf("bits len %d", len(bits))
	}
	exp := bits[0]
	mantissa := uint32(bits[1])<<16 | uint32(bits[2])<<8 | uint32(bits[3])
	target := new(big.Int).SetUint64(uint64(mantissa))
	shift := int(exp) - 3
	target = target.Lsh(target, uint(8*shift))
	return target, nil
}

// TargetFromHex parses a big-endian target hex string.
func TargetFromHex(hexStr string) (*big.Int, error) {
	b, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, err
	}
	return new(big.Int).SetBytes(b), nil
}

func reverseCopy(b []byte) []byte {
	out := make([]byte, len(b))
	copy(out, b)
	reverseBytes(out)
	return out
}

// ReverseHex returns hex string of reversed byte slice.
func ReverseHex(b []byte) string {
	return hex.EncodeToString(reverseCopy(b))
}

// BuildHeaderBlobJuno creates the header blob for xmrig (without solution)
// Format: version(4) + prevhash(32) + merkleroot(32) + blockcommit(32) + time(4) + bits(4) + nonce_placeholder(32)
// Returns hex-encoded string suitable for xmrig blob field.
func BuildHeaderBlobJuno(version int64, prevHashHex, merkleRootHex, blockCommitHex string, curTime int64, bitsHex string) (string, error) {
	header := make([]byte, 0, 4+32+32+32+4+4+32)

	v := make([]byte, 4)
	binary.LittleEndian.PutUint32(v, uint32(version))
	header = append(header, v...)

	prev, err := hex.DecodeString(prevHashHex)
	if err != nil || len(prev) != 32 {
		return "", fmt.Errorf("prevhash decode")
	}
	reverseBytes(prev)
	header = append(header, prev...)

	mr, err := hex.DecodeString(merkleRootHex)
	if err != nil || len(mr) != 32 {
		return "", fmt.Errorf("merkleroot decode")
	}
	reverseBytes(mr) // Convert from RPC big-endian to header little-endian
	header = append(header, mr...)

	bc, err := hex.DecodeString(blockCommitHex)
	if err != nil || len(bc) != 32 {
		return "", fmt.Errorf("blockcommit decode")
	}
	reverseBytes(bc)
	header = append(header, bc...)

	t := make([]byte, 4)
	binary.LittleEndian.PutUint32(t, uint32(curTime))
	header = append(header, t...)

	bits, err := hex.DecodeString(bitsHex)
	if err != nil || len(bits) != 4 {
		return "", fmt.Errorf("bits decode")
	}
	reverseBytes(bits)
	header = append(header, bits...)

	// Nonce placeholder: 32 zero bytes (miner fills this in)
	noncePlaceholder := make([]byte, 32)
	header = append(header, noncePlaceholder...)

	return hex.EncodeToString(header), nil
}

// TargetToCompact converts difficulty to a compact hex target for xmrig.
// xmrig expects a 64-bit little-endian target value as 16 hex chars.
// The target is: maxTarget64 / difficulty
func TargetToCompact(difficulty float64, bitsHex string) string {
	if difficulty <= 0 {
		difficulty = 1
	}

	// Maximum 64-bit value
	maxTarget64 := uint64(0xFFFFFFFFFFFFFFFF)

	// Calculate target = maxTarget64 / difficulty
	target := uint64(float64(maxTarget64) / difficulty)

	// Convert to little-endian 8-byte buffer
	targetBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(targetBytes, target)

	return hex.EncodeToString(targetBytes)
}

// GetHeaderInputJuno returns the header input for RandomX hashing (header without solution).
func GetHeaderInputJuno(version int64, prevHashHex, merkleRootHex, blockCommitHex string, curTime int64, bitsHex string, nonce []byte) ([]byte, error) {
	header := make([]byte, 0, 4+32+32+32+4+4+32)

	v := make([]byte, 4)
	binary.LittleEndian.PutUint32(v, uint32(version))
	header = append(header, v...)

	prev, err := hex.DecodeString(prevHashHex)
	if err != nil || len(prev) != 32 {
		return nil, fmt.Errorf("prevhash decode")
	}
	reverseBytes(prev)
	header = append(header, prev...)

	mr, err := hex.DecodeString(merkleRootHex)
	if err != nil || len(mr) != 32 {
		return nil, fmt.Errorf("merkleroot decode")
	}
	reverseBytes(mr) // Convert from RPC big-endian to header little-endian
	header = append(header, mr...)

	bc, err := hex.DecodeString(blockCommitHex)
	if err != nil || len(bc) != 32 {
		return nil, fmt.Errorf("blockcommit decode")
	}
	reverseBytes(bc)
	header = append(header, bc...)

	t := make([]byte, 4)
	binary.LittleEndian.PutUint32(t, uint32(curTime))
	header = append(header, t...)

	bits, err := hex.DecodeString(bitsHex)
	if err != nil || len(bits) != 4 {
		return nil, fmt.Errorf("bits decode")
	}
	reverseBytes(bits)
	header = append(header, bits...)

	if len(nonce) != 32 {
		return nil, fmt.Errorf("nonce must be 32 bytes")
	}
	header = append(header, nonce...)

	return header, nil
}
