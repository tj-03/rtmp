package rtmp

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"

	"github.com/tj03/rtmp/src/amf"
	"github.com/tj03/rtmp/src/logger"
	"github.com/tj03/rtmp/src/message"
	"github.com/tj03/rtmp/src/parser"
)

//Stream context will be a pub sub object that will allow for different connections to create streams to send data to and receive data from. Each time a stream is written to,
//the stream context will forward that data to all subscirbers of that stream
type StreamContext struct {
	//unimplemented
}

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
	conn      *bufio.ReadWriter
	state     RTMPSessionState
	context   *Context
	chunkSize int
}

func (session *Session) HandleConnection(conn *bufio.ReadWriter) error {
	session.conn = conn
	session.chunkSize = 128
	session.CompleteHandshake()
	conn.Flush()
	return session.Run()
}

func (session *Session) Run() error {
	var messageReader message.MessageReader
	messageReader.Init(session.conn, session.chunkSize)
	for {
		err, msg := messageReader.ReadMessageFromStream()
		if err != nil {
			logger.ErrorLog.Println(err)
			return err
		}
		switch msg.MessageType {
		case message.SetChunkSize:
			session.handleSetChunkSize(msg.MessageData, &messageReader)
		case message.CommandMsg0, message.CommandMsg3:

			logger.InfoLog.Println("Lenfgth of amf0 bytes", len(msg.MessageData))
			session.handleCommandMessage(msg.MessageData)
		default:
			logger.ErrorLog.Fatalln("Unimplemented message type", msg.MessageType)
		}

	}
}

func (session *Session) handleSetChunkSize(data []byte, messageReader *message.MessageReader) {
	if data[0] != 0 {
		logger.WarningLog.Println("Set chunk size message err - first bit not 0", data)
		return
	}
	if len(data) != 4 {
		logger.WarningLog.Println("Set chunk size payload err - payload length not 4")
	}
	chunkSize := binary.BigEndian.Uint32(data[0:4])
	logger.InfoLog.Println("Setting chunk size:", chunkSize)
	messageReader.SetChunkSize(int(chunkSize))
}

func (session *Session) handleCommandMessage(data []byte) {
	arr, _, _ := amf.DecodeBytes(data)
	logger.InfoLog.Println("Total AMF values = ", len(arr))

	if false {
		logger.ErrorLog.Fatalln("Command message payload does not have a string(cmd message) as first amf object")
	}
	logger.InfoLog.Fatalln("Command name = ", arr[2].(map[string]interface{}))

}

func (session *Session) CompleteHandshake() error {
	RTMPVersion := 3
	conn := session.conn
	//ignore client version
	//read C0
	_, err := conn.Reader.ReadByte()
	if err != nil {
		return err
	}
	//send S0
	conn.Writer.WriteByte(byte(RTMPVersion))
	conn.Writer.Write(make([]byte, parser.C1SIZE))
	data := make([]byte, parser.C1SIZE)
	n, err := io.ReadFull(conn.Reader, data)
	if err != nil {
		panic("bad read handshake")
	}
	if n < len(data) {
		panic("Didnt read everything in handshae")
	}
	conn.Write(make([]byte, parser.C1SIZE))
	conn.Flush()
	n, err = io.ReadFull(conn.Reader, data)
	if err != nil {
		panic("bdd read handshake")
	}
	if n < len(data) {
		panic("Didnt read everything in handshae")
	}

	fmt.Println("C2", data)
	return nil

}
