package rtmp

import "net"

type StreamContext struct {
}
type Server struct {
	//change to arr?
	sessions      map[int]*Session
	streamContext StreamContext
	sessionCount  int
}

func (s *Server) OnConnection(c net.Conn) {
	s.sessions[s.sessionCount] = &Session{}
	s.sessionCount += 1

	go s.sessions[0].HandleConn(c)

}

type Handler struct {
}

type Session struct {
	conn net.Conn
}

func (s *Session) HandleConn(c net.Conn) {

}
