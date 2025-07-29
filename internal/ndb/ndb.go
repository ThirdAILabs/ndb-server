package ndb

// #cgo linux LDFLAGS: -L./lib/linux_x64 -lthirdai -lrocksdb -lutf8proc -lcryptopp -fopenmp -lssl -lcrypto
// #cgo darwin LDFLAGS: -L./lib/macos_arm64 -lthirdai -lrocksdb -lutf8proc -lcryptopp -L/opt/homebrew/opt/libomp/lib/ -lomp -L/opt/homebrew/Cellar/openssl@3/3.4.0/lib/ -lssl -lcrypto
// #cgo CFLAGS: -O3
// #cgo CXXFLAGS: -O3 -fPIC -std=c++17 -I./include -fvisibility=hidden
// #include "binding.h"
// #include <stdlib.h>
import "C"
import (
	"errors"
	"fmt"
	"strings"
	"unsafe"
)

type NeuralDB struct {
	ndb *C.NeuralDB_t
}

func New(savePath string) (NeuralDB, error) {
	savePathCStr := C.CString(savePath)
	defer C.free(unsafe.Pointer(savePathCStr))

	var err *C.char
	ndb := C.NeuralDB_new(savePathCStr, &err)
	if err != nil {
		defer C.free(unsafe.Pointer(err))
		return NeuralDB{}, errors.New(C.GoString(err))
	}

	return NeuralDB{ndb: ndb}, nil
}

func (ndb *NeuralDB) Free() {
	C.NeuralDB_free(ndb.ndb)
}

func newMetadataValue(value interface{}) (*C.MetadataValue_t, error) {
	switch value := value.(type) {
	case bool:
		return C.MetadataValue_bool(C.bool(value)), nil
	case int:
		return C.MetadataValue_int(C.int(value)), nil
	case float32:
		return C.MetadataValue_float(C.float(value)), nil
	case float64:
		return C.MetadataValue_float(C.float(value)), nil
	case string:
		valueCStr := C.CString(value)
		defer C.free(unsafe.Pointer(valueCStr))
		return C.MetadataValue_str(valueCStr), nil
	default:
		return nil, fmt.Errorf("unsupported metadata type %T for value %v: type must be bool, int, float, or string", value, value)
	}
}

func newDocument(document, docId string) *C.Document_t {
	documentCStr := C.CString(document)
	defer C.free(unsafe.Pointer(documentCStr))
	docIdCStr := C.CString(docId)
	defer C.free(unsafe.Pointer(docIdCStr))

	doc := C.Document_new(documentCStr, docIdCStr)

	return doc
}

func addChunk(doc *C.Document_t, chunk string) {
	chunkCStr := C.CString(chunk)
	defer C.free(unsafe.Pointer(chunkCStr))
	C.Document_add_chunk(doc, chunkCStr)
}

func addMetadata(doc *C.Document_t, i int, key string, value interface{}) error {
	keyCStr := C.CString(key)
	defer C.free(unsafe.Pointer(keyCStr))

	metadataValue, err := newMetadataValue(value)
	if err != nil {
		return err
	}
	defer C.MetadataValue_free(metadataValue)

	C.Document_add_metadata(doc, C.uint(i), keyCStr, metadataValue)

	return nil
}

func checkInsertArgs(document, docId string, chunks []string, metadata []map[string]interface{}) error {
	if len(document) == 0 {
		return fmt.Errorf("document must not be empty string")
	}
	if len(docId) == 0 {
		return fmt.Errorf("doc_id must not be empty string")
	}
	if strings.ContainsRune(docId, ';') {
		return fmt.Errorf("doc_id cannot contain ';'")
	}
	if metadata != nil && len(chunks) != len(metadata) {
		return fmt.Errorf("len of metadata must match the len of chunks if metadata is specified")
	}

	for _, metadata := range metadata {
		for key, value := range metadata {
			switch value.(type) {
			case bool, int, float32, float64, string:
				// pass
			default:
				return fmt.Errorf("unsupported metadata type %T for key %s value %v: type must be bool, int, float, or string", value, key, value)
			}
		}
	}

	return nil
}

func (ndb *NeuralDB) Insert(document, docId string, chunks []string, metadata []map[string]interface{}, version *uint) error {
	if err := checkInsertArgs(document, docId, chunks, metadata); err != nil {
		return err
	}

	doc := newDocument(document, docId)
	defer C.Document_free(doc)
	for _, chunk := range chunks {
		addChunk(doc, chunk)
	}

	for i, m := range metadata { // this handles if metadata is nil
		for k, v := range m {
			err := addMetadata(doc, i, k, v)
			if err != nil {
				return err
			}
		}
	}

	if version != nil {
		C.Document_set_version(doc, C.uint(*version))
	}

	var err *C.char
	C.NeuralDB_insert(ndb.ndb, doc, &err)
	if err != nil {
		defer C.free(unsafe.Pointer(err))
		return errors.New(C.GoString(err))
	}

	return nil
}

type Constraint interface {
	addToConstraints(constraints *C.Constraints_t, key string) error
}

type binaryConstraintOp int8

const (
	BinaryConstraintEq binaryConstraintOp = iota
	BinaryConstraintLt
	BinaryConstraintGt
	BinaryConstraintSubstr
)

type binaryConstraint struct {
	value interface{}
	op    binaryConstraintOp
}

func (c binaryConstraint) addToConstraints(constraints *C.Constraints_t, key string) error {
	keyCStr := C.CString(key)
	defer C.free(unsafe.Pointer(keyCStr))

	metadataValue, err := newMetadataValue(c.value)
	if err != nil {
		return fmt.Errorf("invalid constraint for key '%v': %w", key, err)
	}
	defer C.MetadataValue_free(metadataValue)

	C.Constraints_add_binary_constraint(constraints, C.int(c.op), keyCStr, metadataValue)

	return nil
}

func EqualTo(value interface{}) Constraint {
	return binaryConstraint{value: value, op: BinaryConstraintEq}
}

func LessThan(value interface{}) Constraint {
	return binaryConstraint{value: value, op: BinaryConstraintLt}
}

func GreaterThan(value interface{}) Constraint {
	return binaryConstraint{value: value, op: BinaryConstraintGt}
}

func Substring(value interface{}) Constraint {
	return binaryConstraint{value: value, op: BinaryConstraintSubstr}
}

type anyOfConstraint struct {
	values []interface{}
}

func (c anyOfConstraint) addToConstraints(constraints *C.Constraints_t, key string) error {
	keyCStr := C.CString(key)
	defer C.free(unsafe.Pointer(keyCStr))

	valuesC := make([]*C.MetadataValue_t, len(c.values))
	for i, v := range c.values {
		metadataValue, err := newMetadataValue(v)
		if err != nil {
			return fmt.Errorf("invalid value for key '%v': %w", key, err)
		}
		valuesC[i] = metadataValue
		defer C.MetadataValue_free(metadataValue)
	}

	C.Constraints_add_any_of_constraint(constraints, keyCStr, &valuesC[0], C.int(len(valuesC)))

	return nil
}

func AnyOf(values []interface{}) Constraint {
	return anyOfConstraint{values: values}
}

type Constraints = map[string]Constraint

func newConstraints(constraints Constraints) (*C.Constraints_t, error) {
	constraintsMap := C.Constraints_new()

	for k, v := range constraints {
		err := v.addToConstraints(constraintsMap, k)
		if err != nil {
			return nil, err
		}
	}

	return constraintsMap, nil
}

type Chunk struct {
	Id         uint64
	Text       string
	Document   string
	DocId      string
	DocVersion uint32
	Metadata   map[string]interface{}
	Score      float32
}

func (ndb *NeuralDB) Query(query string, topk int, constraints Constraints) ([]Chunk, error) {
	if topk <= 0 {
		return nil, errors.New("topk must be > 0")
	}
	queryCStr := C.CString(query)
	defer C.free(unsafe.Pointer(queryCStr))

	var constraintsMap *C.Constraints_t
	if len(constraints) > 0 {
		var err error
		constraintsMap, err = newConstraints(constraints)
		if constraintsMap != nil {
			// This is because we could allocate the map, convert some of the constraints,
			// then get a type error, in which case the map should still be freed.
			defer C.Constraints_free(constraintsMap)
		}
		if err != nil {
			return nil, err
		}
	}

	var err *C.char
	results := C.NeuralDB_query(ndb.ndb, queryCStr, C.uint(topk), constraintsMap, &err)
	if err != nil {
		defer C.free(unsafe.Pointer(err))
		return nil, errors.New(C.GoString(err))
	}
	defer C.QueryResults_free(results)

	nResults := C.QueryResults_len(results)
	chunks := make([]Chunk, nResults)
	for i := C.uint(0); i < nResults; i++ {
		chunks[i].Id = uint64(C.QueryResults_id(results, i))
		chunks[i].Text = C.GoString(C.QueryResults_text(results, i))
		chunks[i].Document = C.GoString(C.QueryResults_document(results, i))
		chunks[i].DocId = C.GoString(C.QueryResults_doc_id(results, i))
		chunks[i].DocVersion = uint32(C.QueryResults_doc_version(results, i))
		chunks[i].Score = float32(C.QueryResults_score(results, i))
		chunks[i].Metadata = convertMetadata(C.QueryResults_metadata(results, i))
	}

	return chunks, nil
}

func convertMetadata(metadata *C.MetadataList_t) map[string]interface{} {
	defer C.MetadataList_free(metadata)

	len := C.MetadataList_len(metadata)
	out := make(map[string]interface{})

	for i := C.uint(0); i < len; i++ {
		key := C.GoString(C.MetadataList_key(metadata, i))
		switch C.MetadataList_type(metadata, i) {
		case 0:
			out[key] = bool(C.MetadataList_bool(metadata, i))
		case 1:
			out[key] = int(C.MetadataList_int(metadata, i))
		case 2:
			out[key] = float32(C.MetadataList_float(metadata, i))
		case 3:
			out[key] = C.GoString(C.MetadataList_str(metadata, i))
		}
	}
	return out
}

func newStringList(values []string) *C.StringList_t {
	list := C.StringList_new()
	for _, v := range values {
		vCStr := C.CString(v)
		C.StringList_append(list, vCStr)
		C.free(unsafe.Pointer(vCStr))
	}
	return list
}

func newLabelList(values []uint64) *C.LabelList_t {
	list := C.LabelList_new()
	for _, v := range values {
		C.LabelList_append(list, C.ulonglong(v))
	}
	return list
}

func CheckFinetuneArgs(queries []string, labels []uint64) error {
	if len(queries) != len(labels) {
		return fmt.Errorf("len of queries must match len of labels")
	}
	return nil
}

func (ndb *NeuralDB) Finetune(queries []string, labels []uint64) error {
	if err := CheckFinetuneArgs(queries, labels); err != nil {
		return err
	}

	queryList := newStringList(queries)
	defer C.StringList_free(queryList)

	labelList := newLabelList(labels)
	defer C.LabelList_free(labelList)

	var err *C.char
	C.NeuralDB_finetune(ndb.ndb, queryList, labelList, &err)
	if err != nil {
		defer C.free(unsafe.Pointer(err))
		return errors.New(C.GoString(err))
	}

	return nil
}

func CheckAssociateArgs(sources, targets []string) error {
	if len(sources) != len(targets) {
		return fmt.Errorf("len of sources must match length of targets")
	}
	return nil
}

const (
	DefaultAssociateStrength uint32 = 4
)

func (ndb *NeuralDB) Associate(sources, targets []string, strength uint32) error {
	if err := CheckAssociateArgs(sources, targets); err != nil {
		return err
	}

	sourceList := newStringList(sources)
	defer C.StringList_free(sourceList)

	targetList := newStringList(targets)
	defer C.StringList_free(targetList)

	var err *C.char
	C.NeuralDB_associate(ndb.ndb, sourceList, targetList, C.uint(strength), &err)
	if err != nil {
		defer C.free(unsafe.Pointer(err))
		return errors.New(C.GoString(err))
	}

	return nil
}

func (ndb *NeuralDB) Delete(docId string, keepLatestVersion bool) error {
	docIdCStr := C.CString(docId)
	defer C.free(unsafe.Pointer(docIdCStr))

	var err *C.char
	C.NeuralDB_delete_doc(ndb.ndb, docIdCStr, C.bool(keepLatestVersion), &err)
	if err != nil {
		defer C.free(unsafe.Pointer(err))
		return errors.New(C.GoString(err))
	}

	return nil
}

type Source struct {
	Document   string
	DocId      string
	DocVersion uint32
}

func (ndb *NeuralDB) Sources() ([]Source, error) {
	var err *C.char
	sources := C.NeuralDB_sources(ndb.ndb, &err)
	if err != nil {
		defer C.free(unsafe.Pointer(err))
		return nil, errors.New(C.GoString(err))
	}
	defer C.Sources_free(sources)

	nResults := C.Sources_len(sources)
	output := make([]Source, nResults)
	for i := C.uint(0); i < nResults; i++ {
		output[i].Document = C.GoString(C.Sources_document(sources, i))
		output[i].DocId = C.GoString(C.Sources_doc_id(sources, i))
		output[i].DocVersion = uint32(C.Sources_doc_version(sources, i))
	}

	return output, nil
}

func (ndb *NeuralDB) Save(savePath string) error {
	savePathCStr := C.CString(savePath)
	defer C.free(unsafe.Pointer(savePathCStr))

	var err *C.char
	C.NeuralDB_save(ndb.ndb, savePathCStr, &err)
	if err != nil {
		defer C.free(unsafe.Pointer(err))
		return errors.New(C.GoString(err))
	}

	return nil
}

func SetLicenseKey(key string) error {
	keyCStr := C.CString(key)
	defer C.free(unsafe.Pointer(keyCStr))

	var err *C.char
	C.set_license_key(keyCStr, &err)
	if err != nil {
		defer C.free(unsafe.Pointer(err))
		return errors.New(C.GoString(err))
	}

	return nil
}

func SetLicensePath(path string) error {
	pathCStr := C.CString(path)
	defer C.free(unsafe.Pointer(pathCStr))

	var err *C.char
	C.set_license_path(pathCStr, &err)
	if err != nil {
		defer C.free(unsafe.Pointer(err))
		return errors.New(C.GoString(err))
	}

	return nil
}
