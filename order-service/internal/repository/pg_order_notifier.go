package repository

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/lib/pq"
)

type PGOrderNotifier struct {
	listener    *pq.Listener
	mu          sync.Mutex
	subscribers map[string][]chan string
}

func NewPGOrderNotifier(dsn string) (*PGOrderNotifier, error) {
	n := &PGOrderNotifier{
		subscribers: make(map[string][]chan string),
	}

	listener := pq.NewListener(
		dsn,
		10*time.Second,
		time.Minute,
		func(ev pq.ListenerEventType, err error) {
			if err != nil {
				log.Printf("[pg-notifier] listener event error: %v", err)
			}
		},
	)

	if err := listener.Listen("order_status_updates"); err != nil {
		return nil, fmt.Errorf("pg notifier: listen: %w", err)
	}

	n.listener = listener
	go n.dispatch()
	return n, nil
}

func (n *PGOrderNotifier) dispatch() {
	for notif := range n.listener.Notify {
		if notif == nil {
			continue
		}

		parts := strings.SplitN(notif.Extra, ":", 2)
		if len(parts) != 2 {
			continue
		}
		orderID, newStatus := parts[0], parts[1]

		n.mu.Lock()
		for _, ch := range n.subscribers[orderID] {
			select {
			case ch <- newStatus:
			default:
			}
		}
		n.mu.Unlock()
	}
}

func (n *PGOrderNotifier) Subscribe(ctx context.Context, orderID string) (<-chan string, error) {
	ch := make(chan string, 16)

	n.mu.Lock()
	n.subscribers[orderID] = append(n.subscribers[orderID], ch)
	n.mu.Unlock()

	go func() {
		<-ctx.Done()
		n.mu.Lock()
		subs := n.subscribers[orderID]
		for i, s := range subs {
			if s == ch {
				n.subscribers[orderID] = append(subs[:i], subs[i+1:]...)
				break
			}
		}
		close(ch)
		n.mu.Unlock()
	}()

	return ch, nil
}
