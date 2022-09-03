package rtmp

import "github.com/tj03/rtmp/src/message"

type RTMPHandler interface {
	onData(msg *message.Message) error
}
