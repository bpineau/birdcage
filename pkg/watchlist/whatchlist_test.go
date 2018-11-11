package watchlist

import (
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/types"
)

type nPair struct {
	src types.NamespacedName
	tgt types.NamespacedName
}

var (
	src1 = types.NamespacedName{Namespace: "foo", Name: "src1"}
	src2 = types.NamespacedName{Namespace: "foo", Name: "src2"}
	tgt1 = types.NamespacedName{Namespace: "foo", Name: "tgt1"}
	tgt2 = types.NamespacedName{Namespace: "foo", Name: "tgt2"}

	rTests = []struct {
		title  string
		add    []nPair
		remove []types.NamespacedName
		get    types.NamespacedName
		expect []types.NamespacedName
	}{

		{
			title:  "Add one entry",
			add:    []nPair{nPair{src: src1, tgt: tgt1}},
			get:    src1,
			expect: []types.NamespacedName{tgt1},
		},

		{
			title:  "Add several entries, same source",
			add:    []nPair{nPair{src: src1, tgt: tgt1}, nPair{src: src1, tgt: tgt2}},
			get:    src1,
			expect: []types.NamespacedName{tgt1, tgt2},
		},

		{
			title:  "Add several entries, same target",
			add:    []nPair{nPair{src: src1, tgt: tgt1}, nPair{src: src2, tgt: tgt1}},
			get:    src2,
			expect: []types.NamespacedName{tgt1},
		},

		{
			title: "Remove a target",
			add: []nPair{
				nPair{src: src1, tgt: tgt1},
				nPair{src: src1, tgt: tgt2},
				nPair{src: src2, tgt: tgt1},
			},
			get:    src1,
			remove: []types.NamespacedName{tgt1},
			expect: []types.NamespacedName{tgt2},
		},

		{
			title: "Remove a target, list empty",
			add: []nPair{
				nPair{src: src1, tgt: tgt1},
				nPair{src: src2, tgt: tgt1},
			},
			get:    src1,
			remove: []types.NamespacedName{tgt1},
			expect: nil,
		},

		{
			title:  "Get non existent source",
			add:    []nPair{nPair{src: src1, tgt: tgt1}},
			get:    src2,
			expect: nil,
		},

		{
			title:  "Remove non existent target",
			add:    []nPair{nPair{src: src1, tgt: tgt1}},
			get:    src1,
			remove: []types.NamespacedName{tgt2},
			expect: []types.NamespacedName{tgt1},
		},

		{
			title:  "Ensure we don't store duplicates",
			add:    []nPair{nPair{src: src1, tgt: tgt1}, nPair{src: src1, tgt: tgt1}},
			get:    src1,
			expect: []types.NamespacedName{tgt1},
		},
	}
)

func TestWatchList(t *testing.T) {
	for _, tt := range rTests {
		lst := New()
		for _, pair := range tt.add {
			lst.Add(pair.src, pair.tgt)
		}
		for _, tgt := range tt.remove {
			lst.Remove(tgt)
		}
		res := lst.Get(tt.get)
		if !reflect.DeepEqual(res, tt.expect) {
			t.Errorf("%s failed; expected %v got %v", tt.title, tt.expect, res)
		}
	}
}
