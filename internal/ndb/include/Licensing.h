#pragma once

#include <string>

namespace thirdai::licensing {

void activate(std::string api_key);

void setLicensePath(std::string license_path, bool verbose = false);

} // namespace thirdai::licensing