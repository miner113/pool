package job

import (
	"encoding/hex"
	"fmt"
)

// ExtraNonce2Size is the per-miner extranonce2 length in bytes. Adjust if miners
// or downstream infrastructure expect a different size.
const ExtraNonce2Size = 8

// SplitCoinbase splits the raw coinbase transaction hex into coinb1 and coinb2
// parts for Stratum notify. The extranonce slot is placed immediately after the
// BIP34 height push within the coinbase scriptSig. coinb1 contains the updated
// script length and the height push; miners must insert extranonce1 +
// extranonce2 there before appending coinb2.
func SplitCoinbase(raw string, extranonce1 string, extranonce2Size int) (coinb1, coinb2 string, err error) {
	if raw == "" {
		return "", "", fmt.Errorf("empty coinbase txn")
	}
	if extranonce2Size <= 0 {
		return "", "", fmt.Errorf("invalid extranonce2 size")
	}

	tx, err := hex.DecodeString(raw)
	if err != nil {
		return "", "", fmt.Errorf("decode coinbase: %w", err)
	}

	// Locate the first (and only) coinbase input by finding the zero prevout + 0xffffffff index pattern.
	marker := []byte{
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0xff, 0xff, 0xff, 0xff,
	}
	idx := indexOf(tx, marker)
	if idx < 0 {
		return "", "", fmt.Errorf("coinbase marker not found")
	}

	scriptLenPos := idx + len(marker)
	if scriptLenPos >= len(tx) {
		return "", "", fmt.Errorf("missing script length")
	}

	scriptLen, n, err := readVarInt(tx[scriptLenPos:])
	if err != nil {
		return "", "", fmt.Errorf("script length: %w", err)
	}
	scriptLenStart := scriptLenPos
	scriptStart := scriptLenPos + n

	if len(tx) < scriptStart+int(scriptLen) {
		return "", "", fmt.Errorf("truncated scriptSig")
	}
	script := tx[scriptStart : scriptStart+int(scriptLen)]

	if len(script) == 0 {
		return "", "", fmt.Errorf("empty scriptSig")
	}
	heightPushLen := int(script[0]) + 1 // opcode byte + pushed height bytes
	if heightPushLen > len(script) {
		return "", "", fmt.Errorf("invalid height push len")
	}
	heightPart := script[:heightPushLen]
	remainingScript := script[heightPushLen:]

	// New script length accounts for extranonce1+2 inserted after height.
	extra1Bytes := len(extranonce1) / 2
	newScriptLen := len(script) + extra1Bytes + extranonce2Size
	newScriptLenVar := writeVarInt(uint64(newScriptLen))

	coinb1Bytes := make([]byte, 0, len(tx)+len(newScriptLenVar))
	coinb1Bytes = append(coinb1Bytes, tx[:scriptLenStart]...)
	coinb1Bytes = append(coinb1Bytes, newScriptLenVar...)
	coinb1Bytes = append(coinb1Bytes, heightPart...)

	coinb2Bytes := make([]byte, 0, len(tx)-scriptStart)
	coinb2Bytes = append(coinb2Bytes, remainingScript...)
	coinb2Bytes = append(coinb2Bytes, tx[scriptStart+int(scriptLen):]...)

	return hex.EncodeToString(coinb1Bytes), hex.EncodeToString(coinb2Bytes), nil
}

func readVarInt(b []byte) (uint64, int, error) {
	if len(b) == 0 {
		return 0, 0, fmt.Errorf("empty buffer")
	}
	switch b[0] {
	case 0xff:
		if len(b) < 9 {
			return 0, 0, fmt.Errorf("varint ff truncated")
		}
		return uint64(b[1]) | uint64(b[2])<<8 | uint64(b[3])<<16 | uint64(b[4])<<24 | uint64(b[5])<<32 | uint64(b[6])<<40 | uint64(b[7])<<48 | uint64(b[8])<<56, 9, nil
	case 0xfe:
		if len(b) < 5 {
			return 0, 0, fmt.Errorf("varint fe truncated")
		}
		return uint64(b[1]) | uint64(b[2])<<8 | uint64(b[3])<<16 | uint64(b[4])<<24, 5, nil
	case 0xfd:
		if len(b) < 3 {
			return 0, 0, fmt.Errorf("varint fd truncated")
		}
		return uint64(b[1]) | uint64(b[2])<<8, 3, nil
	default:
		return uint64(b[0]), 1, nil
	}
}

func writeVarInt(v uint64) []byte {
	switch {
	case v < 0xfd:
		return []byte{byte(v)}
	case v <= 0xffff:
		return []byte{0xfd, byte(v), byte(v >> 8)}
	case v <= 0xffffffff:
		return []byte{0xfe, byte(v), byte(v >> 8), byte(v >> 16), byte(v >> 24)}
	default:
		return []byte{0xff, byte(v), byte(v >> 8), byte(v >> 16), byte(v >> 24), byte(v >> 32), byte(v >> 40), byte(v >> 48), byte(v >> 56)}
	}
}

func indexOf(haystack, needle []byte) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		match := true
		for j := 0; j < len(needle); j++ {
			if haystack[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}
