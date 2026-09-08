package storage

import (
	"context"
	"reflect"
	"sort"
	"testing"
	"time"

	"go.ripls.org/ripls/server/ai/embedding"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestParseVector(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []float32
	}{
		{"simple", "[1,2.5,3]", []float32{1, 2.5, 3}},
		{"spaces", " [0.1, 0.2 , 0.3] ", []float32{0.1, 0.2, 0.3}},
		{"negative", "[-1,0,1]", []float32{-1, 0, 1}},
		{"empty brackets", "[]", nil},
		{"empty string", "", nil},
		{"malformed", "[1,foo,3]", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseVector(c.in)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("parseVector(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

func TestClusterByCosine(t *testing.T) {
	// Three vectors: a1/a2 are near-parallel (same "kind"); b is orthogonal.
	vecs := map[string][]float32{
		"a1": {1, 0, 0},
		"a2": {0.98, 0.2, 0},
		"b":  {0, 0, 1},
	}

	t.Run("merges similar, splits dissimilar", func(t *testing.T) {
		groups := clusterByCosine([]string{"a1", "a2", "b"}, vecs, 0.6)
		if got := normalizeGroups(groups); !reflect.DeepEqual(got, [][]string{{"a1", "a2"}, {"b"}}) {
			t.Errorf("groups = %v, want [[a1 a2] [b]]", got)
		}
	})

	t.Run("threshold 1.0 recovers exact-only grouping", func(t *testing.T) {
		groups := clusterByCosine([]string{"a1", "a2", "b"}, vecs, 1.0)
		if len(groups) != 3 {
			t.Errorf("want 3 singletons at threshold 1.0, got %v", normalizeGroups(groups))
		}
	})

	t.Run("missing vector is a singleton and absorbs nothing", func(t *testing.T) {
		withMissing := map[string][]float32{"a1": {1, 0, 0}, "a2": {1, 0, 0}}
		groups := clusterByCosine([]string{"a1", "a2", "gap"}, withMissing, 0.6)
		got := normalizeGroups(groups)
		want := [][]string{{"a1", "a2"}, {"gap"}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("groups = %v, want %v", got, want)
		}
	})

	t.Run("deterministic regardless of input order", func(t *testing.T) {
		g1 := normalizeGroups(clusterByCosine([]string{"b", "a2", "a1"}, vecs, 0.6))
		g2 := normalizeGroups(clusterByCosine([]string{"a1", "b", "a2"}, vecs, 0.6))
		if !reflect.DeepEqual(g1, g2) {
			t.Errorf("nondeterministic: %v vs %v", g1, g2)
		}
	})
}

// TestGroupIDsByStoredEmbedding_Integration exercises the variant-column read +
// clustering path end-to-end against pgvector with hand-crafted 3-dim vectors
// (no model needed), stored on a non-default embedding variant.
func TestGroupIDsByStoredEmbedding_Integration(t *testing.T) {
	store, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	info := &embedding.Info{Vendor: "test", Model: "grp-model", Dimensions: 3}
	const variant = "name"

	// Create the name-variant column (no index needed for by-id reads).
	if err := store.ensureEmbeddingColumn(ctx, "experience", info, variant, false); err != nil {
		t.Fatalf("ensureEmbeddingColumn: %v", err)
	}

	insert := func(id, name string) {
		if _, err := store.Insert(ctx, &models.Experience{Id: id, Name: name}); err != nil {
			t.Fatalf("Insert(%s): %v", id, err)
		}
	}
	insert("hike-1", "Flatirons hike")
	insert("hike-2", "Sunday hike")
	insert("dinner", "Taco night")
	insert("noemb", "Just created")

	set := func(id string, v []float32) {
		if err := store.updateEmbedding(ctx, "experience", id, info, variant, v); err != nil {
			t.Fatalf("updateEmbedding(%s): %v", id, err)
		}
	}
	set("hike-1", []float32{1, 0, 0})
	set("hike-2", []float32{0.97, 0.24, 0})
	set("dinner", []float32{0, 0, 1})
	// noemb intentionally left without an embedding on the variant column.

	ids := []string{"hike-1", "hike-2", "dinner", "noemb"}
	groups, err := store.GroupIDsByStoredEmbedding(ctx, &models.Experience{}, info, variant, ids, 0.6)
	if err != nil {
		t.Fatalf("GroupIDsByStoredEmbedding: %v", err)
	}

	got := normalizeGroups(groups)
	want := [][]string{{"dinner"}, {"hike-1", "hike-2"}, {"noemb"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("groups = %v, want %v", got, want)
	}
}

// TestExperienceNameVariant_EndToEnd verifies the #2694 wiring with the real
// model: configuring the embedder creates both the default and the name-only
// columns on the experience table, inserting an experience precomputes both, and
// the stored name-only vectors cluster near-identical activity names together.
func TestExperienceNameVariant_EndToEnd(t *testing.T) {
	store, cleanup := SetupTestStorage(t)
	defer cleanup()
	embedder := SetupTestEmbedder(t)
	if embedder == nil {
		return
	}
	ctx := context.Background()

	done := make(chan error, 10)
	store.SetEmbeddingDoneChannel(done)
	if err := store.SetEmbedder(ctx, embedder); err != nil {
		t.Fatalf("SetEmbedder: %v", err)
	}

	info := embedder.Info()
	cols, err := store.ListEmbeddingColumns(ctx, "experience")
	if err != nil {
		t.Fatalf("ListEmbeddingColumns: %v", err)
	}
	wantDefault := EmbeddingColumnName(info)
	wantName := embeddingColumnName(info, ExperienceNameEmbeddingVariant)
	if !containsStr(cols, wantDefault) || !containsStr(cols, wantName) {
		t.Fatalf("experience embedding columns = %v, want both %q and %q", cols, wantDefault, wantName)
	}

	insert := func(id, name string) {
		if _, err := store.Insert(ctx, &models.Experience{Id: id, Name: name, OwnerId: "u"}); err != nil {
			t.Fatalf("Insert(%s): %v", id, err)
		}
		// One signal per row, emitted after both variants are stored.
		select {
		case embErr := <-done:
			if embErr != nil {
				t.Fatalf("embedding generation failed for %s: %v", id, embErr)
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("timed out waiting for embeddings for %s", id)
		}
	}
	insert("a1", "Flatirons hike")
	insert("a2", "Flatirons hike with the group")
	insert("b1", "Taco Tuesday dinner")

	// Both variant columns should be populated for every row.
	_, withName, err := store.countWithEmbedding(ctx, "experience", info, ExperienceNameEmbeddingVariant)
	if err != nil {
		t.Fatalf("countWithEmbedding(name): %v", err)
	}
	if withName != 3 {
		t.Errorf("name-variant embeddings populated = %d, want 3", withName)
	}

	// The two hikes cluster; the dinner stays separate.
	groups, err := store.GroupIDsByStoredEmbedding(
		ctx, &models.Experience{}, info, ExperienceNameEmbeddingVariant,
		[]string{"a1", "a2", "b1"}, 0.7,
	)
	if err != nil {
		t.Fatalf("GroupIDsByStoredEmbedding: %v", err)
	}
	if got := normalizeGroups(groups); !reflect.DeepEqual(got, [][]string{{"a1", "a2"}, {"b1"}}) {
		t.Errorf("groups = %v, want [[a1 a2] [b1]]", got)
	}
}

func containsStr(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

// normalizeGroups sorts within and across groups so assertions are order-stable.
func normalizeGroups(groups [][]string) [][]string {
	out := make([][]string, len(groups))
	for i, g := range groups {
		cp := append([]string(nil), g...)
		sort.Strings(cp)
		out[i] = cp
	}
	sort.Slice(out, func(i, j int) bool {
		if len(out[i]) == 0 || len(out[j]) == 0 {
			return len(out[i]) < len(out[j])
		}
		return out[i][0] < out[j][0]
	})
	return out
}
