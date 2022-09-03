package message

import (
	"bufio"
	"encoding/binary"
	"fmt"

	"github.com/tj03/rtmp/src/logger"
)

//might not use, think i might use it to get prev chunk fo a msg stream/chunk stream
type ChunkStreams struct {
	chunks map[int]Chunk
}
type ChunkBasicHeader struct {
	Fmt uint8
	//is this the right data type?
	ChunkStreamId uint16
}

type ChunkMessageHeader struct {
	Timestamp uint32
	//3 bytes
	MessageLength uint32
	MessageTypeId uint8
	//4 bytes
	MessageStreamId uint32
	//3 bytes
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

type ChunkReader struct {
	chunkStreams map[int]ChunkMessageHeader
	ChunkSize    int
	conn         *bufio.ReadWriter
}

func (cReader *ChunkReader) Init(conn *bufio.ReadWriter, chunkSize int) {
	cReader.conn = conn
	cReader.ChunkSize = chunkSize
	cReader.chunkStreams = make(map[int]ChunkMessageHeader)
}

func (cReader *ChunkReader) ReadChunkFromStream() (error, Chunk) {
	chunkPayloadSize := cReader.ChunkSize
	reader := cReader.conn
	fmtCsid, err := reader.ReadByte()
	format := uint8(fmtCsid >> 6)
	csid := uint16(fmtCsid & 0b00111111)
	if err != nil {
		return err, Chunk{}
	}
	var chunkBasicHeader ChunkBasicHeader
	chunkBasicHeader.Fmt = format
	switch csid {
	//If 6 bit csid is 0, we have 1 byte csid (64 + 1 byte csid)
	case 0:
		var id byte
		id, err = reader.ReadByte()
		csid = uint16(id + 64)
	//If 6 bit csid is 1, we have 2 byte csid (64 + 2 byte csid)
	//this is wrong lol -> 3 byte calculation off, go to specs to see why
	case 1:
		data := make([]byte, 2)
		_, err = reader.Read(data)
		csid = 64 + binary.BigEndian.Uint16(data)
	}
	if err != nil {
		return err, Chunk{}
	}
	chunkBasicHeader.ChunkStreamId = csid

	var chunkMessageHeader ChunkMessageHeader
	//TODO: handle other fmt values
	switch format {
	case 0:
		headerSize := 11
		data := make([]byte, headerSize)
		n, err := reader.Read(data)
		if err != nil {
			return err, Chunk{}
		}
		fmt.Println("Message header", data)
		if n != headerSize {
			logger.ErrorLog.Fatalln("Header read improperly", data, n)
		}
		timestamp := binary.BigEndian.Uint32(append([]byte{0}, data[0:3]...))
		chunkMessageHeader.Timestamp = timestamp

		messageLength := binary.BigEndian.Uint32(append([]byte{0}, data[3:6]...))
		chunkMessageHeader.MessageLength = messageLength
		messageTypeId := data[6]
		chunkMessageHeader.MessageTypeId = messageTypeId
		chunkMessageHeader.MessageStreamId = binary.LittleEndian.Uint32(data[7:11])
		cReader.chunkStreams[int(csid)] = chunkMessageHeader
	case 1:
		headerSize := 7
		data := make([]byte, headerSize)
		n, err := reader.Read(data)
		if err != nil {
			return err, Chunk{}
		}
		fmt.Println("Message header", data)
		if n != headerSize {
			logger.ErrorLog.Fatalln("Header read improperly", data, n)
		}
		timestamp := binary.BigEndian.Uint32(append([]byte{0}, data[0:3]...))
		chunkMessageHeader.Timestamp = timestamp

		messageLength := binary.BigEndian.Uint32(append([]byte{0}, data[3:6]...))
		chunkMessageHeader.MessageLength = messageLength
		messageTypeId := data[6]
		chunkMessageHeader.MessageTypeId = messageTypeId
		if header, ok := cReader.chunkStreams[int(csid)]; ok {
			chunkMessageHeader.MessageStreamId = header.MessageStreamId
		}
	case 2:
		headerSize := 3
		data := make([]byte, headerSize)
		n, err := reader.Read(data)
		if err != nil {
			return err, Chunk{}
		}
		fmt.Println("Message header", data)
		if n != headerSize {
			logger.ErrorLog.Fatalln("Header read improperly", data, n)
		}
		if header, ok := cReader.chunkStreams[int(csid)]; ok {
			chunkMessageHeader = header
		}

		timestamp := binary.BigEndian.Uint32(append([]byte{0}, data[0:3]...))
		chunkMessageHeader.Timestamp = timestamp
	case 3:
		if header, ok := cReader.chunkStreams[int(csid)]; ok {
			chunkMessageHeader = header
		}
	}
	cReader.chunkStreams[int(csid)] = chunkMessageHeader
	chunk := Chunk{BasicHeader: chunkBasicHeader,
		MessageHeader: &chunkMessageHeader}
	//TODO: check if chunksize is greater than REMAINING message size -> have to use previous chunks
	size := chunkPayloadSize
	if int(chunk.MessageHeader.MessageLength) < size {
		size = int(chunk.MessageHeader.MessageLength)
	}
	fmt.Println("wtf?", size)
	data := make([]byte, size)
	_, err = reader.Read(data)
	if err != nil {
		return err, Chunk{}
	}
	chunk.ChunkData = data
	return nil, chunk

}
