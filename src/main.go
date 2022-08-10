package main

import (
	rtmp "github.com/tj03/rtmp/src/rtmp"
)

func main() {
	s := rtmp.Server{}
	s.Listen(8080)
}
