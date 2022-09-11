package rtmp

import (
	"errors"
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
	Metadata          []byte
	AACSequenceHeader []byte
}

type Subscriber struct {
	StreamChannel chan MessageResult
	SessionId     int
}

type Context struct {
	publishersLock    sync.RWMutex
	clientStreamsLock sync.RWMutex
	clientStreams     map[int]string
	waitLists         map[string][]Subscriber
	publishers        map[string]*Publisher
}

func (ctx *Context) GetPublisher(streamName string) *Publisher {
	ctx.publishersLock.RLock()
	publisher := ctx.publishers[streamName]
	ctx.publishersLock.RUnlock()
	return publisher
}

func (ctx *Context) SetPublisher(streamName string, publisher *Publisher) {
	ctx.publishersLock.Lock()
	ctx.publishers[streamName] = publisher
	waitList := ctx.waitLists[streamName]
	if waitList != nil {
		ctx.publishers[streamName].Subscribers = waitList
		ctx.waitLists[streamName] = nil
	}
	ctx.publishersLock.Unlock()
}

func (ctx *Context) GetStreamName(sessionId int) (string, bool) {
	ctx.clientStreamsLock.RLock()
	streamName, ok := ctx.clientStreams[sessionId]
	ctx.clientStreamsLock.RUnlock()
	return streamName, ok
}

func (ctx *Context) SetStreamName(sessionId int, streamName string) {
	ctx.publishersLock.Lock()
	ctx.clientStreams[sessionId] = streamName
	ctx.publishersLock.Unlock()
}

func (publisher *Publisher) AddSubscriber(channel chan MessageResult) error {
	if channel == nil {
		return errors.New("channel is nil")
	}
	publisher.subscriberLock.Lock()
	publisher.Subscribers = append(publisher.Subscribers, Subscriber{StreamChannel: channel})
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
