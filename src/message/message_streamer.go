package message

import (
	"fmt"

	"github.com/tj03/rtmp/src/logger"
)

type MessageStreamer struct {
	MessageStreams map[int]int
	chunkStreamer  ChunkStreamer
}

var t uint64

func createTimestamp() uint64 {
	t += 200
	return 0
}

//Command Messages

func (mStreamer *MessageStreamer) NewChunksFromMessage(msg Message) ([]Chunk, error) {
	data := msg.MessageData
	originalSize := len(data)
	var size int
	cur := 0
	chunkSize := mStreamer.chunkStreamer.ChunkSize
	chunks := []Chunk{}
	chunkStreamId := msg.ChunkStreamId

	for cur < len(data) {
		if chunkSize > len(data) {
			size = len(data) - cur
		} else {
			size = chunkSize
		}
		chunksLength := len(chunks)
		var format int
		messageHeader := ChunkMessageHeader{}
		if chunksLength > 0 {
			format = 3
			messageHeader = chunks[0].MessageHeader
		} else {
			format = 0
			messageHeader = ChunkMessageHeader{
				Timestamp:       uint32(createTimestamp()),
				MessageLength:   uint32(originalSize),
				MessageTypeId:   uint8(msg.MessageType),
				MessageStreamId: uint32(msg.MessageStreamId),
			}
		}
		basicHeader := ChunkBasicHeader{uint8(format), uint64(chunkStreamId)}

		data = data[cur : cur+size]
		cur += size

		chunk := Chunk{basicHeader, messageHeader, 0, data}
		chunks = append(chunks, chunk)
	}
	return chunks, nil
}

func (mStreamer *MessageStreamer) Init(conn Connection, chunkSize int) {
	mStreamer.chunkStreamer.Init(conn, chunkSize)
	mStreamer.MessageStreams = make(map[int]int)
}

func (mStreamer *MessageStreamer) SetChunkSize(chunkSize int) {
	mStreamer.chunkStreamer.ChunkSize = chunkSize
}

func (mStreamer *MessageStreamer) WriteMessageToStream(msg Message) error {
	chunks, err := mStreamer.NewChunksFromMessage(msg)
	if err != nil {
		logger.ErrorLog.Fatalln("Error encountered when chunking message", err, msg)
	}
	return mStreamer.chunkStreamer.WriteChunksToStream(chunks)
}

//dont know if i should demultiplex message streams - dont understand the point of them
func (mStreamer *MessageStreamer) ReadMessageFromStream() (Message, error) {
	cStreamer := mStreamer.chunkStreamer
	chunk, err := cStreamer.ReadChunkFromStream()
	if err != nil {
		return Message{}, err
	}
	var message Message
	msgLength := int(chunk.MessageHeader.MessageLength)
	msgTypeId := MessageType(chunk.MessageHeader.MessageTypeId)
	msgStreamId := int(chunk.MessageHeader.MessageStreamId)
	message.MessageData = append(message.MessageData, chunk.ChunkData...)

	for msgLength > len(message.MessageData) {
		chunk, err = cStreamer.ReadChunkFromStream()
		fmt.Println("Read chunk")
		if err != nil {
			return message, err
		}

		message.MessageData = append(message.MessageData, chunk.ChunkData...)
	}
	previewSize := len(message.MessageData)
	if previewSize > 32 {
		previewSize = 32
	}
	if msgLength != len(message.MessageData) {
		l := len(message.MessageData)
		message.MessageData = message.MessageData[:previewSize]
		logger.ErrorLog.Println("Message data length not equal to length specified in message header", "Header Length:", msgLength, "Provided Lenght:", l, "Message:", message)
	}
	message.MessageType = msgTypeId
	message.MessageStreamId = msgStreamId
	return message, err
}
