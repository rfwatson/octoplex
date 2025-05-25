package server

import (
	"testing"

	"git.netflux.io/rob/octoplex/internal/domain"
	"git.netflux.io/rob/octoplex/internal/store"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestResolveDestinations(t *testing.T) {
	var (
		id1 = uuid.MustParse("4c702e9c-4106-4cd9-b939-019cde075f72")
		id2 = uuid.MustParse("1f1054b7-4fbe-410c-838b-08f4fc4608c5")
		id3 = uuid.MustParse("185f9c17-e0b7-4d3a-82c6-bdd414724214")
		id4 = uuid.MustParse("d79ead17-91e6-4493-99a8-fc05ba09fbe0")
	)

	testCases := []struct {
		name     string
		in       []store.Destination
		existing []domain.Destination
		want     []domain.Destination
	}{
		{
			name:     "nil slices",
			existing: nil,
			want:     nil,
		},
		{
			name:     "empty slices",
			existing: []domain.Destination{},
			want:     []domain.Destination{},
		},
		{
			name:     "identical slices",
			in:       []store.Destination{{ID: id1}, {ID: id2}, {ID: id3}, {ID: id4}},
			existing: []domain.Destination{{ID: id1}, {ID: id2}, {ID: id3}, {ID: id4}},
			want:     []domain.Destination{{ID: id1}, {ID: id2}, {ID: id3}, {ID: id4}},
		},
		{
			name:     "adding a new destination",
			in:       []store.Destination{{ID: id1}, {ID: id2}},
			existing: []domain.Destination{{ID: id1}},
			want:     []domain.Destination{{ID: id1}, {ID: id2}},
		},
		{
			name:     "removing a destination",
			in:       []store.Destination{{ID: id2}},
			existing: []domain.Destination{{ID: id1}, {ID: id2}},
			want:     []domain.Destination{{ID: id2}},
		},
		{
			name:     "switching order, two items",
			in:       []store.Destination{{ID: id2}, {ID: id1}},
			existing: []domain.Destination{{ID: id1}, {ID: id2}},
			want:     []domain.Destination{{ID: id2}, {ID: id1}},
		},
		{
			name:     "switching order, several items",
			in:       []store.Destination{{ID: id2}, {ID: id1}, {ID: id3}, {ID: id4}},
			existing: []domain.Destination{{ID: id1}, {ID: id2}, {ID: id4}, {ID: id3}},
			want:     []domain.Destination{{ID: id2}, {ID: id1}, {ID: id3}, {ID: id4}},
		},
		{
			name:     "removing all destinations",
			in:       []store.Destination{},
			existing: []domain.Destination{{ID: id1}, {ID: id2}, {ID: id3}, {ID: id4}},
			want:     []domain.Destination{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, resolveDestinations(tc.existing, tc.in))
		})
	}
}
