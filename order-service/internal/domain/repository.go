package domain

// OrderRepository is the port (interface) that the use case layer
// depends on. The repository layer implements this interface.
// This enforces Dependency Inversion: use cases depend on abstractions,
// not concrete implementations.
type OrderRepository interface {
	Save(order *Order) error
	FindByID(id string) (*Order, error)
	Update(order *Order) error
	FindByIdempotencyKey(key string) (*Order, error)
}
