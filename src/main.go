package main

import (
	_ "net/http/pprof"

	rtmp "github.com/tj03/rtmp/src/rtmp"
)

func main() {
	// config, err := util.ReadConfig()
	// if err != nil {
	// 	fmt.Println(err)
	// 	return
	// }
	// fmt.Println(config)
	// return
	s := rtmp.NewRTMPServer()
	s.Listen(1935)

}
