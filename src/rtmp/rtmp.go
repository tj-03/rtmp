package rtmp

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strconv"

	"github.com/tj03/rtmp/src/chunk"
	"github.com/tj03/rtmp/src/parser"
)

type StreamContext struct {
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
		return err
	}
	fmt.Println("Listening on port", port)
	server.sessions = make(map[int]*Session)
	defer tcpSocket.Close()
	for {
		rawConnection, err := tcpSocket.Accept()
		conn := bufio.NewReadWriter(bufio.NewReader(rawConnection), bufio.NewWriter(rawConnection))
		//change this eventually -> we dont want to crash because of one failed connection
		if err != nil {
			return err
		}
		server.OnConnection(conn)
	}
	return nil
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
	conn         *bufio.ReadWriter
	state        RTMPSessionState
	chunkStreams chunk.ChunkStreams
	context      *Context
	chunkSize    int
}

func (session *Session) HandleConnection(conn *bufio.ReadWriter) error {
	session.conn = conn
	session.chunkSize = 128
	session.CompleteHandshake()
	conn.Flush()
	err, c := chunk.NewChunkFromStream(session.conn.Reader, session.chunkSize)
	if err != nil {
		return err
	}
	fmt.Println("First chunk basic head", c.BasicHeader)
	fmt.Println("First chunk message head", c.MessageHeader)
	fmt.Println("First chunk data", c.ChunkData)
	conn.Write(make([]byte, 2048))
	err, c = chunk.NewChunkFromStream(session.conn.Reader, session.chunkSize)
	if err != nil {
		return err
	}
	fmt.Println("First chunk basic head", c.BasicHeader)
	fmt.Println("First chunk message head", c.MessageHeader)
	fmt.Println("First chunk data", c.ChunkData)
	return nil
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
		return err
	}
	if n < len(data) {
		panic("Didnt read everything in handshae")
	}
	conn.Writer.Write(make([]byte, parser.C1SIZE))
	conn.Writer.Flush()
	n, err = io.ReadFull(conn.Reader, data)
	if err != nil {
		return err
	}
	if n < len(data) {
		panic("Didnt read everything in handshae")
	}

	fmt.Println("C2", data)
	return nil

}
