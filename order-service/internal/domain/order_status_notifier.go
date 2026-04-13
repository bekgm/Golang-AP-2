package domain

import "context"

type OrderStatusNotifier interface {
	Subscribe(ctx context.Context, orderID string) (<-chan string, error)
}
