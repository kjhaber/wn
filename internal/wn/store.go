package wn

// Store abstracts persistence for work items.
type Store interface {
	List() ([]*Item, error)
	Get(id string) (*Item, error)
	Put(item *Item) error
	UpdateItem(id string, fn func(*Item) (*Item, error)) error
	Delete(id string) error
	Root() string
}
