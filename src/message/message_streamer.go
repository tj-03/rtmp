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
	cache            map[int]ChunkMessageHeader
}

var t uint32

func createTimestamp() uint32 {
	t += 200
	return uint32(time.Now().UnixMilli())
}

//Command Messages

func (mStreamer *MessageStreamer) Init(conn Connection, chunkSize int) {
	mStreamer.chunkStreamer.Init(conn, chunkSize)
	mStreamer.MessageStreams = make(map[int]int)

}

func verifyChunks(chunks []Chunk, msg Message) bool {
	acc := []byte{}
	for _, chunk := range chunks {
		acc = append(acc, chunk.ChunkData...)
	}
	return util.CmpSlice(acc, msg.MessageData)
}
func (mStreamer *MessageStreamer) NewChunksFromMessage(msg Message) ([]Chunk, error) {
	data := []byte{}
	originalSize := len(msg.MessageData)
	var size int
	cur := 0
	chunkSize := mStreamer.chunkStreamer.ChunkSize
	chunks := []Chunk{}
	chunkStreamId := msg.ChunkStreamId

	for cur < originalSize {
		remainingSize := originalSize - cur
		if chunkSize > remainingSize {
			size = remainingSize
		} else {
			size = chunkSize
		}
		//chunksLength := len(chunks)
		var format int
		messageHeader := ChunkMessageHeader{}
		if false {
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
		if false && msg.MessageType == VideoMsg {
			mStreamer.cache[chunkStreamId] = messageHeader
		}
		data = msg.MessageData[cur : cur+size]
		cur += size

		chunk := Chunk{basicHeader, messageHeader, 0, data}
		chunks = append(chunks, chunk)
	}
	// if !verifyChunks(chunks, msg) {
	// 	cData := []byte{}
	// 	for _, c := range chunks {
	// 		cData = append(cData, c.ChunkData...)
	// 	}
	// 	logger.ErrorLog.Println("Bad chunk->message conv: Message Len: ", len(msg.MessageData), "Msg dat: ", MessageToString(msg, true))
	// 	logger.ErrorLog.Println("Bad chunk->message conv: Chunk len: ", len(cData), "Chunk dat:", cData)
	// 	panic("Bad message to chunk conversion: " + MessageToString(msg, false))
	// }
	return chunks, nil
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

func (mStreamer *MessageStreamer) ReadMessageFromStream() (Message, error) {
	cStreamer := mStreamer.chunkStreamer
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
