package rtmp

import (
	"bufio"
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
	sessions     map[int]*Session
	context      *Context
	sessionCount int
}

type Connection struct {
	bufio.ReadWriter
	net.Conn
}

//Dummy connection struct for testing. Implements ByteReader/Writer and Flush
type Test struct {
	net.Conn
}

func (t Test) ReadByte() (byte, error) {
	buf := make([]byte, 1)
	_, err := t.Read(buf)
	return buf[0], err
}

func (t Test) WriteByte(b byte) error {
	buf := []byte{b}
	_, err := t.Write(buf)
	return err
}

func (t Test) Flush() error {
	return nil
}

//Initializes server fields and returns server
func NewRTMPServer() Server {
	server := Server{}
	server.sessions = make(map[int]*Session)
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
	for {
		tcpConnection, err := tcpSocket.Accept()
		if err != nil {
			return err
		}
		logger.InfoLog.Println("New connection!", tcpConnection.RemoteAddr().String())
		fmt.Println("New conneciton", tcpConnection.RemoteAddr().String())

		//If the buffer size is too small bufio.Reader will fail to read messages with a payload close to the buffer size
		//Not sure why
		bufferSize := 4096 * 64
		bufferedConnection := bufio.NewReadWriter(bufio.NewReaderSize(tcpConnection, bufferSize), bufio.NewWriter(tcpConnection))

		c := &Connection{*bufferedConnection, tcpConnection}

		server.onConnection(c)
	}
}

func (server *Server) onConnection(c *Connection) {
	server.sessions[server.sessionCount] = &Session{context: server.context, sessionId: server.sessionCount}
	go server.sessions[server.sessionCount].HandleConnection(c)
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
	chunkSize       int
	streamCount     int
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
	session.streamChannel = make(chan MessageResult, 4)
	session.streamCount = 6
	session.conn = conn
	session.chunkSize = 128
	session.CompleteHandshake()
	conn.Flush()
	err := session.Run()
	if err != nil {
		logger.ErrorLog.Println("Server failed", err)
	}
	return err
}

func (session *Session) Run() error {
	session.messageStreamer.Init(Test{session.conn.Conn}, session.chunkSize)
	go session.ReadMessages()
	for {
		session.conn.SetDeadline(time.Now().Add(time.Second * 5))
		select {
		case msgResult := <-session.messageChannel:
			if msgResult.Err != nil {
				return msgResult.Err
			}
			err := session.HandleMessage(msgResult.Message)
			if err != nil {
				return err
			}
		default:
		}
		select {
		case msgResult := <-session.streamChannel:
			if msgResult.Err != nil {
				return msgResult.Err
			}
			mediaMsg := msgResult.Message
			mediaMsg.MessageStreamId = session.streamCount - 1
			err := session.messageStreamer.WriteMessageToStream(mediaMsg)
			if err != nil {
				return err
			}

		default:
		}

	}

}

func (session *Session) ReadMessages() {
	for {
		msg, err := session.messageStreamer.ReadMessageFromStream()
		if err != nil {
			session.messageChannel <- MessageResult{rtmpMsg.Message{}, err}
			close(session.messageChannel)
			return
		}
		//Any write operations to the message streamer must be handled here to avoid data race
		switch msg.MessageType {
		case rtmpMsg.SetChunkSize:
			session.handleSetChunkSize(msg.MessageData)
			continue
		case rtmpMsg.WindowAckSize:
			session.messageStreamer.MaxWindowAckSize = int(binary.BigEndian.Uint32(msg.MessageData[:4]))
			continue
		}
		session.messageChannel <- MessageResult{msg, err}
	}
}

func (session *Session) HandleMessage(msg rtmpMsg.Message) error {
	var err error
	switch msg.MessageType {

	case 0:
		//logger.ErrorLog.Println("Message type 0")

	case rtmpMsg.UserControl:
		logger.WarningLog.Println("User control Message Sent - Unhandled")

	case rtmpMsg.CommandMsg0, rtmpMsg.CommandMsg3:
		err = session.handleCommandMessage(msg.MessageData)

	case rtmpMsg.DataMsg0:
		err = session.handleDataMessage(msg.MessageData)

	case rtmpMsg.AudioMsg:
		err = session.handleAudioMessage(msg.MessageData)

	case rtmpMsg.VideoMsg:
		err = session.handleVideoMessage(msg.MessageData)

	case rtmpMsg.Ack:

	default:
		logger.ErrorLog.Fatalln("Unimplemented message type", msg.MessageType)
	}
	if err != nil {
		return err
	}
	//session.conn.Flush()
	return nil

}

func (session *Session) handleSetChunkSize(data []byte) {
	if data[0] != 0 {
		logger.WarningLog.Println("Set chunk size message err - first bit not 0", data)
		return
	}
	if len(data) != 4 {
		logger.WarningLog.Println("Set chunk size payload err - payload length not 4")
	}
	chunkSize := binary.BigEndian.Uint32(data[0:4])
	logger.InfoLog.Println("Setting chunk size:", chunkSize)
	session.messageStreamer.SetChunkSize(int(chunkSize))
}

func (session *Session) handleCommandMessage(data []byte) error {
	amfObjects, _, _ := amf.DecodeBytes(data)
	if len(amfObjects) == 0 {
		logger.InfoLog.Fatalln("No objects decoded")
		return errors.New("no objects decoded")
	}
	commandName, ok := amfObjects[0].(string)
	if !ok {
		logger.ErrorLog.Fatalln("Command message payload does not have a string(cmd message) as first amf object")
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
	case "releaseStream", "FCPublish", "FCUnpublish", "getStreamLength", "deleteStream":
		logger.WarningLog.Println("Unknown command message received. Unhandled. Objects:", amfObjects)
	default:
		logger.InfoLog.Fatalln("Unimplemented command - cmdName: ", amfObjects)
	}
	return nil
}

func (session *Session) handleConnectCommand(objects []interface{}) error {
	var result string
	if len(objects) <= 3 {
		result = "_result"
	} else {
		result = "_error"
	}
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
	setChunkMsg := rtmpMsg.NewSetChunkSizeMessage(4096)

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
	session.messageStreamer.SetChunkSize(4096)

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
		responseData = amf.EncodeAMF0("_result", transactionId, nil, session.streamCount)
		//session.streamCount++
	}

	cmdMsg := rtmpMsg.NewCommandMessage0(responseData, 0, COMMAND_MESSAGE_CHUNK_STREAM)
	logger.InfoLog.Println("Sending create stream response.")
	session.messageStreamer.WriteMessageToStream(cmdMsg)
	return nil
}

func (session *Session) handlePlayCommand(objects []interface{}) error {
	if len(objects) < 4 {
		return errors.New("invalid objects")
	}
	streamName, ok := objects[3].(string)
	if !ok {
		return errors.New("invalid stream name")
	}
	publisher := session.context.GetPublisher(streamName)
	if publisher == nil {
		session.curStream = streamName
		session.context.AppendToWaitlist(streamName, Subscriber{session.streamChannel, session.sessionId})
		return nil
	}
	session.messageStreamer.WriteMessageToStream(rtmpMsg.NewSetChunkSizeMessage(4096))
	session.messageStreamer.SetChunkSize(4096)
	session.messageStreamer.WriteMessageToStream(rtmpMsg.NewStreamIsRecordedMessage(uint32(session.streamCount)))
	session.messageStreamer.WriteMessageToStream(rtmpMsg.NewStreamBeginMessage(uint32(session.streamCount)))
	playStartMsg := rtmpMsg.NewStatusMessage("status", "NetStream.Play.Reset", "Playing and resetting stream", session.streamCount)
	playReset := rtmpMsg.NewStatusMessage("status", "NetStream.Play.Start", "Started playing stream.", session.streamCount)
	if true {
		session.messageStreamer.WriteMessageToStream(playReset)
	}
	session.messageStreamer.WriteMessageToStream(playStartMsg)

	publisher.AddSubscriber(session.streamChannel)
	session.curStream = streamName
	metaDataMsg := rtmpMsg.NewMessage(publisher.Metadata, rtmpMsg.DataMsg0, session.streamCount, COMMAND_MESSAGE_CHUNK_STREAM)
	session.messageStreamer.WriteMessageToStream(metaDataMsg)
	if aacSeqHeader := publisher.GetAACSequenceHeader(); aacSeqHeader != nil {
		aacMsg := rtmpMsg.NewMessage(aacSeqHeader, rtmpMsg.AudioMsg, session.streamCount, AUDIO_MESSAGE_CHUNK_STREAM)
		err := session.messageStreamer.WriteMessageToStream(aacMsg)
		if err != nil {
			return err
		}
	}
	if avcSeqHeader := publisher.GetAVCSequenceHeader(); avcSeqHeader != nil {
		avcMsg := rtmpMsg.NewMessage(avcSeqHeader, rtmpMsg.VideoMsg, session.streamCount, VIDEO_MESSAGE_CHUNK_STREAM)
		err := session.messageStreamer.WriteMessageToStream(avcMsg)
		if err != nil {
			return err
		}
	}

	return nil
}

//very incomplete
func (session *Session) handlePublishCommand(objects []interface{}) error {
	if len(objects) < 5 {
		return errors.New("invalid objects")
	}
	_, ok := session.context.GetStreamName(session.sessionId)
	if ok {
		err := session.messageStreamer.WriteMessageToStream(rtmpMsg.NewStatusMessage("error", "NetStream.Publish.BadConnection", "Connection already publishing", 0))
		return err
	}
	if !session.state.Connected {
		logger.WarningLog.Println("Client attempted publishing without connecting")
		return nil
	}
	streamName, ok := objects[3].(string)
	if !ok {
		return errors.New("invalid stream name")
	}
	session.context.SetStreamName(session.sessionId, streamName)
	session.context.SetPublisher(streamName, &Publisher{
		SessionId:   session.sessionId,
		Subscribers: []Subscriber{},
		Metadata:    session.ClientMetadata})

	session.messageStreamer.WriteMessageToStream(rtmpMsg.NewStatusMessage(
		"status",
		"NetStream.Publish.Start",
		"Stream published",
		0))

	return nil
}

func (session *Session) handleDataMessage(data []byte) error {
	objs, _, _ := amf.DecodeBytes(data)
	if len(objs) < 3 {
		//handle bad data message
		return nil
	}
	buf := amf.EncodeAMF0("onMetaData", objs[2])
	dataMsg := rtmpMsg.NewCommandMessage0(buf, 0, COMMAND_MESSAGE_CHUNK_STREAM)
	session.ClientMetadata = data
	logger.InfoLog.Println("Data message received. AMF?:", objs)
	if streamName, ok := session.context.GetStreamName(session.sessionId); ok {
		if publisher := session.context.GetPublisher(streamName); publisher != nil {
			publisher.Metadata = data
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
			msg := rtmpMsg.NewMessage(data, rtmpMsg.AudioMsg, session.streamCount, AUDIO_MESSAGE_CHUNK_STREAM)
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
			msg := rtmpMsg.NewMessage(data, rtmpMsg.VideoMsg, session.streamCount, VIDEO_MESSAGE_CHUNK_STREAM)
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
	conn := session.conn.ReadWriter

	//read C0
	clientVersion, err := conn.ReadByte()
	if err != nil {
		return err
	}
	//send S0
	conn.WriteByte(clientVersion)
	rando := make([]byte, S1SIZE)
	conn.Write(rando)
	conn.Flush()
	data := make([]byte, C1SIZE)
	n, err := conn.Read(data)
	if err != nil {
		return err
	}
	if n < len(data) {
		return errors.New("failed to read handshake from client")
	}
	conn.Write(data)
	conn.Flush()
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
