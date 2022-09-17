package main

import (
	rtmp "github.com/tj03/rtmp/src/rtmp"
)

func main() {
	s := rtmp.NewRTMPServer()
	s.Listen(1935)

}
