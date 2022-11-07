package message

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/tj03/rtmp/src/logger"
	"github.com/tj03/rtmp/src/util"
)

type Connection interface {
	io.Reader
	io.Writer
	io.ByteReader
}

type ChunkBasicHeader struct {
	Fmt uint8
	//is this the right data type?
	ChunkStreamId uint64
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
	MessageHeader ChunkMessageHeader
	//0 or 4 bytes
	ExtendedTimestamp uint32
	ChunkData         []byte
}

func ChunksToBytes(chunks []Chunk) ([]byte, error) {
	if len(chunks) == 0 {
		return nil, errors.New("empty chunks")
	}
	bytes := []byte{}

	for _, chunk := range chunks {

		encodedChunk := []byte{}
		format := chunk.BasicHeader.Fmt
		if format > 3 {
			return nil, fmt.Errorf("invalid basic header format: fmt value =  %d", format)
		}

		fmtCsid := make([]byte, 1)
		fmtCsid[0] = format << 6
		chunkStreamId := chunk.BasicHeader.ChunkStreamId

		//encode fmt/csid
		if chunkStreamId >= 2 && chunkStreamId <= 63 {
			fmtCsid[0] |= byte(chunkStreamId)
		} else if chunkStreamId >= 64 && chunkStreamId <= 319 {
			fmtCsid = append(fmtCsid, byte(chunkStreamId-64))
		} else if chunkStreamId >= 320 && chunkStreamId <= 65599 {
			fmtCsid[0] |= 1
			secondByte := byte((chunkStreamId - 64) & 0x0000000f)
			thirdByte := byte(((chunkStreamId - 64) & 0x000000f0) >> 8)
			fmtCsid = append(fmtCsid, secondByte, thirdByte)
		} else {
			return nil, fmt.Errorf("chunk stream id too large %d", chunkStreamId)
		}
		encodedChunk = append(encodedChunk, fmtCsid...)

		//encode message header
		buf := make([]byte, 4)
		if format <= 2 {
			//timestamp is 3 bytes
			binary.BigEndian.PutUint32(buf, chunk.MessageHeader.Timestamp)
			//make sure this is getting the 3 least signigifcant bytes
			encodedChunk = append(encodedChunk, buf[1:4]...)
		}
		if format <= 1 {
			//message length is 3 bytes
			binary.BigEndian.PutUint32(buf, chunk.MessageHeader.MessageLength)
			encodedChunk = append(encodedChunk, buf[1:4]...)
			encodedChunk = append(encodedChunk, chunk.MessageHeader.MessageTypeId)
		}
		if format == 0 {
			//message streamId
			binary.LittleEndian.PutUint32(buf, chunk.MessageHeader.MessageStreamId)
			encodedChunk = append(encodedChunk, buf...)
		}
		encodedChunk = append(encodedChunk, chunk.ChunkData...)
		bytes = append(bytes, encodedChunk...)
	}
	return bytes, nil
}

type ChunkStreamer struct {
	chunkStreams    map[int]ChunkMessageHeader
	bytesLeftToRead int
	ChunkSize       uint32
	conn            Connection
}

func (cStreamer *ChunkStreamer) Init(conn Connection, chunkSize uint32) {
	cStreamer.conn = conn
	cStreamer.ChunkSize = chunkSize
	cStreamer.bytesLeftToRead = 0
	cStreamer.chunkStreams = make(map[int]ChunkMessageHeader)
}

func (cStreamer *ChunkStreamer) WriteChunksToStream(chunks []Chunk) error {
	data, err := ChunksToBytes(chunks)
	if err != nil {
		return err
	}

	_, err = cStreamer.conn.Write(data)
	if err != nil {
		logger.ErrorLog.Println(err)
		return err
	}
	return nil
}

//https://rtmp.veriskope.com/docs/spec/#531chunk-format
func (cStreamer *ChunkStreamer) ReadChunkFromStream() (Chunk, int, error) {
	chunkPayloadSize := cStreamer.ChunkSize
	reader := cStreamer.conn
	fmtCsid, err := reader.ReadByte()
	format := uint8(fmtCsid >> 6)
	csid := uint16(fmtCsid & 0b00111111)
	bytesRead := 1
	if err != nil {
		return Chunk{}, bytesRead, err
	}

	//https://rtmp.veriskope.com/docs/spec/#531chunk-format
	var chunkBasicHeader ChunkBasicHeader
	chunkBasicHeader.Fmt = format
	switch csid {
	//If 6 bit csid is 0, we have 1 byte csid (64 + 1 byte csid)
	case 0:
		var id byte
		id, err = reader.ReadByte()
		csid = uint16(id + 64)
		bytesRead++
	//If 6 bit csid is 1, we have 2 byte csid (64 + 2 byte csid)
	case 1:
		data := make([]byte, 2)
		n, err := reader.Read(data)
		bytesRead += n
		if err != nil {
			logger.ErrorLog.Println(err)
			return Chunk{}, 0, err
		}
		if n != 2 {
			logger.ErrorLog.Println("Basic header read incorrectly - Supposed to read 2 - read", n)
		}
		csid = 64 + binary.BigEndian.Uint16(data)
	}
	if err != nil {
		return Chunk{}, 0, err
	}
	chunkBasicHeader.ChunkStreamId = uint64(csid)

	//https://rtmp.veriskope.com/docs/spec/#5312-chunk-message-header
	//No extended timestamp handling in this implementation
	var chunkMessageHeader ChunkMessageHeader

	switch format {
	case 0:
		headerSize := 11
		data := make([]byte, headerSize)
		n, err := reader.Read(data)
		bytesRead += n
		if err != nil {
			return Chunk{}, bytesRead, err
		}
		if n != headerSize {
			logger.ErrorLog.Println("Header read improperly", data, n)
			return Chunk{}, bytesRead, fmt.Errorf("header read improperly")
		}
		timestamp := binary.BigEndian.Uint32(append([]byte{0}, data[0:3]...))
		chunkMessageHeader.Timestamp = timestamp

		messageLength := binary.BigEndian.Uint32(append([]byte{0}, data[3:6]...))
		chunkMessageHeader.MessageLength = messageLength
		messageTypeId := data[6]
		chunkMessageHeader.MessageTypeId = messageTypeId
		chunkMessageHeader.MessageStreamId = binary.LittleEndian.Uint32(data[7:11])
		cStreamer.chunkStreams[int(csid)] = chunkMessageHeader
	case 1:
		headerSize := 7
		data := make([]byte, headerSize)
		n, err := reader.Read(data)
		bytesRead += n
		if err != nil {
			return Chunk{}, bytesRead, err
		}
		if n != headerSize {
			logger.ErrorLog.Println("Header read improperly", data, n)
			return Chunk{}, bytesRead, fmt.Errorf("header read improperly")
		}
		timestamp := binary.BigEndian.Uint32(append([]byte{0}, data[0:3]...))
		chunkMessageHeader.Timestamp = timestamp

		messageLength := binary.BigEndian.Uint32(append([]byte{0}, data[3:6]...))
		chunkMessageHeader.MessageLength = messageLength
		messageTypeId := data[6]
		chunkMessageHeader.MessageTypeId = messageTypeId
		if header, ok := cStreamer.chunkStreams[int(csid)]; ok {
			chunkMessageHeader.MessageStreamId = header.MessageStreamId
		}
	case 2:
		headerSize := 3
		data := make([]byte, headerSize)
		n, err := reader.Read(data)
		bytesRead += n
		if err != nil {
			return Chunk{}, bytesRead, err
		}
		if n != headerSize {
			logger.ErrorLog.Println("Header read improperly", data, n)
			return Chunk{}, bytesRead, fmt.Errorf("header read improperly")
		}
		if header, ok := cStreamer.chunkStreams[int(csid)]; ok {
			chunkMessageHeader = header
		} else {
			errMsg := fmt.Sprintf("Client asked for previous chunk but there's no previous chunk. Chunk stream d = %d. FMT = %d\n", csid, format)
			logger.ErrorLog.Println(errMsg)
			//panic(errMsg)
		}

		timestamp := binary.BigEndian.Uint32(append([]byte{0}, data[0:3]...))
		chunkMessageHeader.Timestamp = timestamp
	case 3:
		if header, ok := cStreamer.chunkStreams[int(csid)]; ok {
			chunkMessageHeader = header
		} else {
			errMsg := fmt.Sprintf("Client asked for previous chunk but there's no previous chunk. Chunk stream d = %d. FMT = %d\n", csid, format)
			logger.ErrorLog.Println(errMsg)
			//panic(errMsg)
		}
	}
	cStreamer.chunkStreams[int(csid)] = chunkMessageHeader
	chunk := Chunk{
		BasicHeader:   chunkBasicHeader,
		MessageHeader: chunkMessageHeader}

	if cStreamer.bytesLeftToRead < 0 {
		panic("bytes left to read < 0")
	}
	if cStreamer.bytesLeftToRead == 0 {
		cStreamer.bytesLeftToRead = int(chunkMessageHeader.MessageLength)
	}
	bytesLeft := cStreamer.bytesLeftToRead
	size := util.Min(bytesLeft, int(chunkPayloadSize))

	cStreamer.bytesLeftToRead -= size

	data := make([]byte, size)
	n, err := reader.Read(data)
	bytesRead += n
	if err != nil {
		return Chunk{}, bytesRead, err
	}
	if n != size {
		return Chunk{}, n, errors.New("didnt read everything in chunk data")
	}
	chunk.ChunkData = data
	return chunk, bytesRead, nil

}
