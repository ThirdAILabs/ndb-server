#pragma once

#include "Chunk.h"
#include "Constraints.h"
#include <optional>
#include <vector>

namespace thirdai::search::ndb {

struct Source {
  std::string document;
  DocId doc_id;
  uint32_t doc_version;

  Source(std::string document, DocId doc_id, uint32_t doc_version)
      : document(std::move(document)), doc_id(std::move(doc_id)),
        doc_version(doc_version) {}
};

struct InsertMetadata {
  DocId doc_id;
  uint32_t doc_version;
  ChunkId start_id;
  ChunkId end_id;

  InsertMetadata(DocId doc_id, uint32_t doc_version, ChunkId start_id,
                 ChunkId end_id)
      : doc_id(std::move(doc_id)), doc_version(doc_version), start_id(start_id),
        end_id(end_id) {}
};

class NeuralDB {
public:
  virtual InsertMetadata insert(const std::vector<std::string> &chunks,
                                const std::vector<MetadataMap> &metadata,
                                const std::string &document,
                                const DocId &doc_id,
                                std::optional<uint32_t> doc_version) = 0;

  virtual std::vector<std::pair<Chunk, float>> query(const std::string &query,
                                                     uint32_t top_k) = 0;

  virtual std::vector<std::pair<Chunk, float>>
  rank(const std::string &query, const QueryConstraints &constraints,
       uint32_t top_k) = 0;

  virtual void finetune(const std::vector<std::string> &queries,
                        const std::vector<std::vector<ChunkId>> &chunk_ids) = 0;

  virtual void associate(const std::vector<std::string> &sources,
                         const std::vector<std::string> &targets,
                         uint32_t strength) = 0;

  virtual void deleteDocVersion(const DocId &doc_id, uint32_t doc_version) = 0;

  virtual void deleteDoc(const DocId &doc_id, bool keep_latest_version) = 0;

  virtual void prune() = 0;

  virtual std::vector<Source> sources() = 0;

  virtual ~NeuralDB() = default;
};

} // namespace thirdai::search::ndb