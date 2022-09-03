package amf

import (
	"encoding/binary"
	"io"
)

type AMFVersion uint8

const (
	AMF0 AMFVersion = 0x1
	AMF3 AMFVersion = 0x3
)

type ECMAArray map[string]interface{}

const (
	amf0Number    byte = 0x00
	amf0Boolean        = 0x01
	amf0String         = 0x02
	amf0Object         = 0x03
	amf0Null           = 0x05
	amf0Undefined      = 0x06
	amf0Reference      = 0x07
	amf0Array          = 0x08
	amf0ObjectEnd      = 0x09
	amf0StrictArr      = 0x0a
	amf0Date           = 0x0b
	amf0StringExt      = 0x0c
)

const (
	amf3Undefined    byte = 0x00
	amf3Null              = 0x01
	amf3False             = 0x02
	amf3True              = 0x03
	amf3Integer           = 0x04
	amf3Double            = 0x05
	amf3String            = 0x06
	amf3Date              = 0x08
	amf3Array             = 0x09
	amf3Object            = 0x0a
	amf3ByteArray         = 0x0c
	amf3VectorInt         = 0x0d
	amf3VectorUint        = 0x0d
	amf3VectorDouble      = 0x0d
	amf3VectorObject      = 0x0d
)

func writeBytes(n int, w io.Writer, data []byte) (int, error) {
	newn, err := w.Write(data)
	return n + newn, err
}

func writeData(n int, w io.Writer, order binary.ByteOrder, data interface{}) (int, error) {
	err := binary.Write(w, order, data)
	newn := binary.Size(data)
	return n + newn, err
}
