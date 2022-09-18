package message

import (
	"time"

	"github.com/tj03/rtmp/src/logger"
	"github.com/tj03/rtmp/src/util"
)

type MessageStreamer struct {
	MessageStreams   map[int]int
	chunkStreamer    ChunkStreamer
	MaxWindowAckSize int
	bytesRead        int
}

var t uint32

func createTimestamp() uint32 {
	t += 200
	return uint32(time.Now().UnixMilli())
}

//Command Messages

func (mStreamer *MessageStreamer) Init(conn Connection, chunkSize uint32) {
	mStreamer.chunkStreamer.Init(conn, chunkSize)
	mStreamer.MessageStreams = make(map[int]int)

}

func NewChunksFromMessage(msg Message, chunkSize uint32) []Chunk {
	var data []byte
	originalSize := len(msg.MessageData)
	var size int
	cur := 0
	chunks := []Chunk{}
	chunkStreamId := msg.ChunkStreamId

	for cur < originalSize {
		remainingSize := originalSize - cur
		size = util.Min(int(chunkSize), remainingSize)
		var format int
		messageHeader := ChunkMessageHeader{}
		if len(chunks) > 0 {
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

		data = msg.MessageData[cur : cur+size]
		cur += size

		chunk := Chunk{basicHeader, messageHeader, 0, data}
		chunks = append(chunks, chunk)
	}
	return chunks
}

func (mStreamer *MessageStreamer) SetChunkSize(chunkSize uint32) {
	mStreamer.chunkStreamer.ChunkSize = chunkSize
}

func (mStreamer *MessageStreamer) GetChunkSize() uint32 {
	return mStreamer.chunkStreamer.ChunkSize
}

func (mStreamer *MessageStreamer) WriteMessageToStream(msg Message) error {
	chunks := NewChunksFromMessage(msg, mStreamer.chunkStreamer.ChunkSize)
	return mStreamer.chunkStreamer.WriteChunksToStream(chunks)

}

func (mStreamer *MessageStreamer) WriteMessagesToStream(messages ...Message) (int, error) {

	for i, msg := range messages {
		chunks := NewChunksFromMessage(msg, mStreamer.chunkStreamer.ChunkSize)
		if err := mStreamer.chunkStreamer.WriteChunksToStream(chunks); err != nil {
			return i + 1, err
		}
	}
	return len(messages), nil
}

func (mStreamer *MessageStreamer) ReadMessageFromStream() (Message, error) {
	cStreamer := &mStreamer.chunkStreamer
	chunk, n, err := cStreamer.ReadChunkFromStream()
	mStreamer.bytesRead += n
	if err != nil {
		return Message{}, err
	}
	var message Message
	msgLength := int(chunk.MessageHeader.MessageLength)
	msgTypeId := MessageType(chunk.MessageHeader.MessageTypeId)
	msgStreamId := int(chunk.MessageHeader.MessageStreamId)
	message.MessageData = append(message.MessageData, chunk.ChunkData...)

	for msgLength > len(message.MessageData) {
		chunk, n, err := cStreamer.ReadChunkFromStream()
		mStreamer.bytesRead += n
		//fmt.Println("Read chunk")
		if err != nil {
			return message, err
		}

		message.MessageData = append(message.MessageData, chunk.ChunkData...)
	}
	if mStreamer.MaxWindowAckSize > 0 && mStreamer.bytesRead >= mStreamer.MaxWindowAckSize {
		mStreamer.WriteMessageToStream(NewAckMessage(uint32(mStreamer.bytesRead)))
		mStreamer.bytesRead = 0
	}

	if msgLength != len(message.MessageData) {
		logger.ErrorLog.Println("Message data length not equal to length specified in message header. Msg:", MessageToString(message, false))
	}
	message.MessageType = msgTypeId
	message.MessageStreamId = msgStreamId
	return message, err
}
