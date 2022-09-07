package rtmp

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"strconv"

	"github.com/tj03/rtmp/src/amf"
	"github.com/tj03/rtmp/src/logger"
	"github.com/tj03/rtmp/src/message"
)

var COMMAND_MESSAGE_CHUNK_STREAM = 3

type Server struct {
	//change to arr?
	sessions     map[int]*Session
	context      Context
	sessionCount int
}

type Connection struct {
	bufio.ReadWriter
	net.Conn
}

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

func (server *Server) Listen(port int) error {
	tcpSocket, err := net.Listen("tcp", ":"+strconv.FormatInt(int64(port), 10))
	if err != nil {
		logger.ErrorLog.Println(err)
		return err
	}
	defer tcpSocket.Close()
	logger.InfoLog.Println("Listening on port", port)
	server.sessions = make(map[int]*Session)
	for {
		tcpConnection, err := tcpSocket.Accept()
		if err != nil {
			return err
		}
		logger.InfoLog.Println("New connection!", tcpConnection.RemoteAddr().String())
		fmt.Print("New conneciton", tcpConnection.RemoteAddr().String())
		//If the buffer size is tool small bufio.Reader will fail to read messages with a payload close to the buffer size
		//Not sure why
		bufferSize := 4096 * 64
		bufferedConnection := bufio.NewReadWriter(bufio.NewReaderSize(tcpConnection, bufferSize), bufio.NewWriter(tcpConnection))

		//change this eventually -> we dont want to crash the whole server because of one failed connection
		c := &Connection{*bufferedConnection, tcpConnection}

		server.OnConnection(c)
	}
}

func (server *Server) OnConnection(c *Connection) {
	server.context.ClientStreams = make(map[int]string)
	server.context.Publishers = map[string]*Publisher{}
	server.sessions[server.sessionCount] = &Session{context: &server.context, sessionId: server.sessionCount}
	go server.sessions[server.sessionCount].HandleConnection(c)
	server.sessionCount += 1
}

type RTMPSessionState int

const (
	Disconnected RTMPSessionState = iota
	Connected
)

type Session struct {
	conn            *Connection
	sessionId       int
	state           RTMPSessionState
	context         *Context
	chunkSize       int
	streamCount     int
	playStream      chan message.Message
	messageStreamer message.MessageStreamer
}

func (session *Session) HandleConnection(conn *Connection) error {
	defer conn.Close()
	session.playStream = make(chan message.Message, 256)
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
	session.messageStreamer.Init(session.conn.ReadWriter, session.chunkSize)
	defer session.conn.Close()
	for {
		msg, err := session.messageStreamer.ReadMessageFromStream()
		if err != nil {
			logger.ErrorLog.Println(err)
			return err
		}
		session.HandleMessage(msg)
	}

}

func (session *Session) HandleMessage(msg message.Message) error {
	b := 0
	session.messageStreamer.WriteMessageToStream(message.NewAckMessage(uint32(b)))
	b += 2000

	previewSize := 32
	if previewSize > len(msg.MessageData) {
		previewSize = len(msg.MessageData)
	}
	if true || msg.MessageType != message.VideoMsg && msg.MessageType != message.AudioMsg {
		logger.InfoLog.Println("New message", "Type: ", msg.MessageType, "StreamId: ", msg.MessageStreamId, "Length: ", len(msg.MessageData), msg.MessageData[:previewSize])
	}
	switch msg.MessageType {

	case message.SetChunkSize:
		session.handleSetChunkSize(msg.MessageData)

	case message.CommandMsg0, message.CommandMsg3:
		session.handleCommandMessage(msg.MessageData)

	case message.DataMsg0:
		session.handleDataMessage(msg.MessageData)

	case message.AudioMsg:
		session.handleAudioMessage(msg.MessageData)

	case message.VideoMsg:
		session.handleVideoMessage(msg.MessageData)

	case message.Ack:
		//session.messageStreamer.WriteMessageToStream(message.NewMessage([]byte{0, 6, 0, 0, 0, 0}, message.UserControl, 0))

	default:
		logger.ErrorLog.Fatalln("Unimplemented message type", msg.MessageType)
	}
	session.conn.Flush()
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
	}
	commandName, ok := amfObjects[0].(string)
	if !ok {
		logger.ErrorLog.Fatalln("Command message payload does not have a string(cmd message) as first amf object")
	}

	switch commandName {
	case "connect":
		session.handleConnectCommand(amfObjects)
	case "createStream":
		session.handleCreateStreamCommand(amfObjects)
	case "publish":
		session.handlePublishCommand(amfObjects)
	case "releaseStream", "FCPublish":
		logger.WarningLog.Println("Unknown command message received. Unhandled. Objects:", amfObjects)
	default:
		logger.InfoLog.Fatalln("Unimplemented command - cmdName: ", amfObjects)
	}
	return nil
}

func (session *Session) handleConnectCommand(objects []interface{}) {
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

	winAckMsg := message.NewWinAckMessage(5000000)
	setBandwidthMsg := message.NewSetPeerBandwidthMessage(5000000, 2)
	streamBeginMsg := message.NewStreamBeginMessage(0)
	setChunkMsg := message.NewSetChunkSizeMessage(4096)

	logger.InfoLog.Println("Writing window ack bytes to stream", winAckMsg)
	session.messageStreamer.WriteMessageToStream(winAckMsg)

	logger.InfoLog.Println("Writing set bandwidth message", setBandwidthMsg)
	session.messageStreamer.WriteMessageToStream(setBandwidthMsg)

	logger.InfoLog.Println("Writing set chunk size message", setChunkMsg)
	session.messageStreamer.WriteMessageToStream(setChunkMsg)
	session.messageStreamer.SetChunkSize(4096)

	logger.InfoLog.Println("Writing stream begin message")
	session.messageStreamer.WriteMessageToStream(streamBeginMsg)

	resMsg := message.NewCommandMessage0(response, 0, COMMAND_MESSAGE_CHUNK_STREAM)
	logger.InfoLog.Println("Writing connect response", resMsg)
	session.messageStreamer.WriteMessageToStream(resMsg)
	session.state = Connected
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
		session.streamCount++
	}

	cmdMsg := message.NewCommandMessage0(responseData, 0, COMMAND_MESSAGE_CHUNK_STREAM)
	session.messageStreamer.WriteMessageToStream(cmdMsg)
	return nil
}

//very incomplete
func (session *Session) handlePublishCommand(objects []interface{}) error {

	streamName, ok := session.context.ClientStreams[session.sessionId]
	if ok {
		session.messageStreamer.WriteMessageToStream(message.NewStatusMessage("error", "NetStream.Publish.BadConnection", "Connection already publishing", 0))
		return nil
	}
	if session.state == Disconnected {
		logger.WarningLog.Println("Client attempted publishing without connecting")
		return nil
	}

	session.context.Publishers[streamName] = &Publisher{SessionId: session.sessionId, Subscribers: []Subscriber{}, Metadata: nil}
	//	session.context.SetPublisher(&Publisher{SessionId:session.sessionId, Subscribers:[]*bufio.ReadWriter{}, Metadata:nil})
	session.messageStreamer.WriteMessageToStream(message.NewStatusMessage(
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
	logger.InfoLog.Println("Data message received. AMF?:", objs)
	if streamName, ok := session.context.ClientStreams[session.sessionId]; ok {
		if publisher, ok := session.context.Publishers[streamName]; ok {
			publisher.Metadata = data
			for _, subscriber := range publisher.Subscribers {
				_, err := subscriber.Conn.Write(data)
				if err != nil {
					return err
				}
				if err != nil {
					return err
				}
			}
		}
	}
	buf := amf.EncodeAMF0("onMetaData")
	buf = append(buf, amf.EncodeAMF0(buf[2])...)
	session.messageStreamer.WriteMessageToStream(message.NewCommandMessage0(buf, 0, COMMAND_MESSAGE_CHUNK_STREAM))
	return nil
}

func (session *Session) handleAudioMessage(data []byte) error {
	if streamName, ok := session.context.ClientStreams[session.sessionId]; ok {
		if publisher, ok := session.context.Publishers[streamName]; ok {
			for _, subscriber := range publisher.Subscribers {
				_, err := subscriber.Write(data)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (session *Session) handleVideoMessage(data []byte) error {
	if streamName, ok := session.context.ClientStreams[session.sessionId]; ok {
		if publisher, ok := session.context.Publishers[streamName]; ok {
			for _, subscriber := range publisher.Subscribers {
				_, err := subscriber.Write(data)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
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
