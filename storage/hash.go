package storage

import (
	"crypto/sha256"
	"fmt"
)

// TODO type ProductId string

func ProductIdHash(productId string) string {
	// we need to hash the productId because there's
	// no length constraint on the input and file names are limited
	return fmt.Sprintf("%x", sha256.Sum256([]byte(productId)))
}
