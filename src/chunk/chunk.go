package chunk

import (
	"bufio"
	"encoding/binary"
	"io"
)

//might not use, think i might use it to get prev chunk fo a msg stream/chunk stream
type ChunkStreams struct {
	chunks map[int]Chunk
}
type ChunkBasicHeader struct {
	fmt int8
	//is this the right data type?
	chunkStreamId uint16
}

type ChunkMessageHeader struct {
	messageType int
	//3 bytes
	messageLength uint32
	messageTypeId uint8
	//4 bytes
	messageStreamId uint32
	//3 bytes
	timestamp uint32
}

type Chunk struct {
	BasicHeader   ChunkBasicHeader
	MessageHeader *ChunkMessageHeader
	//0 or 4 bytes
	ExtendedTimestamp uint32
	ChunkData         []byte
}

func handleExtendedTimestamp() {

}
func NewChunkFromStream(reader *bufio.Reader, chunkPayloadSize int) (error, Chunk) {
	fmtCsid, err := reader.ReadByte()
	format := int8(fmtCsid >> 6)
	csid := uint16(fmtCsid & 0b00111111)
	if err != nil {
		return err, Chunk{}
	}
	var chunkBasicHeader ChunkBasicHeader
	chunkBasicHeader.fmt = format
	switch csid {
	//If 6 bit csid is 0, we have 1 byte csid (64 + 1 byte csid)
	case 0:
		var id byte
		id, err = reader.ReadByte()
		csid = uint16(id + 64)
	//If 6 bit csid is 1, we have 2 byte csid (64 + 2 byte csid)
	case 1:
		data := make([]byte, 2)
		_, err = reader.Read(data)
		csid = 64 + binary.BigEndian.Uint16(data)
	}
	if err != nil {
		return err, Chunk{}
	}
	chunkBasicHeader.chunkStreamId = csid

	var chunkMessageHeader ChunkMessageHeader
	//TODO: handle other fmt values
	switch format {
	case 0:
		data := make([]byte, 4)
		_, err = io.ReadAtLeast(reader, data, 3)
		if err != nil {
			return err, Chunk{}
		}
		timestamp := binary.BigEndian.Uint32(data)
		chunkMessageHeader.timestamp = timestamp

		_, err = reader.Read(data)
		if err != nil {
			return err, Chunk{}
		}
		messageLength := binary.BigEndian.Uint32(data)
		chunkMessageHeader.messageLength = messageLength
		messageTypeId, err := reader.ReadByte()
		if err != nil {
			return err, Chunk{}
		}
		chunkMessageHeader.messageTypeId = messageTypeId

		data = data[:4]
		_, err = reader.Read(data)
		if err != nil {
			return err, Chunk{}
		}
		chunkMessageHeader.messageStreamId = binary.BigEndian.Uint32(data)

	}
	chunk := Chunk{BasicHeader: chunkBasicHeader,
		MessageHeader: &chunkMessageHeader}
	//TODO: check if chunksize is greater than remaining message size
	data := make([]byte, chunkPayloadSize)
	_, err = reader.Read(data)
	if err != nil {
		return err, Chunk{}
	}
	chunk.ChunkData = data
	return nil, chunk

}
