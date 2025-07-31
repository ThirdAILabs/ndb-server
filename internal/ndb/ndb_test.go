package ndb_test

import (
	"fmt"
	"ndb-server/internal/ndb"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func checkQuery(t *testing.T, ndb ndb.NeuralDB, query string, constraints ndb.Constraints, expectedIds []uint64) {
	results, err := ndb.Query(query, len(expectedIds), constraints)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}

	ids := []uint64{}
	for _, chunk := range results {
		ids = append(ids, chunk.Id)
	}

	if !slices.Equal(ids, expectedIds) {
		t.Fatalf("query '%v' failed: expected %v got %v", query, expectedIds, ids)
	}
}

func TestBasicRetrieval(t *testing.T) {
	db, err := ndb.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	err = db.Insert(
		"doc_1", "id_1",
		[]string{"a b c d e g", "a b c d", "1 2 3"},
		[]map[string]interface{}{{"q1": true}, {"q2": true}, {"q1": true}},
		nil)
	if err != nil {
		t.Fatal(err)
	}

	err = db.Insert(
		"doc_2", "id_2",
		[]string{"x y z", "2 3", "c f", "f g d g", "c d e f"},
		[]map[string]interface{}{{}, {"q2": true}, {"q2": true}, {"q2": true}, {"q1": true, "q2": true}},
		nil)
	if err != nil {
		t.Fatal(err)
	}

	err = db.Insert(
		"doc_3", "id_3",
		[]string{"f t q v w", "f m n o p", "f g h i", "c 7 8 9 10 11"},
		[]map[string]interface{}{{}, {}, {}, {"q1": true}},
		nil)
	if err != nil {
		t.Fatal(err)
	}

	checkQuery(t, db, "a & b c", nil, []uint64{1, 0, 5, 7})

	checkQuery(t, db, "a & b c", ndb.Constraints{"q1": ndb.EqualTo(true)}, []uint64{0, 7, 11})

	checkQuery(t, db, "f g", nil, []uint64{6, 10, 0, 5, 7})

	checkQuery(t, db, "f g", ndb.Constraints{"q2": ndb.EqualTo(true)}, []uint64{6, 5, 7})
}

func TestLessFrequentTokensScoreHigher(t *testing.T) {
	db, err := ndb.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	err = db.Insert(
		"doc", "id",
		[]string{"a b c d", "a c f d", "b f g k", "a d f h", "b e g e", "h j f e", "w k z m"},
		nil,
		nil)
	if err != nil {
		t.Fatal(err)
	}

	checkQuery(t, db, "a b h j", nil, []uint64{5, 3, 0})
}

func TestRepeatedTokensInDocs(t *testing.T) {
	db, err := ndb.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	err = db.Insert(
		"doc", "id",
		[]string{"c a z a", "y r q z", "e c c m", "l b f h", "a b q d"},
		nil,
		nil)
	if err != nil {
		t.Fatal(err)
	}

	checkQuery(t, db, "c a q", nil, []uint64{0, 4, 2})
}

func TestRepeatedTokensInQuery(t *testing.T) {
	db, err := ndb.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	err = db.Insert(
		"doc", "id",
		[]string{"y r q z", "c a z m", "e c c m", "a b q d", "l b f h q"},
		nil,
		nil)
	if err != nil {
		t.Fatal(err)
	}

	checkQuery(t, db, "q a q m", nil, []uint64{3, 1})
}

func TestShorterDocsScoreHigherWithSameTokens(t *testing.T) {
	db, err := ndb.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	err = db.Insert(
		"doc", "id",
		[]string{"x w z k", "e c a", "a b c d", "l b f h", "y r s"},
		nil,
		nil)
	if err != nil {
		t.Fatal(err)
	}

	checkQuery(t, db, "c a q", nil, []uint64{1, 2})
}

func TestConstrainedSearch(t *testing.T) {
	db, err := ndb.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	err = db.Insert(
		"doc", "id",
		[]string{"a", "a b", "a b c", "a b c d", "a b c d e", "w", "w x", "w x y",
			"w x y z"},
		[]map[string]interface{}{
			{"k1": 5.2, "k2": false, "k3": 7, "k4": "apple"},
			// elminated by constraint 2
			{"k1": 3.1, "k2": true, "k3": 7, "k4": "banana"},
			// elminated by constraint 3
			{"k1": 9.2, "k2": false, "k3": 11, "k4": "kiwi"},
			// elminated by constraint 1
			{"k1": 2.9, "k2": false, "k3": 7, "k4": "grape"},
			// elminated by constraint 4
			{"k1": 4.7, "k2": false, "k3": 7, "k4": "pineapple"},
			{},
			{},
			{},
			{},
		},
		nil)
	if err != nil {
		t.Fatal(err)
	}

	checkQuery(t, db, "a b c d e", nil, []uint64{4, 3, 2, 1, 0})

	constraints1 := ndb.Constraints{
		"k1": ndb.GreaterThan(3.0),
		"k2": ndb.EqualTo(false),
		"k3": ndb.EqualTo(7),
		"k4": ndb.LessThan("peach"),
	}

	checkQuery(t, db, "a b c d e", constraints1, []uint64{0})

	constraints2 := ndb.Constraints{
		"k4": ndb.AnyOf([]any{"apple", "banana"}),
	}
	checkQuery(t, db, "a b c d e", constraints2, []uint64{1, 0})

	constraints3 := ndb.Constraints{
		"k4": ndb.Substring("apple"),
	}
	checkQuery(t, db, "a b c d e", constraints3, []uint64{4, 0})

	constraints4 := ndb.Constraints{
		"k4": ndb.Substring("apple"),
		"k1": ndb.AnyOf([]any{5.2, 2.9, 4.7}),
	}
	checkQuery(t, db, "a b c d e", constraints4, []uint64{4})
}

func intString(start, end int) string {
	ints := make([]string, end-start)
	for i := range ints {
		ints[i] = strconv.Itoa(start + i)
	}
	return strings.Join(ints, " ")
}

func TestReturnsCorrectChunkData(t *testing.T) {
	db, err := ndb.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 20; i++ {
		docId := strconv.Itoa(i)
		doc := fmt.Sprintf("document_%d", i)

		meta1 := map[string]interface{}{"id": i, "type": "first"}
		meta2 := map[string]interface{}{"id": i, "type": "second"}

		err := db.Insert(
			doc, docId,
			[]string{intString(i*10, (i+1)*10), intString(i*10, i*10+5)},
			[]map[string]interface{}{meta1, meta2},
			nil)
		if err != nil {
			t.Fatal(err)
		}
	}

	for i := 0; i < 20; i++ {
		query := intString(i*10, (i+1)*10)

		results, err := db.Query(query, 5, nil)
		if err != nil {
			t.Fatal(err)
		}

		if len(results) != 2 {
			t.Fatal("expected 2 results")
		}

		docId := strconv.Itoa(i)
		doc := fmt.Sprintf("document_%d", i)

		if results[0].Id != uint64(2*i) || results[1].Id != uint64((2*i)+1) ||
			results[0].Text != query || results[1].Text != intString(i*10, i*10+5) ||
			results[0].Document != doc || results[1].Document != doc ||
			results[0].DocId != docId || results[1].DocId != docId ||
			results[0].DocVersion != 1 || results[1].DocVersion != 1 {
			t.Fatal("invalid results")
		}

		if len(results[0].Metadata) != 2 || len(results[1].Metadata) != 2 ||
			results[0].Metadata["id"].(int) != i || results[1].Metadata["id"].(int) != i ||
			results[0].Metadata["type"].(string) != "first" || results[1].Metadata["type"].(string) != "second" {
			t.Fatal("invalid metadata")
		}

		constrainedResults, err := db.Query(query, 5, ndb.Constraints{"type": ndb.EqualTo("second")})
		if err != nil {
			t.Fatal(err)
		}

		if len(constrainedResults) != 1 || constrainedResults[0].Id != uint64((2*i)+1) {
			t.Fatal("incorrect constrained results")
		}
	}
}

func TestFinetuning(t *testing.T) {
	db, err := ndb.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	constraint := map[string]interface{}{"key": true}
	err = db.Insert(
		"doc", "id",
		[]string{intString(0, 10), intString(0, 9), intString(0, 8),
			intString(10, 20), intString(20, 30), intString(30, 40)},
		[]map[string]interface{}{{}, constraint, constraint, {}, {}, {}},
		nil)
	if err != nil {
		t.Fatal(err)
	}

	constraints := ndb.Constraints{"key": ndb.EqualTo(true)}
	query := intString(0, 10) + " x y z"

	checkQuery(t, db, query, nil, []uint64{0, 1, 2})
	checkQuery(t, db, query, constraints, []uint64{1, 2})

	err = db.Finetune([]string{"o p", "x y z", "t q v"}, []uint64{4, 2, 3})
	if err != nil {
		t.Fatal(err)
	}

	checkQuery(t, db, query, nil, []uint64{2, 0, 1})
	checkQuery(t, db, query, constraints, []uint64{2, 1})
}

func TestDeletionWithFinetuning(t *testing.T) {
	db, err := ndb.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	err = db.Insert(
		"doc_1", "11",
		[]string{intString(0, 10), intString(30, 40)},
		nil,
		nil)
	if err != nil {
		t.Fatal(err)
	}

	err = db.Insert(
		"doc_2", "22",
		[]string{intString(0, 8), intString(10, 20), intString(20, 30)},
		nil,
		nil)
	if err != nil {
		t.Fatal(err)
	}

	err = db.Insert(
		"doc_3", "33",
		[]string{intString(0, 9)},
		nil,
		nil)
	if err != nil {
		t.Fatal(err)
	}

	query := intString(0, 10) + " x y z"

	checkQuery(t, db, query, nil, []uint64{0, 5, 2})

	err = db.Finetune([]string{"o p", "x y z", "t q v"}, []uint64{4, 2, 3})
	if err != nil {
		t.Fatal(err)
	}

	checkQuery(t, db, query, nil, []uint64{2, 0, 5})

	err = db.Delete("11", false)
	if err != nil {
		t.Fatal(err)
	}

	checkQuery(t, db, query, nil, []uint64{2, 5})

	err = db.Delete("22", false)
	if err != nil {
		t.Fatal(err)
	}

	checkQuery(t, db, query, nil, []uint64{5})
}

func runDeleteDocTest(t *testing.T, keepLatestVersion bool) {
	db, err := ndb.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	insertions := []struct {
		document, doc_id, chunk string
	}{
		{document: "doc_1", doc_id: "id_1", chunk: "a b c d e"},
		{document: "doc_1", doc_id: "id_1", chunk: "a b c d"},
		{document: "doc_1", doc_id: "id_1", chunk: "a b c"},
		{document: "doc_2", doc_id: "id_2", chunk: "a b"},
		{document: "doc_11", doc_id: "id_11", chunk: "x"},
		{document: "doc_12", doc_id: "id_12", chunk: "x y"},
		{document: "doc111_13", doc_id: "id111_13", chunk: "x y z"},
	}

	for _, insertion := range insertions {
		err := db.Insert(insertion.document, insertion.doc_id, []string{insertion.chunk}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
	}

	checkQuery(t, db, "a b c d e", nil, []uint64{0, 1, 2, 3})
	checkQuery(t, db, "x y z", nil, []uint64{6, 5, 4})

	err = db.Delete("id_1", keepLatestVersion)
	if err != nil {
		t.Fatal(err)
	}

	if keepLatestVersion {
		checkQuery(t, db, "a b c d e", nil, []uint64{2, 3})
	} else {
		checkQuery(t, db, "a b c d e", nil, []uint64{3})
	}

	checkQuery(t, db, "x y z", nil, []uint64{6, 5, 4})
}

func TestDeleteDoc(t *testing.T) {
	runDeleteDocTest(t, false)
}

func TestDeleteDocKeepLatestVersion(t *testing.T) {
	runDeleteDocTest(t, true)
}

func TestDocVersioning(t *testing.T) {
	db, err := ndb.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 5; i++ {
		for j := 0; j < 4; j++ {
			err := db.Insert(fmt.Sprintf("%d_%d", i, j+1), fmt.Sprintf("%d", i), []string{"a chunk"}, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
		}
	}

	getSources := func() []ndb.Source {
		sources, err := db.Sources()
		if err != nil {
			t.Fatal(err)
		}

		slices.SortFunc(sources, func(a, b ndb.Source) int {
			if a.Document < b.Document {
				return -1
			}
			if a.Document > b.Document {
				return 1
			}
			return 0
		})
		return sources
	}

	sourcesBefore := getSources()
	if len(sourcesBefore) != 20 {
		t.Fatal("incorrect sources")
	}

	for i, source := range sourcesBefore {
		docId := i / 4
		docVersion := (i % 4) + 1
		if source.Document != fmt.Sprintf("%d_%d", docId, docVersion) ||
			source.DocId != fmt.Sprintf("%d", docId) ||
			source.DocVersion != uint32(docVersion) {
			t.Fatalf("incorrect sources: expected id=%d, version=%d, got %v", docId, docVersion, source)
		}
	}

	for i := 0; i < 5; i++ {
		err := db.Delete(fmt.Sprintf("%d", i), true)
		if err != nil {
			t.Fatal(err)
		}
	}

	sourcesAfter := getSources()

	if len(sourcesAfter) != 5 {
		t.Fatal("incorrect sources")
	}
	for i, source := range sourcesAfter {
		if source.Document != fmt.Sprintf("%d_%d", i, 4) ||
			source.DocId != fmt.Sprintf("%d", i) ||
			source.DocVersion != 4 {
			t.Fatalf("incorrect sources: %v", sourcesAfter)
		}
	}
}
