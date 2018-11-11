package watchlist

import (
	"sync"

	"k8s.io/apimachinery/pkg/types"
)

type WatchList struct {
	sync.RWMutex
	watched map[string][]types.NamespacedName
}

type Watched interface {
	Add(source, target types.NamespacedName)
	Get(source types.NamespacedName) []types.NamespacedName
	Remove(target types.NamespacedName)
}

func New() *WatchList {
	return &WatchList{
		watched: make(map[string][]types.NamespacedName, 0),
	}
}

func (w *WatchList) Add(source, target types.NamespacedName) {
	w.Lock()
	defer w.Unlock()
	if _, ok := w.watched[source.String()]; !ok {
		w.watched[source.String()] = make([]types.NamespacedName, 0)
	}

	for _, v := range w.watched[source.String()] {
		if v.String() == target.String() {
			return
		}
	}
	w.watched[source.String()] = append(w.watched[source.String()], target)
}

func (w *WatchList) Get(source types.NamespacedName) []types.NamespacedName {
	w.RLock()
	defer w.RUnlock()
	return w.watched[source.String()]
}

func (w *WatchList) Remove(target types.NamespacedName) {
	w.Lock()
	defer w.Unlock()

	for source, stargets := range w.watched {
		var new []types.NamespacedName
		for _, v := range stargets {
			if v.String() == target.String() {
				continue
			}
			new = append(new, v)
		}
		w.watched[source] = new

		if len(w.watched[source]) == 0 {
			delete(w.watched, source)
		}
	}
}
