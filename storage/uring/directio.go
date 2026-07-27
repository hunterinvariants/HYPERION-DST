package uring

import "fmt"

const DefaultAlignment = 4096

func IsAligned(offset, length int64, alignment int) bool {
	if alignment <= 0 {
		return false
	}
	a := int64(alignment)
	return offset%a == 0 && length%a == 0
}

func ValidateDirectIO(offset int64, buffer []byte, alignment int) error {
	if !IsAligned(offset, int64(len(buffer)), alignment) {
		return fmt.Errorf("uring: O_DIRECT offset/length must be %d-byte aligned", alignment)
	}
	return nil
}
