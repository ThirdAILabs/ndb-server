#pragma once

#include "stdbool.h"

#ifdef __cplusplus
extern "C" {
#endif

typedef struct MetadataValue_t MetadataValue_t;
MetadataValue_t *MetadataValue_bool(bool value);
MetadataValue_t *MetadataValue_int(int value);
MetadataValue_t *MetadataValue_float(float value);
MetadataValue_t *MetadataValue_str(const char *value);
void MetadataValue_free(MetadataValue_t *value);

typedef struct Document_t Document_t;
Document_t *Document_new(const char *document, const char *doc_id);
void Document_free(Document_t *doc);
void Document_add_chunk(Document_t *doc, const char *chunk);
void Document_set_version(Document_t *doc, unsigned int version);
void Document_add_metadata(Document_t *doc, unsigned int i, const char *key,
                           const MetadataValue_t *value);

typedef struct MetadataList_t MetadataList_t;
void MetadataList_free(MetadataList_t *metadata);
unsigned int MetadataList_len(MetadataList_t *metadata);
const char *MetadataList_key(MetadataList_t *metadata, unsigned int i);
int MetadataList_type(MetadataList_t *metadata, unsigned int i);
bool MetadataList_bool(MetadataList_t *metadata, unsigned int i);
int MetadataList_int(MetadataList_t *metadata, unsigned int i);
float MetadataList_float(MetadataList_t *metadata, unsigned int i);
const char *MetadataList_str(MetadataList_t *metadata, unsigned int i);

typedef struct Constraints_t Constraints_t;
Constraints_t *Constraints_new();
void Constraints_free(Constraints_t *constraints);
void Constraints_add_binary_constraint(Constraints_t *constraints, int op,
                                       const char *key,
                                       const MetadataValue_t *value);
void Constraints_add_any_of_constraint(Constraints_t *constraints,
                                       const char *key,
                                       const MetadataValue_t **values, int n);

typedef struct QueryResults_t QueryResults_t;
unsigned int QueryResults_len(QueryResults_t *results);
unsigned long long QueryResults_id(QueryResults_t *results, unsigned int i);
void QueryResults_free(QueryResults_t *results);
const char *QueryResults_text(QueryResults_t *results, unsigned int i);
const char *QueryResults_document(QueryResults_t *results, unsigned int i);
const char *QueryResults_doc_id(QueryResults_t *results, unsigned int i);
unsigned int QueryResults_doc_version(QueryResults_t *results, unsigned int i);
MetadataList_t *QueryResults_metadata(QueryResults_t *results, unsigned int i);
float QueryResults_score(QueryResults_t *results, unsigned int i);

typedef struct StringList_t StringList_t;
StringList_t *StringList_new();
void StringList_free(StringList_t *list);
void StringList_append(StringList_t *list, const char *value);

typedef struct LabelList_t LabelList_t;
LabelList_t *LabelList_new();
void LabelList_free(LabelList_t *list);
void LabelList_append(LabelList_t *list, unsigned long long value);

typedef struct Sources_t Sources_t;
void Sources_free(Sources_t *sources);
unsigned int Sources_len(Sources_t *sources);
const char *Sources_document(Sources_t *sources, unsigned int i);
const char *Sources_doc_id(Sources_t *sources, unsigned int i);
unsigned int Sources_doc_version(Sources_t *sources, unsigned int i);

typedef struct NeuralDB_t NeuralDB_t;
NeuralDB_t *NeuralDB_new(const char *save_path, const char **err_ptr);
void NeuralDB_free(NeuralDB_t *ndb);
void NeuralDB_insert(NeuralDB_t *ndb, Document_t *doc, const char **err_ptr);
QueryResults_t *NeuralDB_query(NeuralDB_t *ndb, const char *query,
                               unsigned int topk,
                               const Constraints_t *constraints,
                               const char **err_ptr);
void NeuralDB_finetune(NeuralDB_t *ndb, const StringList_t *queries,
                       const LabelList_t *chunk_ids, const char **err_ptr);
void NeuralDB_associate(NeuralDB_t *ndb, const StringList_t *sources,
                        const StringList_t *targets, unsigned int strength,
                        const char **err_ptr);
void NeuralDB_delete_doc(NeuralDB_t *ndb, const char *doc_id,
                         bool keep_latest_version, const char **err_ptr);
Sources_t *NeuralDB_sources(NeuralDB_t *ndb, const char **err_ptr);
void NeuralDB_save(NeuralDB_t *ndb, const char *save_path,
                   const char **err_ptr);

void set_license_key(const char *key, const char **err_ptr);

void set_license_path(const char *path, const char **err_ptr);

#ifdef __cplusplus
}
#endif
