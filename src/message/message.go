package message

import (
	"bufio"
	"fmt"
)

type Message struct {
	MessageType int
	MessageData []byte
}

type MessageReader struct {
	ChunkSize int
}

//dont know if i should demultiplex message streams - dont understand the point of them
func (mReader *MessageReader) ReadMessageFromStream(reader *bufio.Reader, cReader *ChunkReader) (error, Message) {
	err, chunk := cReader.ReadChunkFromStream(reader, mReader.ChunkSize)
	if err != nil {
		return err, Message{}
	}
	var message Message
	message.MessageData = append(message.MessageData, chunk.ChunkData...)
	fmt.Println("First chunk basic head", chunk.BasicHeader)
	fmt.Println("First chunk message head type", chunk.MessageHeader)
	fmt.Println("First chunk data", chunk.ChunkData)
	fmt.Println("")
	for int(chunk.MessageHeader.MessageLength) > len(message.MessageData) {
		err, chunk = cReader.ReadChunkFromStream(reader, mReader.ChunkSize)
		fmt.Println("Read chunk")
		if err != nil {
			return err, message
		}

		message.MessageData = append(message.MessageData, chunk.ChunkData...)
	}
	return nil, Message{}
}
