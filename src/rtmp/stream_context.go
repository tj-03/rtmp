package rtmp

import (
	"bufio"
	"errors"
	"sync"
)

//Currently testing using only one connection to the server so for now this will work HOWEVER, this is not thread safe with how rtmp.go uses it
//Multiple goroutines could be read/writing the ClientStreams map and the Publishers map and this will cause a data race (map not thread safe)
type Publisher struct {
	subscriberLock sync.RWMutex
	SessionId      int
	Subscribers    []*bufio.ReadWriter
	//AMF0 Encoded Video MetaData
	Metadata []byte
}
type Context struct {
	publishersLock    sync.RWMutex
	clientStreamsLock sync.RWMutex
	ClientStreams     map[int]string
	Publishers        map[string]*Publisher
}

func (ctx *Context) GetPublisher(streamName string) *Publisher {
	ctx.publishersLock.RLock()
	publisher := ctx.Publishers[streamName]
	ctx.publishersLock.RUnlock()
	return publisher
}

func (ctx *Context) SetPublisher(streamName string, publisher *Publisher) {
	ctx.publishersLock.Lock()
	ctx.Publishers[streamName] = publisher
	ctx.publishersLock.Unlock()
}

func (publisher *Publisher) AddSubscriber(conn *bufio.ReadWriter) error {
	if conn == nil {
		return errors.New("connection is nil")
	}
	publisher.subscriberLock.Lock()
	publisher.Subscribers = append(publisher.Subscribers, conn)
	publisher.subscriberLock.Unlock()
	return nil
}

func (publisher *Publisher) GetSubscribers() []*bufio.ReadWriter {
	publisher.subscriberLock.RLock()
	subs := make([]*bufio.ReadWriter, 0, len(publisher.Subscribers))
	copy(subs, publisher.Subscribers)
	publisher.subscriberLock.RUnlock()
	return subs
}
