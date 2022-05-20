package dataloader

import (
	"sync"
	"time"
)

type Config[In comparable, Out any] struct {
	// Fetch is a method that provides the data for the loader
	Fetch func(keys []In) ([]Out, []error)

	// Wait is how long wait before sending a batch
	Wait time.Duration

	// MaxBatch will limit the maximum number of keys to send in one batch, 0 = not limit
	MaxBatch int
}

func New[In comparable, Out any](config Config[In, Out]) DataLoader[In, Out] {
	return &DataLoaderParent[In, Out]{
		fetch:    config.Fetch,
		wait:     config.Wait,
		maxBatch: config.MaxBatch,
	}
}

type DataLoaderParent[In comparable, Out any] struct {
	// this method provides the data for the loader
	fetch func(keys []In) ([]Out, []error)

	// how long to done before sending a batch
	wait time.Duration

	// this will limit the maximum number of keys to send in one batch, 0 = no limit
	maxBatch int

	// INTERNAL

	// the current batch. keys will continue to be collected until timeout is hit,
	// then everything will be sent to the fetch method and out to the listeners
	batch *dataloaderBatch[In, Out]

	// mutex to prevent races
	mu sync.Mutex
}

type dataloaderBatch[In comparable, Out any] struct {
	keys    []In
	data    []Out
	error   []error
	closing bool
	done    chan struct{}
}

// Load a string by key, batching and caching will be applied automatically
func (l *DataLoaderParent[In, Out]) Load(key In) (Out, error) {
	return l.LoadThunk(key)()
}

// LoadThunk returns a function that when called will block waiting for a string.
// This method should be used if you want one goroutine to make requests to many
// different data loaders without blocking until the thunk is called.
func (l *DataLoaderParent[In, Out]) LoadThunk(key In) func() (Out, error) {
	l.mu.Lock()
	if l.batch == nil {
		l.batch = &dataloaderBatch[In, Out]{done: make(chan struct{})}
	}
	batch := l.batch
	pos := batch.keyIndex(l, key)
	l.mu.Unlock()

	return func() (Out, error) {
		<-batch.done

		var data Out
		if pos < len(batch.data) {
			data = batch.data[pos]
		}

		var err error
		// its convenient to be able to return a single error for everything
		if len(batch.error) == 1 {
			err = batch.error[0]
		} else if batch.error != nil {
			err = batch.error[pos]
		}

		return data, err
	}
}

// LoadAll fetches many keys at once. It will be broken into appropriate sized
// sub batches depending on how the loader is configured
func (l *DataLoaderParent[In, Out]) LoadAll(keys []In) ([]Out, []error) {
	results := make([]func() (Out, error), len(keys))

	for i, key := range keys {
		results[i] = l.LoadThunk(key)
	}

	outs := make([]Out, len(keys))
	errors := make([]error, len(keys))
	for i, thunk := range results {
		outs[i], errors[i] = thunk()
	}
	return outs, errors
}

// LoadAllThunk returns a function that when called will block waiting for a strings.
// This method should be used if you want one goroutine to make requests to many
// different data loaders without blocking until the thunk is called.
func (l *DataLoaderParent[In, Out]) LoadAllThunk(keys []In) func() ([]Out, []error) {
	results := make([]func() (Out, error), len(keys))
	for i, key := range keys {
		results[i] = l.LoadThunk(key)
	}
	return func() ([]Out, []error) {
		outs := make([]Out, len(keys))
		errors := make([]error, len(keys))
		for i, thunk := range results {
			outs[i], errors[i] = thunk()
		}
		return outs, errors
	}
}

// keyIndex will return the location of the key in the batch, if its not found
// it will add the key to the batch
func (b *dataloaderBatch[In, Out]) keyIndex(l *DataLoaderParent[In, Out], key In) int {
	for i, existingKey := range b.keys {
		if key == existingKey {
			return i
		}
	}

	pos := len(b.keys)
	b.keys = append(b.keys, key)
	if pos == 0 {
		go b.startTimer(l)
	}

	if l.maxBatch != 0 && pos >= l.maxBatch-1 {
		if !b.closing {
			b.closing = true
			l.batch = nil
			go b.end(l)
		}
	}

	return pos
}

func (b *dataloaderBatch[In, Out]) startTimer(l *DataLoaderParent[In, Out]) {
	time.Sleep(l.wait)
	l.mu.Lock()

	// we must have hit a batch limit and are already finalizing this batch
	if b.closing {
		l.mu.Unlock()
		return
	}

	l.batch = nil
	l.mu.Unlock()

	b.end(l)
}

func (b *dataloaderBatch[In, Out]) end(l *DataLoaderParent[In, Out]) {
	b.data, b.error = l.fetch(b.keys)
	close(b.done)
}
