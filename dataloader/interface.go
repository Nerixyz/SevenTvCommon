package dataloader

type DataLoader[In comparable, Out any] interface {
	Load(key In) (Out, error)
	LoadThunk(key In) func() (Out, error)
	LoadAll(keys []In) ([]Out, []error)
	LoadAllThunk(keys []In) func() ([]Out, []error)
}
