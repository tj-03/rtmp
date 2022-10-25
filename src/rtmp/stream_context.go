package rtmp

import (
	"sync"

	"github.com/tj03/rtmp/src/logger"
	rtmpMsg "github.com/tj03/rtmp/src/messaging"
	"github.com/tj03/rtmp/src/util"
)

//Holds data about the publisher and associated subscribers and provides methods to read/write via RWMutex.
type Publisher struct {
	subscriberLock sync.RWMutex
	aacHeaderLock  sync.RWMutex
	avcHeaderLock  sync.RWMutex
	metadataLock   sync.RWMutex
	SessionId      int
	Subscribers    []Subscriber
	//AMF0 Encoded Video MetaData
	metadata []byte
	//AAC and AVC sequence headers MUST be sent to subscriber before sending any video/audio data
	aacSequenceHeader []byte
	avcSequenceHeader []byte
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

//Creates a publisher. Returns false if unsuccesful(streamname already exists) , true otherwise.
func (ctx *Context) CreateStream(sessionId int, streamName string, metaData []byte) bool {
	ctx.publishersLock.Lock()
	ctx.clientStreamsLock.Lock()
	ctx.waitListLock.Lock()
	defer ctx.publishersLock.Unlock()
	defer ctx.clientStreamsLock.Unlock()
	defer ctx.waitListLock.Unlock()
	if publisher := ctx.publishers[streamName]; publisher != nil {
		return false
	}
	ctx.clientStreams[sessionId] = streamName
	waitList := ctx.waitLists[streamName]
	ctx.waitLists[streamName] = nil
	if waitList == nil {
		waitList = []Subscriber{}
	}
	ctx.publishers[streamName] = &Publisher{
		SessionId:   sessionId,
		Subscribers: waitList,
		metadata:    metaData}

	return true
}

func (ctx *Context) GetPublisher(streamName string) *Publisher {
	ctx.publishersLock.RLock()
	publisher := ctx.publishers[streamName]
	ctx.publishersLock.RUnlock()
	return publisher
}

func (ctx *Context) RemovePublisher(sessionId int) {
	ctx.publishersLock.Lock()
	ctx.clientStreamsLock.Lock()
	defer ctx.clientStreamsLock.Unlock()
	defer ctx.publishersLock.Unlock()
	streamName, ok := ctx.clientStreams[sessionId]
	if !ok {
		return
	}
	publisher := ctx.publishers[streamName]
	if publisher == nil {
		panic("Stream name set but clientstream not set")
	}
	publisher.BroadcastMessage(rtmpMsg.NewStatusMessage("status", "NetStream.Play.UnpublishNotify", "Stream unpublished"))

	delete(ctx.publishers, streamName)
	delete(ctx.clientStreams, sessionId)
}

func (ctx *Context) GetStreamName(sessionId int) (string, bool) {
	ctx.clientStreamsLock.RLock()
	streamName, ok := ctx.clientStreams[sessionId]
	ctx.clientStreamsLock.RUnlock()
	return streamName, ok
}

func (ctx *Context) AppendToWaitlist(streamName string, subscriber Subscriber) {
	ctx.waitListLock.Lock()
	defer ctx.waitListLock.Unlock()
	waitList := ctx.waitLists[streamName]
	if waitList == nil {
		waitList = make([]Subscriber, 0)
	}
	waitList = append(waitList, subscriber)
	ctx.waitLists[streamName] = waitList
}

func (ctx *Context) RemoveFromWaitlist(sessionId int, streamName string) {
	ctx.waitListLock.Lock()
	defer ctx.waitListLock.Unlock()
	waitList := ctx.waitLists[streamName]
	filtered := util.FilterSlice(waitList, func(s Subscriber) bool {
		return s.SessionId == sessionId
	})
	if len(filtered) == len(waitList) {
		logger.WarningLog.Printf("Subscriber with id = %d was not in waitlist", sessionId)
	}
	ctx.waitLists[streamName] = filtered

}

func (publisher *Publisher) AddSubscriber(channel chan MessageResult, sessionId int) {
	publisher.subscriberLock.Lock()
	defer publisher.subscriberLock.Unlock()
	if channel == nil {
		panic("channel is nil")
	}
	publisher.Subscribers = append(publisher.Subscribers, Subscriber{StreamChannel: channel, SessionId: sessionId})
}

func (publisher *Publisher) GetSubscribers() []Subscriber {
	publisher.subscriberLock.RLock()
	defer publisher.subscriberLock.RUnlock()
	subs := make([]Subscriber, 0, len(publisher.Subscribers))
	copy(subs, publisher.Subscribers)
	return subs
}

func (publisher *Publisher) BroadcastMessage(msg rtmpMsg.Message) {
	publisher.subscriberLock.RLock()
	defer publisher.subscriberLock.RUnlock()
	for i := range publisher.Subscribers {
		select {
		case publisher.Subscribers[i].StreamChannel <- MessageResult{msg, nil}:

		default:
			//logger.WarningLog.Printf("Subscriber with id = %d could not process message", publisher.Subscribers[i].SessionId)
			//fmt.Printf("Subscriber with id = %d could not process message\n", publisher.Subscribers[i].SessionId)
		}
	}
}

func (publisher *Publisher) SetAACSequenceHeader(seqHeader []byte) {
	publisher.aacHeaderLock.Lock()
	defer publisher.aacHeaderLock.Unlock()
	publisher.aacSequenceHeader = seqHeader
}

func (publisher *Publisher) GetAACSequenceHeader() []byte {
	publisher.aacHeaderLock.RLock()
	defer publisher.aacHeaderLock.RUnlock()
	if publisher.aacSequenceHeader == nil {
		return nil
	}
	seqHeaderCopy := make([]byte, len(publisher.aacSequenceHeader))
	copy(seqHeaderCopy, publisher.aacSequenceHeader)
	return seqHeaderCopy
}

func (publisher *Publisher) SetMetadata(metadata []byte) {
	publisher.metadataLock.Lock()
	defer publisher.metadataLock.Unlock()
	publisher.metadata = metadata
}

func (publisher *Publisher) GetMetadata() []byte {
	publisher.metadataLock.RLock()
	defer publisher.metadataLock.RUnlock()
	if publisher.metadata == nil {
		return nil
	}
	metadataCopy := make([]byte, len(publisher.metadata))
	copy(metadataCopy, publisher.metadata)
	return metadataCopy
}

func (publisher *Publisher) SetAVCSequenceHeader(seqHeader []byte) {
	publisher.avcHeaderLock.Lock()
	publisher.avcSequenceHeader = seqHeader
	publisher.avcHeaderLock.Unlock()
}

func (publisher *Publisher) GetAVCSequenceHeader() []byte {
	publisher.avcHeaderLock.RLock()
	defer publisher.avcHeaderLock.RUnlock()
	if publisher.avcSequenceHeader == nil {
		return nil
	}
	seqHeaderCopy := make([]byte, len(publisher.avcSequenceHeader))
	copy(seqHeaderCopy, publisher.avcSequenceHeader)
	return seqHeaderCopy
}

func (publisher *Publisher) RemoveSubscriber(sessionId int) {
	publisher.subscriberLock.Lock()
	defer publisher.subscriberLock.Unlock()
	filtered := util.FilterSlice(publisher.Subscribers, func(s Subscriber) bool {
		return s.SessionId == sessionId
	})
	if len(filtered) == len(publisher.Subscribers) {
		logger.WarningLog.Printf("Subscriber with id = %d was not subcribed to publisher with id = %d", sessionId, publisher.SessionId)
	}
	publisher.Subscribers = filtered
}
