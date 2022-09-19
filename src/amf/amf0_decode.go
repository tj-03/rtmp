package amf

import (
	"encoding/binary"
	"errors"
	"math"
)

const (
	amfNumberType    byte = 0
	amfBoolType      byte = 1
	amfStringType    byte = 2
	amfObjectType    byte = 3
	amfNullType      byte = 5
	amfUndefinedType byte = 6
	amfArrType       byte = 8
	amfObjEnd        byte = 9
)

func DecodeAMF0Sequence(data []byte) ([]interface{}, int, error) {
	offset := 0
	result := []interface{}{}
	for offset < len(data) {
		value, bytesRead, err := DecodeAMF0Value(data[offset:])
		if err != nil {
			return nil, offset, err
		}
		offset += bytesRead
		result = append(result, value)
	}
	return result, offset, nil
}

func DecodeAMF0Value(data []byte) (interface{}, int, error) {
	if len(data) == 0 || (len(data) < 2 && data[0] != amfNullType) {
		return nil, 0, errors.New("data slice too small")
	}
	amfType := data[0]
	switch amfType {
	case amfNumberType:
		val, bytesRead, err := decodeAMFNumber(data[1:])
		bytesRead += 1
		return val, bytesRead, err
	case amfBoolType:
		val, bytesRead, err := decodeAMFBool(data[1:])
		bytesRead += 1
		return val, bytesRead, err
	case amfStringType:
		val, bytesRead, err := decodeAMFString(data[1:])
		bytesRead += 1
		return val, bytesRead, err
	case amfObjectType:
		val, bytesRead, err := decodeAMFObject(data[1:])
		bytesRead += 1
		return val, bytesRead, err
	case amfNullType, amfUndefinedType:
		return nil, 1, nil
	case amfArrType:
		val, bytesRead, err := decodeAMFArr(data[1:])
		bytesRead += 1
		return val, bytesRead, err
	}

	//panic("unknown amf type")
	return nil, 0, errors.New("unknown type")
}

func decodeAMFNumber(data []byte) (float64, int, error) {
	if len(data) < 8 {
		return 0, 0, errors.New("invalid amf number type")
	}
	bits := binary.BigEndian.Uint64(data[0:8])
	floatNum := math.Float64frombits(bits)
	return floatNum, 8, nil
}

func decodeAMFBool(data []byte) (bool, int, error) {
	if len(data) < 1 {
		return false, 0, errors.New("invalid amf bool type")
	}
	return data[0] != 1, 1, nil
}

func decodeAMFString(data []byte) (string, int, error) {
	if len(data) < 2 {
		return "", 0, errors.New("invalid amf string type")
	}
	strSize := binary.BigEndian.Uint16(data[:2])
	if len(data) < 2+int(strSize) {
		return "", 0, errors.New("invalid amf string type")
	}

	str := string(data[2 : strSize+2])
	return str, 2 + int(strSize), nil
}

func decodeAMFArr(data []byte) (map[string]interface{}, int, error) {
	if len(data) < 5 {
		return nil, 0, errors.New("invalid amf ecma arr type")
	}
	length := binary.BigEndian.Uint32(data[:4])
	result := map[string]interface{}{}
	offset := 4
	for i := 0; i < int(length); i++ {
		propName, bytesRead, err := decodeAMFString(data[offset:])
		offset += bytesRead
		if err != nil {
			return nil, offset, err
		}
		val, bytesRead, err := DecodeAMF0Value(data[offset:])
		offset += bytesRead
		if err != nil {
			return nil, offset, err
		}
		result[propName] = val
	}
	return result, offset + 1, nil
}

func decodeAMFObject(data []byte) (map[string]interface{}, int, error) {
	if len(data) < 1 {
		return nil, 0, errors.New("invalid amf object type")
	}
	offset := 0
	object := map[string]interface{}{}
	for data[offset] != amfObjEnd {
		propName, bytesRead, err := decodeAMFString(data[offset:])
		offset += bytesRead
		if err != nil {
			return nil, offset, err
		}
		if propName == "" {
			if data[offset] == amfObjEnd {
				return object, offset + 1, nil
			} else {
				return nil, offset, errors.New("invalid object ending")
			}
		}
		val, bytesRead, err := DecodeAMF0Value(data[offset:])
		offset += bytesRead
		if err != nil {
			return nil, offset, err
		}
		object[propName] = val

	}
	return object, offset + 1, nil
}
