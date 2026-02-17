package wn

// Store abstracts persistence for work items.
type Store interface {
	List() ([]*Item, error)
	Get(id string) (*Item, error)
	Put(item *Item) error
	Delete(id string) error
	Root() string
}
