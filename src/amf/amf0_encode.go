package amf

import (
	"encoding/binary"
	"math"
)

func EncodeAMF0(vals ...interface{}) []byte {
	data := []byte{}
	for _, val := range vals {
		encodedVal := encodeAMFValue(val)
		data = append(data, encodedVal...)
	}
	return data
}

func encodeAMFValue(untypedVal interface{}) []byte {
	switch val := untypedVal.(type) {
	case int:
		return encodeAMFFloat64(float64(val))
	case float64:
		return encodeAMFFloat64(val)
	case bool:
		return encodeAMFBool(val)
	case string:
		return encodeAMFStr(val)
	case nil:
		return []byte{amfNullType}
	case map[string]interface{}:
		return encodeAMFObj(val)
	}
	//panic(fmt.Sprintln("unkown type", untypedVal))
	return nil
}

func encodeAMFFloat64(num float64) []byte {
	//type marker + 8 byte float
	buf := make([]byte, 9)
	buf[0] = amfNumberType
	binary.BigEndian.PutUint64(buf[1:], math.Float64bits(num))
	return buf
}
func encodeAMFBool(boolVal bool) []byte {
	var boolNum int
	if boolVal {
		boolNum = 1
	} else {
		boolNum = 0
	}
	return []byte{amfBoolType, byte(boolNum)}

}

func encodeAMFStr(str string) []byte {
	length := len(str)
	//string type marker + string length bytes + string data
	buf := make([]byte, 1+2+length)
	buf[0] = amfStringType
	binary.BigEndian.PutUint16(buf[1:3], uint16(length))
	copy(buf[3:], str)
	return buf
}

func encodeAMFObj(obj map[string]interface{}) []byte {
	buf := []byte{amfObjectType}
	for key := range obj {
		//ignore the string type marker
		encodedProp := encodeAMFStr(key)[1:]
		encodedVal := encodeAMFValue(obj[key])
		buf = append(buf, encodedProp...)
		buf = append(buf, encodedVal...)
	}
	objectEnd := []byte{0, 0, amfObjEnd}
	buf = append(buf, objectEnd...)
	return buf
}
