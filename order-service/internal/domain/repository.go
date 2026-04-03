package domain

type OrderRepository interface {
	Save(order *Order) error
	FindByID(id string) (*Order, error)
	Update(order *Order) error
	FindByIdempotencyKey(key string) (*Order, error)
	FindRecent(limit int) ([]*Order, error)
}
