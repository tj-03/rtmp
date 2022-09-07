package rtmp

import (
	"errors"
	"net"
	"sync"
)

//Stream context will be a pub sub object that will allow for different connections to create streams to send data to and receive data from. Each time a stream is written to,
//the stream context will forward that data to all subscirbers of that stream

//Currently testing using only one connection to the server so for now this will work HOWEVER, this is not thread safe with how rtmp.go uses it
//Multiple goroutines could be read/writing the ClientStreams map and the Publishers map and this will cause a data race (map not thread safe)
type Publisher struct {
	subscriberLock sync.RWMutex
	SessionId      int
	Subscribers    []Subscriber
	//AMF0 Encoded Video MetaData
	Metadata []byte
}

type Subscriber struct {
	net.Conn
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

func (publisher *Publisher) AddSubscriber(conn net.Conn) error {
	if conn == nil {
		return errors.New("connection is nil")
	}
	publisher.subscriberLock.Lock()
	publisher.Subscribers = append(publisher.Subscribers, Subscriber{Conn: conn})
	publisher.subscriberLock.Unlock()
	return nil
}

func (publisher *Publisher) GetSubscribers() []Subscriber {
	publisher.subscriberLock.RLock()
	subs := make([]Subscriber, 0, len(publisher.Subscribers))
	copy(subs, publisher.Subscribers)
	publisher.subscriberLock.RUnlock()
	return subs
}
