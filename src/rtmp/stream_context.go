package rtmp

import (
	"errors"
	"sync"
)

//Holds data about the publisher and associated subscribers and provides methods to read/write via RWMutex.
type Publisher struct {
	subscriberLock sync.RWMutex
	SessionId      int
	Subscribers    []Subscriber
	//AMF0 Encoded Video MetaData
	Metadata []byte
	//AAC and AVC sequence headers MUST be sent to subscriber before sending any video/audio data
	AACSequenceHeader []byte
	AVCSequenceHeader []byte
}

type Subscriber struct {
	StreamChannel chan MessageResult
	SessionId     int
}

//Context holds information about different RTMP sessions to enable different sessions to communicate with each other via channels.
//Provides methods to read/write data using RWMutex.
type Context struct {
	publishersLock    sync.RWMutex
	clientStreamsLock sync.RWMutex
	waitListLock      sync.Mutex
	clientStreams     map[int]string
	//Sessions wait here when a stream is not yet published.
	waitLists  map[string][]Subscriber
	publishers map[string]*Publisher
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
	ctx.waitListLock.Lock()
	waitList := ctx.waitLists[streamName]
	if waitList != nil {
		ctx.publishers[streamName].Subscribers = waitList
		ctx.waitLists[streamName] = nil
	}
	ctx.waitListLock.Unlock()
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

func (ctx *Context) AppendToWaitlist(streamName string, subscriber Subscriber) {
	ctx.waitListLock.Lock()
	waitList := ctx.waitLists[streamName]
	if waitList == nil {
		waitList = make([]Subscriber, 0)
	}
	waitList = append(waitList, subscriber)
	ctx.waitLists[streamName] = waitList
	ctx.waitListLock.Unlock()
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
