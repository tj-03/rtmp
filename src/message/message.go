package message

import (
	"bufio"
	"fmt"
)

type MessageType int

const (
	SetChunkSize MessageType = iota + 1
	AbortMessage
	Ack
	WindowAckSize = Ack + 2
	SetPeerBandwidth
	CommandMsg0  MessageType = 20
	CommandMsg3  MessageType = 17
	DataMsg0     MessageType = 18
	DataMsg3     MessageType = 15
	AudioMsg     MessageType = 8
	VideoMsg     MessageType = 9
	AggregateMsg MessageType = 22
)

type Message struct {
	MessageType MessageType
	MessageData []byte
}

type MessageReader struct {
	chunkReader ChunkReader
}

func (mReader *MessageReader) Init(conn *bufio.ReadWriter, chunkSize int) {
	mReader.chunkReader.Init(conn, chunkSize)
}

func (mReader *MessageReader) SetChunkSize(chunkSize int) {
	mReader.chunkReader.ChunkSize = chunkSize
}

//dont know if i should demultiplex message streams - dont understand the point of them
func (mReader *MessageReader) ReadMessageFromStream() (error, Message) {
	cReader := mReader.chunkReader
	err, chunk := cReader.ReadChunkFromStream()
	if err != nil {
		return err, Message{}
	}
	var message Message
	message.MessageData = append(message.MessageData, chunk.ChunkData...)
	// fmt.Println("First chunk basic head", chunk.BasicHeader)
	// fmt.Println("First chunk message head type", chunk.MessageHeader)
	// fmt.Println("First chunk data", chunk.ChunkData)
	// fmt.Println("")
	for int(chunk.MessageHeader.MessageLength) > len(message.MessageData) {
		err, chunk = cReader.ReadChunkFromStream()
		fmt.Println("Read chunk")
		if err != nil {
			return err, message
		}

		message.MessageData = append(message.MessageData, chunk.ChunkData...)
	}
	message.MessageType = MessageType(chunk.MessageHeader.MessageTypeId)
	return nil, message
}
