package rtmp

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/tj03/rtmp/src/amf"
	"github.com/tj03/rtmp/src/logger"
	rtmpMsg "github.com/tj03/rtmp/src/messaging"
)

//There are still some data race issues

type Server struct {
	//change to arr?
	context      *Context
	sessionCount int
}

//Dummy connection struct for testing. Implements ByteReader/Writer and Flush
type Connection struct {
	net.Conn
}

func (c *Connection) ReadByte() (byte, error) {
	buf := make([]byte, 1)
	_, err := c.Read(buf)
	return buf[0], err
}

func (c *Connection) WriteByte(b byte) error {
	buf := []byte{b}
	_, err := c.Write(buf)
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

//Session state enum. Only used for disconnect and pause states currently. Will eventually add publishing, playing etc. states.
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
	//Channel buffer size set to arbitrary size of 4
	//No special reason for that buffer size but should be enough to process incoming messages without blocking
	session.messageChannel = make(chan MessageResult, 4)
	session.streamChannel = make(chan MessageResult, 4)
	session.conn = conn
	defer session.disconnect()
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
		//Deadline necessary to detect client side or server side errors
		//Arbitrary timeout length, no special reason for why it's 15 seconds
		deadLine := 15
		session.conn.SetDeadline(time.Now().Add(time.Second * time.Duration(deadLine)))

		//If a stream is paused we want to make sure the client is still available during that time
		//If they dont send a ping response the connection will close
		if session.state.Paused && time.Since(prevPingTime) > time.Second {
			curTime := time.Now()
			pingMsg := rtmpMsg.NewPingMsg(curTime, session.streamId)
			if err := session.messageStreamer.WriteMessageToStream(pingMsg); err != nil {
				return err
			}
			prevPingTime = curTime
		}

		//Handle any messages received from the client session (sent from ReadMessages goroutine)
		//Any detected errors will shutdown/disconnet the current session
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

		//Handle any messages sent to the stream we are playing (sent from the goroutine receiving the publisher stream)
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

//Used as a goroutine to asynchronously receive messages from client
//This goroutine handles the closing of the messageChannel
func (session *Session) ReadMessages() {
	for {
		msg, err := session.messageStreamer.ReadMessageFromStream()
		switch msg.MessageType {
		//Handle write operations to the message streamer here to make sure future messages are read properly
		//If we don't do it here and let the message handler do it the ReadMessages goroutine will read the next message before the chunk size is updated (data race)
		case rtmpMsg.SetChunkSize:
			session.handleSetChunkSize(msg.MessageData)
			continue
		case rtmpMsg.WindowAckSize:
			session.handleSetWindowAckSize(msg.MessageData)
			continue
		}
		session.messageChannel <- MessageResult{msg, err}
		if err != nil {
			close(session.messageChannel)
			return
		}
	}
}

func (session *Session) HandleMessage(msg rtmpMsg.Message) error {
	var err error
	switch msg.MessageType {

	case 0:
		//This only occurs if MessageStreamer read from the connection wrong or if the client side isn't working properly
		//We do nothing here and eventually an EOF or timeout error will occur on our Connection object and the session will disconnect

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
		logger.ErrorLog.Println("Unhandled message - type unkown. type =", msg.MessageType)
	}

	return err

}

//This function is used to update session state if the streamer we are receiving data from stops streaming
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
	//According to RTMP spec the first bit of the 4 byte chunk size cannot be 0
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

//https://rtmp.veriskope.com/docs/spec/#7211connect
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

	//RTMP spec specifies that the WindowAck, SetBandwidth, UserControl(StreamBegin), and a response object be sent to the client after
	//receiving a connect command message. Additionally it seems that OBS Studio and FFMPEG expect  a SetChunkSize message as well, not sure why.
	_, err := session.messageStreamer.WriteMessagesToStream(
		rtmpMsg.NewWinAckMessage(5000000),
		rtmpMsg.NewSetPeerBandwidthMessage(5000000, 2),
		rtmpMsg.NewStreamBeginMessage(0),
		rtmpMsg.NewSetChunkSizeMessage(session.messageStreamer.GetChunkSize()),
		rtmpMsg.NewCommandMessage0(response, 0))

	if err != nil {
		return err
	}
	session.state.Connected = true
	return nil
}

//https://rtmp.veriskope.com/docs/spec/#7213createstream
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

	cmdMsg := rtmpMsg.NewCommandMessage0(responseData, 0)
	logger.InfoLog.Println("Sending create stream response.")
	err := session.messageStreamer.WriteMessageToStream(cmdMsg)
	return err
}

//https://rtmp.veriskope.com/docs/spec/#7221play
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

	if session.state.Playing {
		logger.WarningLog.Printf("Session %d: client attempted to play when already playing", session.sessionId)
		session.messageStreamer.WriteMessageToStream(rtmpMsg.NewStatusMessage("error", "NetStream.Play.BadConnection", "Cannot play stream when already playing"))
		return nil
	}

	//TODO: fix data race -> publisher may be created between the time we check for a publisher and add to waitlist
	//Subscribe to stream/publisher
	publisher := session.context.GetPublisher(streamName)
	if publisher == nil {
		session.curStream = streamName
		session.context.AppendToWaitlist(streamName, Subscriber{session.streamChannel, session.sessionId})
		return nil
	}

	_, err := session.messageStreamer.WriteMessagesToStream(
		rtmpMsg.NewSetChunkSizeMessage(session.messageStreamer.GetChunkSize()),
		rtmpMsg.NewStreamIsRecordedMessage(uint32(session.streamId)),
		rtmpMsg.NewStreamBeginMessage(uint32(session.streamId)),
		rtmpMsg.NewStatusMessage("status", "NetStream.Play.Reset", "Playing and resetting stream"),
		rtmpMsg.NewStatusMessage("status", "NetStream.Play.Start", "Started playing stream."))

	if err != nil {
		return err
	}

	publisher.AddSubscriber(session.streamChannel, session.sessionId)
	session.state.Playing = true
	session.curStream = streamName
	metaDataMsg := rtmpMsg.NewMetaDataMessage(publisher.GetMetadata(), session.streamId)
	session.messageStreamer.WriteMessageToStream(metaDataMsg)

	//Client cant play audio/video without seqeunce headers (assuming client is using FLV)
	if aacSeqHeader := publisher.GetAACSequenceHeader(); aacSeqHeader != nil {
		aacMsg := rtmpMsg.NewAudioMessage(aacSeqHeader, session.streamId)
		err := session.messageStreamer.WriteMessageToStream(aacMsg)
		if err != nil {
			return err
		}
	}
	if avcSeqHeader := publisher.GetAVCSequenceHeader(); avcSeqHeader != nil {
		avcMsg := rtmpMsg.NewVideoMessage(avcSeqHeader, session.streamId)
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
			"Connection already publishing"))
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

	streamWasCreated := session.context.CreateStream(session.sessionId, streamName, session.ClientMetadata)
	if !streamWasCreated {
		logger.InfoLog.Printf("Session %d attempted publishing stream name that already exists. stream name = %s", session.sessionId, streamName)
		err := session.messageStreamer.WriteMessageToStream(rtmpMsg.NewStatusMessage("error",
			"NetStream.Publish.BadName",
			"Stream name already exists"))
		return err
	}

	session.messageStreamer.WriteMessageToStream(rtmpMsg.NewStatusMessage(
		"status",
		"NetStream.Publish.Start",
		"Stream published"))

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
			"Stream paused")
		if err := session.messageStreamer.WriteMessageToStream(pauseMsg); err != nil {
			return err
		}
		session.state.Paused = true
	} else {
		//if session not paused ignore resume
		if !session.state.Paused {
			return nil
		}
		publisher := session.context.GetPublisher(session.curStream)
		//if publisher not found just return
		if publisher == nil {
			return nil
		}
		aacSeqHeader := publisher.GetAACSequenceHeader()
		//if there is no sequence header for audio/video we don't need to send anything and just return
		if aacSeqHeader == nil {
			return nil
		}
		aacHeaderMsg := rtmpMsg.NewAudioMessage(aacSeqHeader, session.streamId)
		if err := session.messageStreamer.WriteMessageToStream(aacHeaderMsg); err != nil {
			return err
		}
		avcSeqHeader := publisher.GetAVCSequenceHeader()
		//if there is no sequence header for audio/video we don't need to send anything and just return
		if avcSeqHeader == nil {
			return nil
		}
		avcHeaderMsg := rtmpMsg.NewVideoMessage(avcSeqHeader, session.streamId)
		if err := session.messageStreamer.WriteMessageToStream(avcHeaderMsg); err != nil {
			return err
		}
		publisher.AddSubscriber(session.streamChannel, session.sessionId)
		unpauseMsg := rtmpMsg.NewStatusMessage("status",
			"NetStream.Unause.Notify",
			"Stream resumed")
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
		return nil
	}
	encodedDataObj := amf.EncodeAMF0("onMetaData", objs[2])
	dataMsg := rtmpMsg.NewCommandMessage0(encodedDataObj, 0)
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

	streamName, ok := session.context.GetStreamName(session.sessionId)
	if !ok {
		return nil
	}
	publisher := session.context.GetPublisher(streamName)
	if publisher == nil {
		return nil
	}
	if isSequenceHeader {
		publisher.SetAACSequenceHeader(data)
	}
	msg := rtmpMsg.NewAudioMessage(data, session.streamId)
	publisher.BroadcastMessage(msg)
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
	streamName, ok := session.context.GetStreamName(session.sessionId)
	if !ok {
		return nil
	}
	publisher := session.context.GetPublisher(streamName)
	if publisher == nil {
		return nil
	}
	if isSequenceHeader {
		publisher.SetAVCSequenceHeader(data)
	}
	msg := rtmpMsg.NewVideoMessage(data, session.streamId)
	publisher.BroadcastMessage(msg)
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
	const handshakePacketSize = 1536
	//RTMPVersion := 3
	conn := session.conn

	//read C0
	clientVersion, err := conn.ReadByte()
	if err != nil {
		return err
	}
	//send S0
	conn.WriteByte(clientVersion)
	randomBytes := make([]byte, handshakePacketSize)
	conn.Write(randomBytes)
	data := make([]byte, handshakePacketSize)
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
