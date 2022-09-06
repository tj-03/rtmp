package rtmp

import (
	"bufio"
	"encoding/binary"
	"io"
	"net"
	"strconv"

	"github.com/tj03/rtmp/src/amf"
	"github.com/tj03/rtmp/src/logger"
	"github.com/tj03/rtmp/src/message"
)

//Stream context will be a pub sub object that will allow for different connections to create streams to send data to and receive data from. Each time a stream is written to,
//the stream context will forward that data to all subscirbers of that stream
type StreamContext struct {
	//unimplemented
}

var COMMAND_MESSAGE_CHUNK_STREAM = 3

type Context struct {
	streamContext StreamContext
}
type Server struct {
	//change to arr?
	sessions     map[int]*Session
	context      Context
	sessionCount int
}

func (server *Server) Listen(port int) error {
	tcpSocket, err := net.Listen("tcp", ":"+strconv.FormatInt(int64(port), 10))
	if err != nil {
		logger.ErrorLog.Println(err)
		return err
	}
	logger.InfoLog.Println("Listening on port", port)
	server.sessions = make(map[int]*Session)
	defer tcpSocket.Close()
	for {
		tcpConnection, err := tcpSocket.Accept()
		conn := bufio.NewReadWriter(bufio.NewReader(tcpConnection), bufio.NewWriter(tcpConnection))
		//change this eventually -> we dont want to crash because of one failed connection
		if err != nil {
			return err
		}
		server.OnConnection(conn)
	}
}

func (server *Server) OnConnection(c *bufio.ReadWriter) {
	server.sessions[server.sessionCount] = &Session{context: &server.context}
	go server.sessions[server.sessionCount].HandleConnection(c)
	server.sessionCount += 1
}

type Handler struct {
}

type RTMPSessionState int

const (
	Uninitialized RTMPSessionState = iota
	HandshakeCompleted
)

type Session struct {
	conn            *bufio.ReadWriter
	state           RTMPSessionState
	context         *Context
	chunkSize       int
	streamCount     int
	messageStreamer message.MessageStreamer
}

func (session *Session) HandleConnection(conn *bufio.ReadWriter) error {
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
	session.messageStreamer.Init(session.conn, session.chunkSize)

	for {
		err, msg := session.messageStreamer.ReadMessageFromStream()
		if err != nil {
			logger.ErrorLog.Println(err)
			return err
		}
		logger.InfoLog.Println("New message", msg)
		switch msg.MessageType {
		case message.SetChunkSize:
			session.handleSetChunkSize(msg.MessageData)

		case message.CommandMsg0, message.CommandMsg3:
			session.handleCommandMessage(msg.MessageData)
		case message.Ack:
			//session.messageStreamer.WriteMessageToStream(message.NewMessage([]byte{0, 6, 0, 0, 0, 0}, message.UserControl, 0))
		default:
			logger.ErrorLog.Fatalln("Unimplemented message type", msg.MessageType)
		}
	}
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
	case "":

	case "releaseStream", "FCPublish":
		logger.WarningLog.Println("Unknown command message received. Unhandled. Objects:", amfObjects)
	default:
		logger.InfoLog.Fatalln("Unimplemented command - cmdName: ", amfObjects)
	}
	return nil
}

func (session *Session) handleConnectCommand(objects []interface{}) {
	response := []byte{}
	var result string
	if len(objects) <= 3 {
		result = "_result"
	} else {
		result = "_error"
	}
	logger.InfoLog.Println("Connect object from client", objects[2])
	encoded := append(amf.EncodeAMF0(result), amf.EncodeAMF0(1)...)
	info := amf.EncodeAMF0(map[string]interface{}{
		"fmsVer":       "FMS/3,0,1,123",
		"capabilities": 31,
	})

	props := amf.EncodeAMF0(map[string]interface{}{
		"level":          "status",
		"code":           "NetConnection.Connect.Success",
		"description":    "Connection succeeds",
		"objectEncoding": 0,
	})
	response = append(response, encoded...)
	response = append(response, info...)
	response = append(response, props...)

	winAckMsg := message.NewWinAckMessage(5000000)
	setBandwidthMsg := message.NewSetPeerBandwidthMessage(5000000, 2)
	streamBeginMsg := message.NewStreamBeginMessage(0)
	setChunkMsg := message.NewSetChunkSizeMessage(8192)

	logger.InfoLog.Println("Writing window ack bytes to stream", winAckMsg)
	session.messageStreamer.WriteMessageToStream(winAckMsg)

	logger.InfoLog.Println("Writing set bandwidth message", setBandwidthMsg)
	session.messageStreamer.WriteMessageToStream(setBandwidthMsg)

	logger.InfoLog.Println("Writing set chunk size message", setChunkMsg)
	session.messageStreamer.WriteMessageToStream(setChunkMsg)
	session.messageStreamer.SetChunkSize(8192)

	logger.InfoLog.Println("Writing stream begin message")
	session.messageStreamer.WriteMessageToStream(streamBeginMsg)

	resMsg := message.NewCommandMessage0(response, 20, 3)
	logger.InfoLog.Println("Writing connect response", resMsg)
	session.messageStreamer.WriteMessageToStream(resMsg)
}

func (session *Session) handleCreateStreamCommand(objects []interface{}) error {
	responseData := []byte{}
	validObjects := len(objects) >= 2
	var transactionId float64
	var ok bool
	if validObjects {
		transactionId, ok = objects[1].(float64)
		validObjects = validObjects && ok
	}
	if !validObjects {
		responseData = append(responseData, amf.EncodeAMF0("_error")...)
		responseData = append(responseData, amf.EncodeAMF0(-1)...)
		responseData = append(responseData, amf.EncodeAMF0(nil)...)
		responseData = append(responseData, amf.EncodeAMF0(map[string]interface{}{"error": "No transactionId"})...)
	} else {
		responseData = append(responseData, amf.EncodeAMF0("_result")...)
		responseData = append(responseData, amf.EncodeAMF0(transactionId)...)
		responseData = append(responseData, amf.EncodeAMF0(nil)...)
		responseData = append(responseData, amf.EncodeAMF0(session.streamCount)...)
		session.streamCount++
	}

	cmdMsg := message.NewCommandMessage0(responseData, 0, COMMAND_MESSAGE_CHUNK_STREAM)
	session.messageStreamer.WriteMessageToStream(cmdMsg)
	return nil
}

func (session *Session) CompleteHandshake() error {
	//RTMPVersion := 3
	conn := session.conn
	//ignore client version
	//read C0
	clientVersion, err := conn.Reader.ReadByte()
	if err != nil {
		return err
	}
	//send S0
	conn.Writer.WriteByte(clientVersion)
	rando := make([]byte, S1SIZE)
	rando[0] = 35
	rando[1] = 010
	conn.Writer.Write(rando)
	data := make([]byte, C1SIZE)
	n, err := io.ReadFull(conn.Reader, data)
	if err != nil {
		panic("bad read handshake")
	}
	if n < len(data) {
		panic("Didnt read everything in handshae")
	}
	conn.Write(data)
	conn.Flush()
	n, err = io.ReadFull(conn.Reader, data)
	if err != nil {
		panic("bdd read handshake")
	}
	if n < len(data) {
		panic("Didnt read everything in handshae")
	}
	return nil

}
