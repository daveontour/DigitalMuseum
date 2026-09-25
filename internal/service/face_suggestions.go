package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"runtime"
	"sync"

	"github.com/daveontour/aimuseum/internal/model"
)

// ErrSuggestionRefreshCancelled is returned by RefreshClusterSuggestions when
// cancelled() reports true before the computation finishes; nothing is written.
var ErrSuggestionRefreshCancelled = errors.New("possible-match refresh cancelled")

type namedFaceEmbedding struct {
	contactID int64
	vec       []float32
}

// nearestNamedContact returns the contact of the named face closest to query
// by L2 distance (the same metric vec0 reports), if any lies within
// maxDistance. The squared partial sum is checked every 64 dimensions so a
// face already farther than the best so far (or the threshold) is abandoned
// early — most named faces are nowhere near a given query.
func nearestNamedContact(query []float32, named []namedFaceEmbedding, maxDistance float64) (int64, float64, bool) {
	bestSq := maxDistance * maxDistance
	var bestContact int64
	found := false
	for _, n := range named {
		if len(n.vec) != len(query) {
			continue
		}
		var sum float64
		abandoned := false
		for i, q := range query {
			d := float64(q - n.vec[i])
			sum += d * d
			if i&63 == 63 && sum > bestSq {
				abandoned = true
				break
			}
		}
		if abandoned || sum > bestSq {
			continue
		}
		bestSq = sum
		bestContact = n.contactID
		found = true
	}
	return bestContact, math.Sqrt(bestSq), found
}

// ComputeClusterSuggestions finds, for every unnamed cluster, the closest
// already-named face to its representative face and keeps it as a "possible
// match" when within faceSameClusterL2Sq — the same "close enough to be the
// same person" threshold clustering uses. It's a brute-force comparison in
// memory against named faces only, not a vec0 KNN per cluster: on a real
// archive (tens of thousands of embeddings, ~12k unnamed clusters) one KNN
// per cluster took ~160 ms each, i.e. over half an hour, while this is a
// single table scan plus parallel arithmetic. Read-only.
func (s *FaceService) ComputeClusterSuggestions(ctx context.Context, cancelled func() bool, progress func(done, total int)) ([]model.ClusterSuggestion, error) {
	if s.embed == nil {
		return nil, fmt.Errorf("compute cluster suggestions: embedding helper not configured")
	}
	namedFaces, err := s.repo.ListNamedFaceContacts(ctx)
	if err != nil {
		return nil, err
	}
	clusters, _, err := s.repo.ListClusters(ctx, false, true, false, 0, 0, "", 0, 0)
	if err != nil {
		return nil, fmt.Errorf("compute cluster suggestions: list unnamed clusters: %w", err)
	}

	wanted := make(map[int64]struct{}, len(namedFaces)+len(clusters))
	for faceID := range namedFaces {
		wanted[faceID] = struct{}{}
	}
	for _, c := range clusters {
		if c.RepresentativeFaceID != nil {
			wanted[*c.RepresentativeFaceID] = struct{}{}
		}
	}
	vecs, err := s.embed.LoadEmbeddings(ctx, wanted)
	if err != nil {
		return nil, err
	}

	named := make([]namedFaceEmbedding, 0, len(namedFaces))
	for faceID, contactID := range namedFaces {
		if v, ok := vecs[faceID]; ok {
			named = append(named, namedFaceEmbedding{contactID: contactID, vec: v})
		}
	}

	total := len(clusters)
	results := make([]*model.ClusterSuggestion, total)
	jobs := make(chan int)
	var done int
	var mu sync.Mutex
	var wg sync.WaitGroup
	for w := 0; w < runtime.NumCPU(); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				c := clusters[i]
				if c.RepresentativeFaceID != nil {
					if q, ok := vecs[*c.RepresentativeFaceID]; ok {
						if contactID, dist, ok := nearestNamedContact(q, named, faceSameClusterL2Sq); ok {
							results[i] = &model.ClusterSuggestion{ClusterID: c.ID, ContactID: contactID, Distance: dist}
						}
					}
				}
				mu.Lock()
				done++
				if progress != nil && (done%200 == 0 || done == total) {
					progress(done, total)
				}
				mu.Unlock()
			}
		}()
	}
	cancelledEarly := false
	for i := range clusters {
		if cancelled != nil && i%200 == 0 && cancelled() {
			cancelledEarly = true
			break
		}
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	if cancelledEarly {
		return nil, ErrSuggestionRefreshCancelled
	}

	out := make([]model.ClusterSuggestion, 0)
	for _, r := range results {
		if r != nil {
			out = append(out, *r)
		}
	}
	return out, nil
}

// RefreshClusterSuggestions recomputes every unnamed cluster's "possible
// match" (see ComputeClusterSuggestions) and replaces the stored ones, which
// is what the People in Photos "Possible Matches" filter reads. Guesses only
// change when people get named (or faces get clustered), so this runs as a
// background job rather than per request. Returns how many clusters now have
// a suggestion.
func (s *FaceService) RefreshClusterSuggestions(ctx context.Context, cancelled func() bool, progress func(done, total int)) (int, error) {
	suggestions, err := s.ComputeClusterSuggestions(ctx, cancelled, progress)
	if err != nil {
		return 0, err
	}
	if err := s.repo.ReplaceClusterSuggestions(ctx, suggestions); err != nil {
		return 0, err
	}
	return len(suggestions), nil
}
