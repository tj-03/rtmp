package rtmp

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/tj03/rtmp/src/amf"
	"github.com/tj03/rtmp/src/logger"
	rtmpMsg "github.com/tj03/rtmp/src/message"
)

var COMMAND_MESSAGE_CHUNK_STREAM = 3
var VIDEO_MESSAGE_CHUNK_STREAM = 6
var AUDIO_MESSAGE_CHUNK_STREAM = 7
var DATA_MESSAGE_CHUNK_STREAM = 8

type Server struct {
	//change to arr?
	context      *Context
	sessionCount int
}

//Dummy connection struct for testing. Implements ByteReader/Writer and Flush
type Connection struct {
	net.Conn
}

func (t Connection) ReadByte() (byte, error) {
	buf := make([]byte, 1)
	_, err := t.Read(buf)
	return buf[0], err
}

func (t Connection) WriteByte(b byte) error {
	buf := []byte{b}
	_, err := t.Write(buf)
	return err
}

//Initializes server fields and returns server
func NewRTMPServer() Server {
	server := Server{}
	server.context = &Context{}
	server.context.clientStreams = make(map[int]string)
	server.context.publishers = map[string]*Publisher{}
	server.context.waitLists = map[string][]Subscriber{}
	return server
}

func (server *Server) Listen(port int) error {
	tcpSocket, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		logger.ErrorLog.Println(err)
		return err
	}
	defer tcpSocket.Close()
	logger.InfoLog.Println("Listening on port", port)
	fmt.Println("Listening on port", port)
	for {
		tcpConnection, err := tcpSocket.Accept()
		if err != nil {
			return err
		}
		logger.InfoLog.Println("New connection!", tcpConnection.RemoteAddr().String())
		fmt.Println("New conneciton", tcpConnection.RemoteAddr().String())
		fmt.Printf("New connection session id = %d\n", server.sessionCount)

		c := &Connection{tcpConnection}

		server.onConnection(c)
	}
}

func (server *Server) onConnection(c *Connection) {
	session := Session{context: server.context, sessionId: server.sessionCount}
	go session.HandleConnection(c)
	server.sessionCount++
}

//Session state enum. Only used for disconnect state currently. Will eventually add publishing, playing etc. states.
type RTMPSessionState struct {
	Connected bool
	Playing   bool
	Paused    bool
	Published bool
}

//Represents a RTMP session with a client.
type Session struct {
	conn            *Connection
	sessionId       int
	state           RTMPSessionState
	context         *Context
	streamId        int
	messageStreamer rtmpMsg.MessageStreamer
	messageChannel  chan MessageResult
	streamChannel   chan MessageResult
	ClientMetadata  []byte
	curStream       string
}

type MessageResult struct {
	Message rtmpMsg.Message
	Err     error
}

func (session *Session) HandleConnection(conn *Connection) error {
	defer session.disconnect()
	session.messageChannel = make(chan MessageResult, 4)
	session.streamChannel = make(chan MessageResult, 32)
	session.streamId = 6
	session.conn = conn
	if err := session.CompleteHandshake(); err != nil {
		logger.ErrorLog.Printf("Session %d handshake failed: err: %s", session.sessionId, err.Error())
		return err
	}
	err := session.Run()
	if err != nil {
		logger.ErrorLog.Printf("Session %d exited with error:%s", session.sessionId, err.Error())
	}
	return err
}

func (session *Session) Run() error {
	session.messageStreamer.Init(session.conn, 128)
	prevPingTime := time.Now()
	go session.ReadMessages()
	for {
		//Deadline necessary to detect errors
		deadLine := 15
		session.conn.SetDeadline(time.Now().Add(time.Second * time.Duration(deadLine)))

		//If a stream is paused we want to make sure the client is still available during that time
		//If they dont send a ping response the connection will close
		if session.state.Paused {
			if time.Since(prevPingTime) > time.Second {
				curTime := time.Now()
				pingMsg := rtmpMsg.NewPingMsg(curTime, session.streamId)
				if err := session.messageStreamer.WriteMessageToStream(pingMsg); err != nil {
					return err
				}
				prevPingTime = curTime
			}
		}

		select {
		case msgResult := <-session.messageChannel:
			if msgResult.Err != nil {
				logger.ErrorLog.Printf("Session %d: Message reader err:%s", session.sessionId, msgResult.Err.Error())
				return msgResult.Err
			}
			err := session.HandleMessage(msgResult.Message)
			if err != nil {
				logger.ErrorLog.Printf("Session %d: Error after attempting to handle msg: err:%s, Msg:%s",
					session.sessionId,
					err.Error(),
					rtmpMsg.MessageToString(msgResult.Message, false))
				return err
			}

		case msgResult := <-session.streamChannel:
			streamMsg := msgResult.Message
			streamMsg.MessageStreamId = session.streamId
			err := session.handleStreamMessage(streamMsg)
			session.messageStreamer.WriteMessageToStream(streamMsg)
			if err != nil {
				logger.ErrorLog.Printf("Session %d: Error writing message to stream after receieve from stream channel: err:%s",
					session.sessionId,
					err.Error())
				return err
			}
		}

	}

}

func (session *Session) ReadMessages() {
	for {
		msg, err := session.messageStreamer.ReadMessageFromStream()
		switch msg.MessageType {
		//Handle chunk size here to avoid reading new messages without changing the chunk size
		case rtmpMsg.SetChunkSize:
			session.handleSetChunkSize(msg.MessageData)
			continue
		case rtmpMsg.WindowAckSize:
			session.handleSetWindowAckSize(msg.MessageData)
			continue
		}
		if err != nil {
			session.messageChannel <- MessageResult{rtmpMsg.Message{}, err}
			close(session.messageChannel)
			return
		}
		//Any write operations to the message streamer must be handled here to avoid data race
		session.messageChannel <- MessageResult{msg, err}
	}
}

func (session *Session) HandleMessage(msg rtmpMsg.Message) error {
	var err error
	switch msg.MessageType {

	case 0:
		//logger.ErrorLog.Println("Message type 0")

	case rtmpMsg.SetChunkSize:
		err = session.handleSetChunkSize(msg.MessageData)

	case rtmpMsg.WindowAckSize:
		err = session.handleSetWindowAckSize(msg.MessageData)

	case rtmpMsg.UserControl:
		eventType := 0
		if len(msg.MessageData) >= 2 {
			eventType = int(binary.BigEndian.Uint16(msg.MessageData[:2]))
		}
		logger.WarningLog.Println("User control Message Sent - Unhandled: Event type = ", rtmpMsg.EventType(eventType))

	case rtmpMsg.CommandMsg0, rtmpMsg.CommandMsg3:
		err = session.handleCommandMessage(msg.MessageData)

	case rtmpMsg.DataMsg0:
		err = session.handleDataMessage(msg.MessageData)

	case rtmpMsg.AudioMsg:
		err = session.handleAudioMessage(msg.MessageData)

	case rtmpMsg.VideoMsg:
		err = session.handleVideoMessage(msg.MessageData)

	case rtmpMsg.Ack:
		//do nothing

	default:
		logger.ErrorLog.Println("Unhandled message  - type unkown. type =", msg.MessageType)
	}

	return err

}

//If a publisher sends a unpublish message we should update the session state
func (session *Session) handleStreamMessage(msg rtmpMsg.Message) error {
	if msg.MessageType == rtmpMsg.CommandMsg0 {
		objs, _, err := amf.DecodeAMF0Sequence(msg.MessageData)
		//dont really care if decode doesn't work
		if err != nil {
			return nil
		}
		if len(objs) < 4 {
			return nil
		}
		status, ok := objs[3].(map[string]interface{})
		if !ok {
			return nil
		}
		if status["code"] == "NetStream.Play.UnpublishNotify" {
			session.state.Paused = false
			session.state.Playing = false
		}

	}
	return nil
}

func (session *Session) handleSetChunkSize(data []byte) error {
	if len(data) < 4 || data[0]&1 == 1 {
		logger.WarningLog.Println("Set chunk size message err - first bit not 0", data)
		return errors.New("invalid chunk size sent")
	}
	chunkSize := binary.BigEndian.Uint32(data[:4])
	logger.InfoLog.Println("Setting chunk size:", chunkSize)
	session.messageStreamer.SetChunkSize(chunkSize)
	return nil
}

func (session *Session) handleSetWindowAckSize(data []byte) error {
	if len(data) != 4 {
		logger.WarningLog.Println("Set window ack sizw payload err - payload length not 4")
		return errors.New("invalid window size sent")
	}
	windowSize := binary.BigEndian.Uint32(data[:4])
	logger.InfoLog.Println("Setting window ack size:", windowSize)
	session.messageStreamer.MaxWindowAckSize = int(windowSize)
	return nil
}

func (session *Session) handleCommandMessage(data []byte) error {
	amfObjects, _, _ := amf.DecodeAMF0Sequence(data)
	if len(amfObjects) == 0 {
		logger.WarningLog.Println("No objects decoded")
		return nil
	}
	commandName, ok := amfObjects[0].(string)
	if !ok {
		logger.WarningLog.Println("Command message payload does not have a string(cmd message) as first amf object")
		return nil
	}

	switch commandName {
	case "connect":
		return session.handleConnectCommand(amfObjects)
	case "createStream":
		return session.handleCreateStreamCommand(amfObjects)
	case "publish":
		return session.handlePublishCommand(amfObjects)
	case "play":
		return session.handlePlayCommand(amfObjects)
	case "pause":
		return session.handlePauseCommand(amfObjects)
	default:
		logger.WarningLog.Println("Unknown command message received. Unhandled. Objects:", amfObjects)
	}
	return nil
}

func (session *Session) handleConnectCommand(objects []interface{}) error {
	result := "_result"
	logger.InfoLog.Println("Connect object from client", objects[2])
	info := map[string]interface{}{
		"fmsVer":       "FMS/3,0,1,123",
		"capabilities": 31,
	}

	props := map[string]interface{}{
		"level":          "status",
		"code":           "NetConnection.Connect.Success",
		"description":    "Connection succeeds",
		"objectEncoding": 0,
	}

	response := amf.EncodeAMF0(result, 1, props, info)

	winAckMsg := rtmpMsg.NewWinAckMessage(5000000)
	setBandwidthMsg := rtmpMsg.NewSetPeerBandwidthMessage(5000000, 2)
	streamBeginMsg := rtmpMsg.NewStreamBeginMessage(0)
	chunkSize := session.messageStreamer.GetChunkSize()
	setChunkMsg := rtmpMsg.NewSetChunkSizeMessage(chunkSize)

	logger.InfoLog.Println("Writing window ack bytes to stream", winAckMsg)
	if err := session.messageStreamer.WriteMessageToStream(winAckMsg); err != nil {
		return err
	}

	logger.InfoLog.Println("Writing set bandwidth message", setBandwidthMsg)
	if err := session.messageStreamer.WriteMessageToStream(setBandwidthMsg); err != nil {
		return err
	}

	logger.InfoLog.Println("Writing set chunk size message", setChunkMsg)
	if err := session.messageStreamer.WriteMessageToStream(setChunkMsg); err != nil {
		return err
	}

	logger.InfoLog.Println("Writing stream begin message")
	if err := session.messageStreamer.WriteMessageToStream(streamBeginMsg); err != nil {
		return err
	}
	resMsg := rtmpMsg.NewCommandMessage0(response, 0, COMMAND_MESSAGE_CHUNK_STREAM)
	logger.InfoLog.Println("Writing connect response", resMsg)
	if err := session.messageStreamer.WriteMessageToStream(resMsg); err != nil {
		return err
	}
	session.state.Connected = true
	return nil
}

func (session *Session) handleCreateStreamCommand(objects []interface{}) error {
	var responseData []byte
	validObjects := len(objects) >= 2
	var transactionId float64
	var ok bool
	if validObjects {
		transactionId, ok = objects[1].(float64)
		validObjects = validObjects && ok
	}
	if !validObjects {
		responseData = amf.EncodeAMF0("_error", -1, nil, "No transaction Id given")
	} else {
		responseData = amf.EncodeAMF0("_result", transactionId, nil, session.streamId)
		//session.streamCount++
	}

	cmdMsg := rtmpMsg.NewCommandMessage0(responseData, 0, COMMAND_MESSAGE_CHUNK_STREAM)
	logger.InfoLog.Println("Sending create stream response.")
	err := session.messageStreamer.WriteMessageToStream(cmdMsg)
	return err
}

func (session *Session) handlePlayCommand(objects []interface{}) error {
	if len(objects) < 4 {
		logger.WarningLog.Printf("Session %d: client sent invalid objects", session.sessionId)
		logger.WarningLog.Println("Bad objects sent", objects)
		return nil
	}
	streamName, ok := objects[3].(string)
	if !ok {
		logger.WarningLog.Printf("Session %d: client sent invalid stream name", session.sessionId)
		logger.WarningLog.Println("Bad stream name sent", objects[3])
		return nil
	}

	//Subscribe to stream/publisher
	publisher := session.context.GetPublisher(streamName)
	if publisher == nil {
		session.curStream = streamName
		session.context.AppendToWaitlist(streamName, Subscriber{session.streamChannel, session.sessionId})
		return nil
	}
	session.messageStreamer.WriteMessageToStream(rtmpMsg.NewSetChunkSizeMessage(session.messageStreamer.GetChunkSize()))

	streamRecordedMsg := rtmpMsg.NewStreamIsRecordedMessage(uint32(session.streamId))
	streamBeginMsg := rtmpMsg.NewStreamBeginMessage(uint32(session.streamId))
	playResetMsg := rtmpMsg.NewStatusMessage("status", "NetStream.Play.Reset", "Playing and resetting stream", session.streamId)
	playStartMsg := rtmpMsg.NewStatusMessage("status", "NetStream.Play.Start", "Started playing stream.", session.streamId)
	_, err := session.messageStreamer.WriteMessagesToStream(
		streamRecordedMsg,
		streamBeginMsg,
		playResetMsg,
		playStartMsg)

	if err != nil {
		return err
	}

	publisher.AddSubscriber(session.streamChannel, session.sessionId)
	session.state.Playing = true
	session.curStream = streamName
	metaDataMsg := rtmpMsg.NewMessage(publisher.GetMetadata(), rtmpMsg.DataMsg0, session.streamId, COMMAND_MESSAGE_CHUNK_STREAM)
	session.messageStreamer.WriteMessageToStream(metaDataMsg)

	//Client cant play audio/video without seqeunce headers (assuming FLV)
	if aacSeqHeader := publisher.GetAACSequenceHeader(); aacSeqHeader != nil {
		aacMsg := rtmpMsg.NewMessage(aacSeqHeader, rtmpMsg.AudioMsg, session.streamId, AUDIO_MESSAGE_CHUNK_STREAM)
		err := session.messageStreamer.WriteMessageToStream(aacMsg)
		if err != nil {
			return err
		}
	}
	if avcSeqHeader := publisher.GetAVCSequenceHeader(); avcSeqHeader != nil {
		avcMsg := rtmpMsg.NewMessage(avcSeqHeader, rtmpMsg.VideoMsg, session.streamId, VIDEO_MESSAGE_CHUNK_STREAM)
		err := session.messageStreamer.WriteMessageToStream(avcMsg)
		if err != nil {
			return err
		}
	}

	return nil
}

//Create a new publisher in the context object if not already publishing and if stream name is unique
func (session *Session) handlePublishCommand(objects []interface{}) error {
	if len(objects) < 5 {
		logger.WarningLog.Printf("Session %d: client sent invalid objects", session.sessionId)
		logger.WarningLog.Println("Bad objects sent", objects)
		return nil
	}
	_, ok := session.context.GetStreamName(session.sessionId)
	if ok {
		err := session.messageStreamer.WriteMessageToStream(rtmpMsg.NewStatusMessage("error",
			"NetStream.Publish.BadConnection",
			"Connection already publishing",
			0))
		return err
	}
	if !session.state.Connected {
		logger.WarningLog.Printf("Session %d: Client attempted publishing without connecting", session.sessionId)
		return nil
	}
	//Stream name should be the 4th object sent in the command
	streamName, ok := objects[3].(string)
	if !ok {
		logger.WarningLog.Printf("Session %d: client sent invalid stream name", session.sessionId)
		logger.WarningLog.Println("Bad stream name sent", objects[3])
		return nil
	}
	if publisher := session.context.GetPublisher(streamName); publisher != nil {
		logger.InfoLog.Printf("Session %d attempted publishing stream name that already exists. stream name = %s", session.sessionId, streamName)
		err := session.messageStreamer.WriteMessageToStream(rtmpMsg.NewStatusMessage("error",
			"NetStream.Publish.BadName",
			"Stream name already exists",
			0))
		return err
	}
	//Getting and setting the publisher should all happen in one transaction. Currently 2 threads could publish at the same time and overwrite one another, since there is no check
	//in the set publisher method to see if a publisher already exists.
	session.context.SetStreamName(session.sessionId, streamName)
	session.context.SetPublisher(streamName, &Publisher{
		SessionId:   session.sessionId,
		Subscribers: []Subscriber{},
		metadata:    session.ClientMetadata})

	session.messageStreamer.WriteMessageToStream(rtmpMsg.NewStatusMessage(
		"status",
		"NetStream.Publish.Start",
		"Stream published",
		0))

	return nil
}

func (session *Session) handlePauseCommand(objects []interface{}) error {
	if len(objects) < 4 {
		logger.WarningLog.Printf("Session %d: client sent invalid objects", session.sessionId)
		logger.WarningLog.Println("Bad objects sent", objects)
		return nil
	}
	pause, ok := objects[3].(bool)
	if !ok {
		logger.WarningLog.Printf("Session %d: client sent invalid stream name", session.sessionId)
		logger.WarningLog.Println("Bad stream name sent", objects[3])
		return nil
	}
	//True == pause, false == resume
	if pause {
		//if session paused ignore pause
		if session.state.Paused {
			return nil
		}
		if publisher := session.context.GetPublisher(session.curStream); publisher != nil {
			publisher.RemoveSubscriber(session.sessionId)
		}
		pauseMsg := rtmpMsg.NewStatusMessage("status",
			"NetStream.Pause.Notify",
			"Stream paused",
			COMMAND_MESSAGE_CHUNK_STREAM)
		if err := session.messageStreamer.WriteMessageToStream(pauseMsg); err != nil {
			return err
		}
		session.state.Paused = true
	} else {
		//if session not paused ignore resume
		if !session.state.Paused {
			return nil
		}
		if publisher := session.context.GetPublisher(session.curStream); publisher != nil {

			if aacSeqHeader := publisher.GetAACSequenceHeader(); aacSeqHeader != nil {
				aacHeaderMsg := rtmpMsg.NewMessage(aacSeqHeader, rtmpMsg.AudioMsg, session.streamId, AUDIO_MESSAGE_CHUNK_STREAM)
				if err := session.messageStreamer.WriteMessageToStream(aacHeaderMsg); err != nil {
					return err
				}
			}
			if avcSeqHeader := publisher.GetAVCSequenceHeader(); avcSeqHeader != nil {
				avcHeaderMsg := rtmpMsg.NewMessage(avcSeqHeader, rtmpMsg.VideoMsg, session.streamId, VIDEO_MESSAGE_CHUNK_STREAM)
				if err := session.messageStreamer.WriteMessageToStream(avcHeaderMsg); err != nil {
					return err
				}
			}
			publisher.AddSubscriber(session.streamChannel, session.sessionId)
		}
		unpauseMsg := rtmpMsg.NewStatusMessage("status",
			"NetStream.Unause.Notify",
			"Stream resumed",
			COMMAND_MESSAGE_CHUNK_STREAM)
		if err := session.messageStreamer.WriteMessageToStream(unpauseMsg); err != nil {
			return err
		}
		session.state.Paused = false
	}
	return nil
}

func (session *Session) handleDataMessage(data []byte) error {
	objs, _, _ := amf.DecodeAMF0Sequence(data)
	if len(objs) < 3 {
		//TODO:handle bad data message
		return nil
	}
	encodedDataObj := amf.EncodeAMF0("onMetaData", objs[2])
	dataMsg := rtmpMsg.NewCommandMessage0(encodedDataObj, 0, COMMAND_MESSAGE_CHUNK_STREAM)
	session.ClientMetadata = data
	if streamName, ok := session.context.GetStreamName(session.sessionId); ok {
		if publisher := session.context.GetPublisher(streamName); publisher != nil {
			publisher.SetMetadata(data)
			publisher.BroadcastMessage(dataMsg)
		}
	}

	err := session.messageStreamer.WriteMessageToStream(dataMsg)
	return err
}

func (session *Session) handleAudioMessage(data []byte) error {
	soundFormat := byte(0)
	isSequenceHeader := false
	if len(data) >= 2 {
		soundFormat = (data[0] >> 4) & 0b00001111
		isSequenceHeader = soundFormat == 10 && data[1] == 0
	}

	if streamName, ok := session.context.GetStreamName(session.sessionId); ok {
		if publisher := session.context.GetPublisher(streamName); publisher != nil {
			if isSequenceHeader {
				publisher.SetAACSequenceHeader(data)
			}
			msg := rtmpMsg.NewMessage(data, rtmpMsg.AudioMsg, session.streamId, AUDIO_MESSAGE_CHUNK_STREAM)
			publisher.BroadcastMessage(msg)
		}
	}
	return nil
}

func (session *Session) handleVideoMessage(data []byte) error {
	codecId := byte(0)
	frameType := byte(0)
	isSequenceHeader := false
	if len(data) >= 2 {
		frameType = (data[0] >> 4) & 0b00001111
		codecId = data[0] & 0b00001111
		isSequenceHeader = codecId == 7 && frameType == 1 && data[1] == 0
	}
	if streamName, ok := session.context.GetStreamName(session.sessionId); ok {
		if publisher := session.context.GetPublisher(streamName); publisher != nil {
			if isSequenceHeader {
				publisher.SetAVCSequenceHeader(data)
			}
			msg := rtmpMsg.NewMessage(data, rtmpMsg.VideoMsg, session.streamId, VIDEO_MESSAGE_CHUNK_STREAM)
			publisher.BroadcastMessage(msg)
		}
	}
	return nil
}

func (session *Session) disconnect() {
	session.state.Connected = false
	if publisher := session.context.GetPublisher(session.curStream); publisher != nil {
		publisher.RemoveSubscriber(session.sessionId)
	}
	session.context.RemoveFromWaitlist(session.sessionId, session.curStream)
	session.context.RemovePublisher(session.sessionId)
	close(session.streamChannel)
	session.conn.Close()
	logger.InfoLog.Printf("Session %d disconnected", session.sessionId)
	fmt.Printf("Session %d disconnected\n", session.sessionId)
}

func (session *Session) CompleteHandshake() error {
	//RTMPVersion := 3
	conn := session.conn

	//read C0
	clientVersion, err := conn.ReadByte()
	if err != nil {
		return err
	}
	//send S0
	conn.WriteByte(clientVersion)
	rando := make([]byte, S1SIZE)
	conn.Write(rando)
	data := make([]byte, C1SIZE)
	n, err := conn.Read(data)
	if err != nil {
		return err
	}
	if n < len(data) {
		return errors.New("failed to read handshake from client")
	}
	conn.Write(data)
	n, err = conn.Read(data)
	if err != nil {
		return err
	}
	if n < len(data) {
		return errors.New("failed to read handshake from client")
	}
	logger.InfoLog.Println("Handshake complete")
	return nil

}
