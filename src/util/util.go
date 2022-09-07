package util

import "encoding/binary"

func Uint32ToBuf(num uint32) []byte {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, num)
	return buf
}

func Uint16ToBuf(num uint16) []byte {
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf, num)
	return buf
}

func Max(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

func Min(a int, b int) int {
	if a < b {
		return a
	}
	return b
}
