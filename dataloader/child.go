package dataloader

import (
	"github.com/SevenTV/Common/sync_map"
)

type DataLoaderChild[In comparable, Out any] struct {
	parent DataLoader[In, Out]
	cache  *sync_map.Map[In, Out]
}

func NewChild[In comparable, Out any](parent DataLoader[In, Out]) DataLoader[In, Out] {
	return &DataLoaderChild[In, Out]{
		parent: parent,
	}
}

func (d *DataLoaderChild[In, Out]) Load(key In) (Out, error) {
	if v, ok := d.cache.Load(key); ok {
		return v, nil
	}

	out, err := d.parent.Load(key)
	if err != nil {
		return out, err
	}

	d.cache.Store(key, out)
	return out, nil
}

func (d *DataLoaderChild[In, Out]) LoadThunk(key In) func() (Out, error) {
	return func() (Out, error) {
		return d.Load(key)
	}
}

func (d *DataLoaderChild[In, Out]) LoadAll(keys []In) ([]Out, []error) {
	result := make([]Out, len(keys))
	resultErrs := make([]error, len(keys))

	keyMp := map[int]int{}
	idx := 0
	newKeys := []In{}
	for i, key := range keys {
		if v, ok := d.cache.Load(key); ok {
			result[i] = v
		} else {
			keyMp[idx] = i
			newKeys = append(newKeys, key)
			idx++
		}
	}
	if len(keys) != 0 {
		out, errs := d.parent.LoadAll(newKeys)
		for idx, v := range out {
			result[keyMp[idx]] = v
			resultErrs[keyMp[idx]] = errs[idx]
			if errs[idx] == nil {
				d.cache.Store(keys[idx], v)
			}
		}
	}

	return result, nil
}

func (d *DataLoaderChild[In, Out]) LoadAllThunk(keys []In) func() ([]Out, []error) {
	return func() ([]Out, []error) {
		return d.LoadAll(keys)
	}
}
