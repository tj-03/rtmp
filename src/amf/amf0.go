package amf

import (
	"bytes"
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
	steen := binary.BigEndian.Uint16(v[:2])
	strlen := int(steen)
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

func EncodeAMF0(v interface{}) []byte {
	switch v.(type) {
	case float64:
		return encodeNumber(v.(float64))
	case int:
		return encodeNumber(float64(v.(int)))
	case bool:
		return encodeBoolean(v.(bool))
	case string:
		return encodeString(v.(string))
	case nil:
		return encodeNull()
	case Amf0Reference:
		return encodeReference(v.(Amf0Reference))
	case map[string]interface{}:
		return encodeObject(v.(map[string]interface{}))
	case Amf0ECMAArray:
		return encodeECMAArray(v.(Amf0ECMAArray))
	case time.Time:
		return encodeDate(v.(time.Time))
	case []interface{}:
		return encodeStrictArr(v.([]interface{}))
	case Amf0Xml:
		return encodeXml(v.(Amf0Xml))
	}
	return nil
}

func encodeNumber(v float64) []byte {
	msg := make([]byte, 1+8) // 1 header + 8 float64
	msg[0] = byte(amf0Number)
	binary.BigEndian.PutUint64(msg[1:], uint64(math.Float64bits(v)))
	return msg
}

func encodeBoolean(v bool) []byte {
	msg := make([]byte, 1+1) // 1 header + 1 boolean
	msg[0] = byte(amf0Boolean)
	if v {
		msg[1] = 0x1
	} else {
		msg[1] = 0x0
	}
	return msg
}

func encodeString(v string) []byte {
	var msg []byte
	if len(v) < 0xffff {
		msg = make([]byte, 1+2+len(v)) // 1 header + 2 length + length of string
		msg[0] = byte(amf0String)
		binary.BigEndian.PutUint16(msg[1:], uint16(len(v)))
		copy(msg[3:], v)
	} else {
		msg = make([]byte, 1+4+len(v)) // 1 header + 4 length + length of string
		msg[0] = byte(amf0StringExt)
		binary.BigEndian.PutUint32(msg[1:], uint32(len(v)))
		copy(msg[5:], v)
	}
	return msg
}

func encodeKey(v string) []byte {
	var msg []byte
	if len(v) < 0xffff {
		msg = make([]byte, 2+len(v)) // 1 header + 2 length + length of string
		binary.BigEndian.PutUint16(msg, uint16(len(v)))
		copy(msg[2:], v)
	} else {
		msg = make([]byte, 4+len(v)) // 1 header + 4 length + length of string
		binary.BigEndian.PutUint32(msg, uint32(len(v)))
		copy(msg[4:], v)
	}
	return msg
}

func encodeObject(v map[string]interface{}) []byte {
	buf := new(bytes.Buffer)
	var keys []string
	for k := range v {
		keys = append(keys, k)
	}
	for _, key := range keys {
		value := v[key]
		buf.Write(encodeKey(key))
		switch value.(type) {
		case int:
			buf.Write(EncodeAMF0(value.(int)))
		case float64:
			buf.Write(EncodeAMF0(value.(float64)))
		case string:
			buf.Write(EncodeAMF0(value.(string)))
		case bool:
			buf.Write(EncodeAMF0(value.(bool)))
		case time.Time:
			buf.Write(EncodeAMF0(value.(time.Time)))
		case Amf0Reference:
			buf.Write(EncodeAMF0(value.(Amf0Reference)))
		case nil:
			buf.Write(EncodeAMF0(nil))
		}
	}
	buf.Write(encodeObjectEnd())
	msg := make([]byte, 1+buf.Len()) // 1 header + length
	msg[0] = byte(amf0Object)
	copy(msg[1:], buf.Bytes())
	return msg
}

func encodeNull() []byte {
	return []byte{byte(amf0Null)}
}

func encodeReference(v Amf0Reference) []byte {
	msg := make([]byte, 1+2) // 1 header + 2 uint16
	msg[0] = byte(amf0Reference)
	binary.BigEndian.PutUint16(msg[1:], uint16(v))
	return msg
}

func encodeECMAArray(v Amf0ECMAArray) []byte {
	msg_body := encodeObject(v)
	summary_length := len(msg_body) - 4
	msg := make([]byte, 1+4+summary_length) // 1 header + 4 length + sum length
	msg[0] = byte(amf0Array)
	binary.BigEndian.PutUint32(msg[1:], uint32(len(v)))
	copy(msg[5:], msg_body[1:summary_length+1])
	return msg
}

func encodeObjectEnd() []byte {
	return []byte{0x00, 0x00, byte(amf0ObjectEnd)}
}

func encodeDate(v time.Time) []byte {
	msg := make([]byte, 1+8+2) // 1 header + 8 float64 + 2 timezone
	msg[0], msg[9], msg[10] = byte(amf0Date), 0x0, 0x0
	binary.BigEndian.PutUint64(msg[1:], uint64(v.UnixNano()/1000000))
	return msg
}

func encodeStrictArr(v []interface{}) []byte {
	buf := new(bytes.Buffer)
	StringExt := false
	for _, k := range v {
		switch k.(type) {
		case string:
			if len(k.(string)) > 0xffff {
				StringExt = true
			}
		}
	}
	for _, k := range v {
		if StringExt {
			msg := make([]byte, 1+4+len(k.(string))) // 1 header + 4 length + length of string
			msg[0] = byte(amf0StringExt)
			binary.BigEndian.PutUint32(msg[1:], uint32(len(k.(string))))
			copy(msg[5:], k.(string))
			buf.Write(msg)
			continue
		}
		buf.Write(EncodeAMF0(k))
	}
	msg := make([]byte, 1+8+buf.Len()) // 1 header + 8 array count + length
	msg[0] = byte(amf0StrictArr)
	binary.BigEndian.PutUint32(msg[1:], uint32(len(v)))
	copy(msg[9:], buf.Bytes())
	return msg
}

func encodeXml(v Amf0Xml) []byte {
	msg := make([]byte, 1+4+len(v)) // 1 header + 4 string length + string
	msg[0] = byte(amf0Xml)
	binary.BigEndian.PutUint32(msg[1:], uint32(len(v)))
	copy(msg[5:], v)
	return msg
}
