#include "binding.h"
#include "OnDiskNeuralDB.h"
#include <algorithm>
#include <cstring>
#include <iostream>
#include <memory>
#include <optional>
#include <vector>

using thirdai::search::ndb::AnyOf;
using thirdai::search::ndb::Chunk;
using thirdai::search::ndb::EqualTo;
using thirdai::search::ndb::GreaterThan;
using thirdai::search::ndb::LessThan;
using thirdai::search::ndb::MetadataMap;
using thirdai::search::ndb::MetadataValue;
using thirdai::search::ndb::OnDiskNeuralDB;
using thirdai::search::ndb::QueryConstraints;
using thirdai::search::ndb::Source;
using thirdai::search::ndb::Substring;

void copyError(const std::exception &e, const char **err_ptr) {
  char *err_msg = new char[std::strlen(e.what()) + 1];
  std::strcpy(err_msg, e.what());
  *err_ptr = err_msg;
}

struct MetadataValue_t {
  MetadataValue value;
};

MetadataValue_t *MetadataValue_bool(bool value) {
  MetadataValue_t *out = new MetadataValue_t();
  out->value = MetadataValue::Bool(value);
  return out;
}

MetadataValue_t *MetadataValue_int(int value) {
  MetadataValue_t *out = new MetadataValue_t();
  out->value = MetadataValue::Int(value);
  return out;
}

MetadataValue_t *MetadataValue_float(float value) {
  MetadataValue_t *out = new MetadataValue_t();
  out->value = MetadataValue::Float(value);
  return out;
}

MetadataValue_t *MetadataValue_str(const char *value) {
  MetadataValue_t *out = new MetadataValue_t();
  out->value = MetadataValue::Str(value);
  return out;
}

void MetadataValue_free(MetadataValue_t *value) { delete value; }

struct Document_t {
  std::vector<std::string> chunks;
  std::vector<MetadataMap> metadata;
  std::string document;
  std::string doc_id;
  std::optional<uint32_t> doc_version;
};

Document_t *Document_new(const char *document, const char *doc_id) {
  Document_t *doc = new Document_t();
  doc->document = document;
  doc->doc_id = doc_id;

  return doc;
}

void Document_free(Document_t *doc) { delete doc; }

void Document_add_chunk(Document_t *doc, const char *chunk) {
  doc->chunks.emplace_back(chunk);
  doc->metadata.emplace_back();
}

void Document_set_version(Document_t *doc, unsigned int version) {
  doc->doc_version = version;
}

void Document_add_metadata(Document_t *doc, unsigned int i, const char *key,
                           const MetadataValue_t *value) {
  doc->metadata.at(i)[key] = value->value;
}

struct MetadataList_t {
  std::vector<std::pair<std::string, MetadataValue>> metadata;
};

void MetadataList_free(MetadataList_t *metadata) { delete metadata; }

unsigned int MetadataList_len(MetadataList_t *metadata) {
  return metadata->metadata.size();
}

const char *MetadataList_key(MetadataList_t *metadata, unsigned int i) {
  return metadata->metadata.at(i).first.c_str();
}

int MetadataList_type(MetadataList_t *metadata, unsigned int i) {
  return int(metadata->metadata.at(i).second.type());
}

bool MetadataList_bool(MetadataList_t *metadata, unsigned int i) {
  return metadata->metadata.at(i).second.asBool();
}

int MetadataList_int(MetadataList_t *metadata, unsigned int i) {
  return metadata->metadata.at(i).second.asInt();
}

float MetadataList_float(MetadataList_t *metadata, unsigned int i) {
  return metadata->metadata.at(i).second.asFloat();
}

const char *MetadataList_str(MetadataList_t *metadata, unsigned int i) {
  return metadata->metadata.at(i).second.asStr().c_str();
}

struct Constraints_t {
  QueryConstraints constraints;
};

Constraints_t *Constraints_new() { return new Constraints_t(); }

void Constraints_free(Constraints_t *constraints) { delete constraints; }

const int BinaryConstraintEq = 0;
const int BinaryConstraintLt = 1;
const int BinaryConstraintGt = 2;
const int BinaryConstraintSubstr = 3;

void Constraints_add_binary_constraint(Constraints_t *constraints, int op,
                                       const char *key,
                                       const MetadataValue_t *value) {
  switch (op) {
  case BinaryConstraintEq:
    constraints->constraints[key] = EqualTo::make(value->value);
    break;
  case BinaryConstraintLt:
    constraints->constraints[key] = LessThan::make(value->value);
    break;
  case BinaryConstraintGt:
    constraints->constraints[key] = GreaterThan::make(value->value);
    break;
  case BinaryConstraintSubstr:
    constraints->constraints[key] = Substring::make(value->value);
    break;
  }
}

void Constraints_add_any_of_constraint(Constraints_t *constraints,
                                       const char *key,
                                       const MetadataValue_t **values, int n) {
  std::vector<MetadataValue> value_vec;
  value_vec.reserve(n);
  for (int i = 0; i < n; ++i) {
    value_vec.push_back(values[i]->value);
  }

  constraints->constraints[key] = AnyOf::make(std::move(value_vec));
}

struct QueryResults_t {
  std::vector<std::pair<Chunk, float>> results;
};

void QueryResults_free(QueryResults_t *results) { delete results; }

unsigned int QueryResults_len(QueryResults_t *results) {
  return results->results.size();
}

unsigned long long QueryResults_id(QueryResults_t *results, unsigned int i) {
  return results->results.at(i).first.id;
}

const char *QueryResults_text(QueryResults_t *results, unsigned int i) {
  return results->results.at(i).first.text.c_str();
}

const char *QueryResults_document(QueryResults_t *results, unsigned int i) {
  return results->results.at(i).first.document.c_str();
}

const char *QueryResults_doc_id(QueryResults_t *results, unsigned int i) {
  return results->results.at(i).first.doc_id.c_str();
}

unsigned int QueryResults_doc_version(QueryResults_t *results, unsigned int i) {
  return results->results.at(i).first.doc_version;
}

float QueryResults_score(QueryResults_t *results, unsigned int i) {
  return results->results.at(i).second;
}

MetadataList_t *QueryResults_metadata(QueryResults_t *results, unsigned int i) {
  const auto &metadata_map = results->results.at(i).first.metadata;
  MetadataList_t *out = new MetadataList_t();
  out->metadata = {metadata_map.begin(), metadata_map.end()};
  return out;
}

struct StringList_t {
  std::vector<std::string> list;
};

StringList_t *StringList_new() { return new StringList_t(); }
void StringList_free(StringList_t *list) { delete list; }
void StringList_append(StringList_t *list, const char *value) {
  list->list.emplace_back(value);
}

struct LabelList_t {
  std::vector<std::vector<uint64_t>> list;
};

LabelList_t *LabelList_new() { return new LabelList_t(); }
void LabelList_free(LabelList_t *list) { delete list; }
void LabelList_append(LabelList_t *list, unsigned long long value) {
  list->list.emplace_back(std::vector<uint64_t>{value});
}

struct Sources_t {
  std::vector<Source> sources;
};

void Sources_free(Sources_t *sources) { delete sources; }

unsigned int Sources_len(Sources_t *sources) { return sources->sources.size(); }

const char *Sources_document(Sources_t *sources, unsigned int i) {
  return sources->sources.at(i).document.c_str();
}

const char *Sources_doc_id(Sources_t *sources, unsigned int i) {
  return sources->sources.at(i).doc_id.c_str();
}

unsigned int Sources_doc_version(Sources_t *sources, unsigned int i) {
  return sources->sources.at(i).doc_version;
}

struct NeuralDB_t {
  std::unique_ptr<OnDiskNeuralDB> ndb;

  NeuralDB_t(const std::string &save_path)
      : ndb(OnDiskNeuralDB::make(save_path)) {}
};

NeuralDB_t *NeuralDB_new(const char *save_path, const char **err_ptr) {
  try {
    std::string path(save_path);
    return new NeuralDB_t(path);
  } catch (const std::exception &e) {
    // TODO(Nicholas): have case for NeuralDBError to return better errors
    copyError(e, err_ptr);
    return nullptr;
  }
}

void NeuralDB_free(NeuralDB_t *ndb) { delete ndb; }

void NeuralDB_insert(NeuralDB_t *ndb, Document_t *doc, const char **err_ptr) {
  try {
    ndb->ndb->insert(
        /*chunks=*/doc->chunks,
        /*metadata*/ doc->metadata,
        /*document=*/doc->document,
        /*doc_id=*/doc->doc_id,
        /*doc_version=*/doc->doc_version);
  } catch (const std::exception &e) {
    copyError(e, err_ptr);
    return;
  }
}

QueryResults_t *NeuralDB_query(NeuralDB_t *ndb, const char *query,
                               unsigned int topk,
                               const Constraints_t *constraints,
                               const char **err_ptr) {
  try {
    std::vector<std::pair<Chunk, float>> results;
    if (constraints == nullptr) {
      results = ndb->ndb->query(query, topk);
    } else {
      results = ndb->ndb->rank(query, constraints->constraints, topk);
    }
    auto out = new QueryResults_t();
    out->results = std::move(results);
    return out;
  } catch (const std::exception &e) {
    // TODO(Nicholas): have case for NeuralDBError to return better errors
    copyError(e, err_ptr);
    return nullptr;
  }
}

void NeuralDB_finetune(NeuralDB_t *ndb, const StringList_t *queries,
                       const LabelList_t *chunk_ids, const char **err_ptr) {
  try {
    ndb->ndb->finetune(queries->list, chunk_ids->list);
  } catch (const std::exception &e) {
    // TODO(Nicholas): have case for NeuralDBError to return better errors
    copyError(e, err_ptr);
    return;
  }
}

void NeuralDB_associate(NeuralDB_t *ndb, const StringList_t *sources,
                        const StringList_t *targets, unsigned int strength,
                        const char **err_ptr) {
  try {
    ndb->ndb->associate(sources->list, targets->list, strength);
  } catch (const std::exception &e) {
    // TODO(Nicholas): have case for NeuralDBError to return better errors
    copyError(e, err_ptr);
    return;
  }
}

void NeuralDB_delete_doc(NeuralDB_t *ndb, const char *doc_id,
                         bool keep_latest_version, const char **err_ptr) {

  try {
    ndb->ndb->deleteDoc(doc_id, keep_latest_version);
  } catch (const std::exception &e) {
    // TODO(Nicholas): have case for NeuralDBError to return better errors
    copyError(e, err_ptr);
    return;
  }
}

Sources_t *NeuralDB_sources(NeuralDB_t *ndb, const char **err_ptr) {
  try {
    auto sources = ndb->ndb->sources();
    auto out = new Sources_t();
    out->sources = std::move(sources);
    return out;
  } catch (const std::exception &e) {
    // TODO(Nicholas): have case for NeuralDBError to return better errors
    copyError(e, err_ptr);
    return nullptr;
  }
}

void NeuralDB_save(NeuralDB_t *ndb, const char *save_path,
                   const char **err_ptr) {
  try {
    ndb->ndb->save(save_path);
  } catch (const std::exception &e) {
    copyError(e, err_ptr);
    return;
  }
}
