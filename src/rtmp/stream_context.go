package rtmp

import (
	"errors"
	"sync"

	"github.com/tj03/rtmp/src/logger"
	rtmpMsg "github.com/tj03/rtmp/src/message"
	"github.com/tj03/rtmp/src/util"
)

//Holds data about the publisher and associated subscribers and provides methods to read/write via RWMutex.
type Publisher struct {
	subscriberLock sync.RWMutex
	aacHeaderLock  sync.RWMutex
	avcHeaderLock  sync.RWMutex
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

func (ctx *Context) RemovePublisher(sessionId int) {
	ctx.publishersLock.Lock()
	ctx.clientStreamsLock.Lock()
	streamName, ok := ctx.clientStreams[sessionId]
	if !ok {
		ctx.publishersLock.Unlock()
		ctx.clientStreamsLock.Unlock()
		return
	}
	publisher := ctx.publishers[streamName]
	if publisher == nil {
		panic("Stream name set but clientstream not set")
	}
	publisher.BroadcastMessage(rtmpMsg.NewStatusMessage("status", "NetStream.Play.UnpublishNotify", "Stream unpublished", COMMAND_MESSAGE_CHUNK_STREAM))

	delete(ctx.publishers, streamName)
	delete(ctx.clientStreams, sessionId)
	ctx.clientStreamsLock.Unlock()
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

func (ctx *Context) RemoveFromWaitlist(sessionId int, streamName string) {
	ctx.waitListLock.Lock()
	waitList := ctx.waitLists[streamName]
	filtered := util.FilterSlice(waitList, func(s Subscriber) bool {
		return s.SessionId != sessionId
	})
	if len(filtered) == len(waitList) {
		logger.WarningLog.Printf("Subscriber with id = %d was not int waitlist", sessionId)
	}
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

func (publisher *Publisher) BroadcastMessage(msg rtmpMsg.Message) {
	publisher.subscriberLock.RLock()
	for i := range publisher.Subscribers {
		select {
		case publisher.Subscribers[i].StreamChannel <- MessageResult{msg, nil}:

		default:
			logger.WarningLog.Printf("Subscriber with id = %d could not process message", publisher.Subscribers[i].SessionId)

		}
	}
	publisher.subscriberLock.RUnlock()
}

func (publisher *Publisher) SetAACSequenceHeader(seqHeader []byte) {
	publisher.aacHeaderLock.Lock()
	publisher.AACSequenceHeader = seqHeader
	publisher.aacHeaderLock.Unlock()
}

func (publisher *Publisher) GetAACSequenceHeader() []byte {
	if publisher.AACSequenceHeader == nil {
		return nil
	}
	publisher.aacHeaderLock.RLock()
	seqHeaderCopy := make([]byte, len(publisher.AACSequenceHeader))
	copy(seqHeaderCopy, publisher.AACSequenceHeader)
	publisher.aacHeaderLock.RUnlock()
	return seqHeaderCopy
}

func (publisher *Publisher) SetAVCSequenceHeader(seqHeader []byte) {
	publisher.avcHeaderLock.Lock()
	publisher.AVCSequenceHeader = seqHeader
	publisher.avcHeaderLock.Unlock()
}

func (publisher *Publisher) GetAVCSequenceHeader() []byte {
	if publisher.AVCSequenceHeader == nil {
		return nil
	}
	publisher.avcHeaderLock.RLock()
	seqHeaderCopy := make([]byte, len(publisher.AVCSequenceHeader))
	copy(seqHeaderCopy, publisher.AVCSequenceHeader)
	publisher.avcHeaderLock.RUnlock()
	return seqHeaderCopy
}

func (publisher *Publisher) RemoveSubscriber(sessionId int) {
	publisher.subscriberLock.Lock()
	filtered := util.FilterSlice(publisher.Subscribers, func(s Subscriber) bool {
		return s.SessionId != sessionId
	})
	if len(filtered) == len(publisher.Subscribers) {
		logger.WarningLog.Printf("Subscriber with id = %d was not subcribed to publisher with id = %d", sessionId, publisher.SessionId)
	}
	publisher.Subscribers = filtered
	publisher.subscriberLock.Unlock()
}
