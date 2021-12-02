package store

import (
	"context"
	"sync"
	"time"
)

type Pubsub struct {
	mutex       sync.RWMutex
	subscribers []*subscriber
	closed      bool
}

func (p *Pubsub) Subscribe(ctx context.Context) chan WatchEvent {
	ch := make(chan WatchEvent)
	s := &subscriber{ch: ch}
	p.addSubscriber(s)
	go func() {
		<-ctx.Done()
		p.removeSubscriber(s)
	}()
	return ch
}

func (p *Pubsub) Publish(evt WatchEvent) {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	if p.closed || len(p.subscribers) == 0 {
		return
	}

	newSubscribers := make([]*subscriber, 0, len(p.subscribers))
	ch := make(chan int, len(p.subscribers))
	for i, subscriber := range p.subscribers {
		// Notify each subscriber in a separate goroutine to prevent hanging subscribers
		// from blocking others longer than the message timeout of a single subscriber.
		go subscriber.Accept(evt, i, ch)
	}
	i := 0
	for c := range ch {
		if c >= 0 {
			// Only keep subscribers that consume events and kick those that don't
			newSubscribers = append(newSubscribers, p.subscribers[c])
		}
		i++
		if i == len(p.subscribers) {
			close(ch)
		}
	}
	p.subscribers = newSubscribers
}

func (p *Pubsub) Close() {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if !p.closed {
		p.closed = true
		for _, subs := range p.subscribers {
			subs.Close()
		}
	}
}

func (p *Pubsub) addSubscriber(s *subscriber) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.subscribers = append(p.subscribers, s)
}

func (p *Pubsub) removeSubscriber(s *subscriber) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	subscribers := make([]*subscriber, 0, len(p.subscribers))
	for _, subs := range p.subscribers {
		if subs == s {
			subs.Close()
		} else {
			subscribers = append(subscribers, subs)
		}
	}
	p.subscribers = subscribers
}

type subscriber struct {
	ch    chan WatchEvent
	mutex sync.Mutex
}

// Accept sends an event to the subscriber and writes the provided index into the provided channel afterwards.
// In case the event was not accepted from the other end of the subscriber's channel the value -1 is written to the idx channel to mark it as unresponsive in order to indicate to the caller that the subscriber should be removed.
func (s *subscriber) Accept(evt WatchEvent, subscriberIdx int, idxCh chan<- int) {
	// Synchronize this operation since a subscriber could stop watching while a new event is emitted
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.ch != nil {
		select {
		case s.ch <- evt:
			idxCh <- subscriberIdx
			return
		case <-time.After(5 * time.Second):
		}
	}
	s.Close()
	idxCh <- -1
}

// Close the subscriber's channel.
func (s *subscriber) Close() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	close(s.ch)
	s.ch = nil
}
