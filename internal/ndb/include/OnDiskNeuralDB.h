#pragma once

#include "Chunk.h"
#include "NeuralDB.h"
#include <memory>
#include <optional>
#include <unordered_map>
#include <vector>

namespace thirdai::search::ndb {

class OnDiskNeuralDB final : public NeuralDB {
public:
  static std::unique_ptr<OnDiskNeuralDB> make(const std::string &save_path);

  explicit OnDiskNeuralDB(const std::string &save_path);

  InsertMetadata insert(const std::vector<std::string> &chunks,
                        const std::vector<MetadataMap> &metadata,
                        const std::string &document, const DocId &doc_id,
                        std::optional<uint32_t> doc_version) final;

  std::vector<std::pair<Chunk, float>> query(const std::string &query,
                                             uint32_t top_k) final;

  std::vector<std::pair<Chunk, float>> rank(const std::string &query,
                                            const QueryConstraints &constraints,
                                            uint32_t top_k) final;

  void finetune(const std::vector<std::string> &queries,
                const std::vector<std::vector<ChunkId>> &chunk_ids) final;

  void associate(const std::vector<std::string> &sources,
                 const std::vector<std::string> &targets,
                 uint32_t strength) final;

  void deleteDocVersion(const DocId &doc_id, uint32_t doc_version) final;

  void deleteDoc(const DocId &doc_id, bool keep_latest_version) final;

  void prune() final;

  std::vector<Source> sources() final;

  void save(const std::string &save_path) const;
};

} // namespace thirdai::search::ndb