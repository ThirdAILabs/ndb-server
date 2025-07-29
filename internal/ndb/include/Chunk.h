#pragma once

#include "Constraints.h"
#include <string>

namespace thirdai::search::ndb {

using ChunkId = uint64_t;
using DocId = std::string;

struct Chunk {
  ChunkId id;
  std::string text;

  std::string document;
  DocId doc_id;
  uint32_t doc_version;

  MetadataMap metadata;

  Chunk(ChunkId id, std::string text, std::string document, DocId doc_id,
        uint32_t doc_version, MetadataMap metadata)
      : id(id), text(std::move(text)), document(std::move(document)),
        doc_id(std::move(doc_id)), doc_version(doc_version),
        metadata(std::move(metadata)) {}
};

} // namespace thirdai::search::ndb