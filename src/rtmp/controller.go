package rtmp

import "github.com/tj03/rtmp/src/message"

type RTMPController interface {
	onData(msg *message.Message) error
	onCommand(msg *message.Message) error
}
