package api

import (
	"fmt"
	"strings"
)

func ethereumAddress(value string) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) != 42 || !strings.HasPrefix(value, "0x") {
		return "", fmt.Errorf("expected an Ethereum address")
	}
	for _, char := range value[2:] {
		if !(char >= '0' && char <= '9') && !(char >= 'a' && char <= 'f') && !(char >= 'A' && char <= 'F') {
			return "", fmt.Errorf("expected an Ethereum address")
		}
	}
	return strings.ToLower(value), nil
}
