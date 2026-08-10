package adapter

import (
	"fmt"
	"math/big"
	"strings"
)

const base58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

func decodeBase58(value string) ([]byte, error) {
	if value == "" {
		return nil, fmt.Errorf("expected a base58 value")
	}
	number := big.NewInt(0)
	base := big.NewInt(58)
	for _, char := range value {
		index := strings.IndexRune(base58Alphabet, char)
		if index < 0 {
			return nil, fmt.Errorf("expected a base58 value")
		}
		number.Mul(number, base)
		number.Add(number, big.NewInt(int64(index)))
	}
	decoded := number.Bytes()
	for _, char := range value {
		if char != '1' {
			break
		}
		decoded = append([]byte{0}, decoded...)
	}
	return decoded, nil
}

func encodeBase58(value []byte) string {
	number := new(big.Int).SetBytes(value)
	base := big.NewInt(58)
	zero := big.NewInt(0)
	result := make([]byte, 0, len(value)*2)
	for number.Cmp(zero) > 0 {
		remainder := new(big.Int)
		number.DivMod(number, base, remainder)
		result = append(result, base58Alphabet[remainder.Int64()])
	}
	for _, byteValue := range value {
		if byteValue != 0 {
			break
		}
		result = append(result, '1')
	}
	for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
		result[left], result[right] = result[right], result[left]
	}
	return string(result)
}
