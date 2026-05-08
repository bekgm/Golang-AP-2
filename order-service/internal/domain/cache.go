package domain

// OrderCache defines the caching contract for orders.
// Implementations must not be referenced directly by use cases.
type OrderCache interface {
	Get(id string) (*Order, error)
	Set(order *Order) error
	Delete(id string) error
}
