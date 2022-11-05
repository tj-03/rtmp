package message

import (
	"fmt"
	"time"

	"github.com/tj03/rtmp/src/amf"
	"github.com/tj03/rtmp/src/util"
)

//https://rtmp.veriskope.com/docs/spec/#71rtmp-message-types

type MessageType int
type EventType int

func MessageToString(msg Message, fullMsg bool) string {
	previewSize := 32
	if fullMsg {
		previewSize = len(msg.MessageData)
	}
	return fmt.Sprintln("New message", "Type: ", msg.MessageType, "StreamId: ", msg.MessageStreamId, "Length: ", len(msg.MessageData), msg.MessageData[:previewSize])
}

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

var COMMAND_MESSAGE_CHUNK_STREAM = 3
var VIDEO_MESSAGE_CHUNK_STREAM = 6
var AUDIO_MESSAGE_CHUNK_STREAM = 7
var DATA_MESSAGE_CHUNK_STREAM = 8

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

//Command Messages
func NewCommandMessage0(data []byte, msgStreamId int) Message {
	return NewMessage(data, CommandMsg0, msgStreamId, COMMAND_MESSAGE_CHUNK_STREAM)
}

func NewMetaDataMessage(data []byte, msgStreamId int) Message {
	return NewMessage(data, DataMsg0, msgStreamId, COMMAND_MESSAGE_CHUNK_STREAM)
}

func NewStatusMessage(level string, code string, description string) Message {
	if level != "status" && level != "warning" && level != "error" {
		panic("invalid level string for status message")
	}
	msgData := amf.EncodeAMF0("onStatus", 0, nil, map[string]interface{}{
		"level":       level,
		"code":        code,
		"description": description})
	return NewCommandMessage0(msgData, COMMAND_MESSAGE_CHUNK_STREAM)
}

//Protocol Control Messages
func NewWinAckMessage(winSize uint32) Message {
	return NewMessage(util.Uint32ToBuf(winSize), WindowAckSize, 0, 2)
}

func NewAckMessage(sequence uint32) Message {
	return NewMessage(util.Uint32ToBuf(sequence), Ack, 0, 2)
}

func NewSetPeerBandwidthMessage(windowSize uint32, limitType byte) Message {
	if limitType > 2 {
		panic("Limit type greater than 2 for set bandwidth msg.")
	}
	buf := append(util.Uint32ToBuf(windowSize), limitType)
	return NewMessage(buf, SetPeerBandwidth, 0, 2)
}

func NewSetChunkSizeMessage(chunkSize uint32) Message {
	if chunkSize&1 == 1 {
		panic("First bit of chunkSize MUST NOT be 0")
	}
	return NewMessage(util.Uint32ToBuf(chunkSize), SetChunkSize, 0, 2)
}

//User Control Messages
func NewStreamBeginMessage(streamId uint32) Message {
	buf := util.Uint16ToBuf(uint16(StreamBegin))
	buf = append(buf, util.Uint32ToBuf(streamId)...)
	return NewMessage(buf, UserControl, int(streamId), 2)
}

func NewStreamIsRecordedMessage(streamId uint32) Message {
	buf := util.Uint16ToBuf(uint16(StreamIsRecorded))
	buf = append(buf, util.Uint32ToBuf(streamId)...)
	return NewMessage(buf, UserControl, int(streamId), 2)
}

func NewStreamEOFMsg(streamId uint32) Message {
	buf := util.Uint16ToBuf(uint16(StreamIsRecorded))
	buf = append(buf, util.Uint32ToBuf(streamId)...)
	return NewMessage(buf, UserControl, int(streamId), 2)
}

func NewPingMsg(pingTime time.Time, streamId int) Message {
	buf := util.Uint16ToBuf(uint16(PingRequest))
	buf = append(buf, util.Uint32ToBuf(uint32(pingTime.UnixMilli()))...)
	return NewMessage(buf, UserControl, int(streamId), 2)
}

//Video/Audio
func NewVideoMessage(data []byte, msgStreamId int) Message {
	return Message{VideoMsg, data, msgStreamId, VIDEO_MESSAGE_CHUNK_STREAM}
}

func NewAudioMessage(data []byte, msgStreamId int) Message {
	return Message{AudioMsg, data, msgStreamId, AUDIO_MESSAGE_CHUNK_STREAM}
}

func NewMessage(data []byte, msgTypeId MessageType, msgStreamId int, chunkStreamId int) Message {
	return Message{msgTypeId, data, msgStreamId, chunkStreamId}
}
