package message

import (
	"bufio"
	"encoding/binary"
	"fmt"

	"github.com/tj03/rtmp/src/logger"
)

type MessageType int
type EventType int

const (
	SetChunkSize MessageType = iota + 1
	AbortMessage
	Ack
	UserControl                  = 4
	WindowAckSize                = 5
	SetPeerBandwidth             = 6
	CommandMsg0      MessageType = 20
	CommandMsg3      MessageType = 17
	DataMsg0         MessageType = 18
	DataMsg3         MessageType = 15
	AudioMsg         MessageType = 8
	VideoMsg         MessageType = 9
	AggregateMsg     MessageType = 22
)

const (
	StreamBegin EventType = iota
	StreamEOF
	StreamDry
	SetBufferLength
	StreamIsRecorded
	_
	PingRequest
	PingResponse
)

type Message struct {
	MessageType     MessageType
	MessageData     []byte
	MessageStreamId int
	ChunkStreamId   int
}

type MessageStreamer struct {
	MessageStreams map[int]int
	chunkStreamer  ChunkStreamer
}

var t uint64

func createTimestamp() uint64 {
	t += 200
	return 0
}

func uint32ToBuf(num uint32) []byte {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, num)
	return buf
}

func uint16ToBuf(num uint16) []byte {
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf, num)
	return buf
}

func NewCommandMessage0(data []byte, msgStreamId int, chunkStreamId int) Message {
	return NewMessage(data, CommandMsg0, msgStreamId, chunkStreamId)
}

//Protocol Control Messages
func NewWinAckMessage(winSize uint32) Message {
	return NewMessage(uint32ToBuf(winSize), WindowAckSize, 0, 2)
}

func NewAckMessage(sequence uint32) Message {
	return NewMessage(uint32ToBuf(sequence), Ack, 0, 2)
}

func NewSetPeerBandwidthMessage(windowSize uint32, limitType byte) Message {
	if limitType > 2 {
		panic("Limit type greater than 2 for set bandwidth msg.")
	}
	buf := append(uint32ToBuf(windowSize), limitType)
	return NewMessage(buf, SetPeerBandwidth, 0, 2)
}

func NewSetChunkSizeMessage(chunkSize uint32) Message {
	if chunkSize&1 == 1 {
		panic("First bit of chunkSize MUST NOT be 0")
	}
	return NewMessage(uint32ToBuf(chunkSize), SetChunkSize, 0, 2)
}

//User Control Messages
func NewStreamBeginMessage(streamId uint32) Message {
	buf := uint16ToBuf(uint16(StreamBegin))
	buf = append(buf, uint32ToBuf(streamId)...)
	return NewMessage(buf, UserControl, int(streamId), 2)
}
func NewMessage(data []byte, msgTypeId MessageType, msgStreamId int, chunkStreamId int) Message {
	return Message{msgTypeId, data, msgStreamId, chunkStreamId}
}
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

func (mStreamer *MessageStreamer) Init(conn *bufio.ReadWriter, chunkSize int) {
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
func (mStreamer *MessageStreamer) ReadMessageFromStream() (error, Message) {
	cReader := mStreamer.chunkStreamer
	err, chunk := cReader.ReadChunkFromStream()
	if err != nil {
		return err, Message{}
	}
	var message Message
	msgLength := int(chunk.MessageHeader.MessageLength)
	msgTypeId := MessageType(chunk.MessageHeader.MessageTypeId)
	msgStreamId := int(chunk.MessageHeader.MessageStreamId)
	message.MessageData = append(message.MessageData, chunk.ChunkData...)

	for msgLength > len(message.MessageData) {
		err, chunk = cReader.ReadChunkFromStream()
		fmt.Println("Read chunk")
		if err != nil {
			return err, message
		}

		message.MessageData = append(message.MessageData, chunk.ChunkData...)
	}
	if msgLength != len(message.MessageData) {
		logger.ErrorLog.Println("Message data length not equal to length specified in message header", message)
	}
	message.MessageType = msgTypeId
	message.MessageStreamId = msgStreamId
	return nil, message
}
