package amf

import (
	"encoding/binary"
	"fmt"
	"math"
	"time"
)

func DecodeAMF0(v []byte) (interface{}, error) {
	result, _, err := decodeAMF0(v)
	return result, err
}

func decodeAMF0(v []byte) (interface{}, int, error) {
	switch v[0] {
	case amf0Number:
		return decodeNumber(v)
	case amf0Boolean:
		return decodeBoolean(v)
	case amf0String, amf0StringExt:
		return decodeString(v)
	case amf0Object:
		return decodeObject(v)
	case amf0Null:
		return nil, 1, nil
	case amf0Undefined:
		return nil, 1, nil
	case amf0Array:
		return decodeECMAArray(v)
	case amf0StrictArr:
		return decodeStrictArray(v)
	case amf0Date:
		return decodeDate(v)
	}
	return nil, 0, fmt.Errorf("unsupported type 0x%0X", v[0])
}

func decodeNumber(v []byte) (float64, int, error) {
	return math.Float64frombits(binary.BigEndian.Uint64(v[1:9])), 9, nil
}

func decodeBoolean(v []byte) (bool, int, error) {
	if v[1] == 0x1 {
		return true, 2, nil
	} else {
		return false, 2, nil
	}
}

func decodeUTF8(v []byte) (string, int) {
	strlen := int(binary.BigEndian.Uint16(v[:2]))
	s := string(v[2 : 2+strlen])
	return s, 2 + strlen
}

func decodeString(v []byte) (string, int, error) {
	if v[0] == amf0String {
		s, n := decodeUTF8(v[1:])
		return s, 1 + n, nil
	} else if v[0] == amf0StringExt {
		strlen := int(binary.BigEndian.Uint32(v[1:5]))
		s := string(v[5 : 5+strlen])
		return s, 5 + strlen, nil
	} else {
		return "", 0, fmt.Errorf("invalid string tag")
	}
}

func decodeECMAArray(v []byte) (ECMAArray, int, error) {
	result := make(map[string]interface{})
	num := binary.BigEndian.Uint32(v[1:5])
	offset := 5
	for i := uint32(0); i < num; i++ {
		key, nkey := decodeUTF8(v[offset:])
		offset += nkey
		value, nvalue, err := decodeAMF0(v[offset:])
		if err != nil {
			return nil, 0, err
		}
		offset += nvalue
		result[key] = value
	}
	return ECMAArray(result), offset + 3, nil
}

func decodeStrictArray(v []byte) ([]interface{}, int, error) {
	result := make([]interface{}, 0)
	num := binary.BigEndian.Uint32(v[1:5])
	offset := 5
	for i := uint32(0); i < num; i++ {
		value, nvalue, err := decodeAMF0(v[offset:])
		if err != nil {
			return nil, 0, err
		}
		offset += nvalue
		result = append(result, value)
	}
	return result, offset, nil
}

func DecodeBytes(v []byte) ([]interface{}, int, error) {
	result := make([]interface{}, 0)
	offset := 0
	for offset < len(v) {
		value, nvalue, err := decodeAMF0(v[offset:])
		if err != nil {
			return nil, 0, err
		}
		offset += nvalue
		result = append(result, value)
	}
	return result, offset, nil
}

func decodeDate(v []byte) (time.Time, int, error) {
	t := int64(math.Float64frombits(binary.BigEndian.Uint64(v[1:9])) * 1000000)
	if v[9] != 0x00 || v[10] != 0x00 {
		return time.Unix(0, 0), 0, fmt.Errorf("invalid timezone")
	}
	return time.Unix(0, t), 11, nil
}

func decodeObject(v []byte) (map[string]interface{}, int, error) {
	result := make(map[string]interface{})
	offset := 1
	for {
		key, nkey := decodeUTF8(v[offset:])
		offset += nkey
		if key == "" {
			if v[offset] == byte(amf0ObjectEnd) {
				offset++
				break
			} else {
				return nil, 0, fmt.Errorf("invalid end of object")
			}
		}
		value, nvalue, err := decodeAMF0(v[offset:])
		if err != nil {
			return nil, 0, err
		}
		offset += nvalue
		result[key] = value
	}
	return result, offset, nil
}
