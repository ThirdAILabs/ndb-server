# Notes on Building Go NDB Bindings

## Using ThirdAI as a static library

Currently we are building the bindings using thirdai as a static library. The main limitation of this is that linking openssl and openmp become a little more complicated. This is why we have flags on line 4 of ndb.go to set the link paths so that cgo can find the libraries. These assume the installed path and version is consistent on macos. 

The other libraries are obtained by runing `bin/build.py -f THIRDAI_BUILD_LICENSE THIRDAI_CHECK_LICENSE` from within universe, and copying the resulting static libraries. 
The following libraries are needed
- `build/libthirdai.a`
- `build/deps/rocksdb/librocksdb.a`
- `build/deps/utf8proc/libutf8proc.a`
- `build/deps/cryptopp-cmake/cryptopp/libcryptopp.a`

## Using ThirdAI as a shared library

Another option is to use thirdai as a shared library. 
Steps:
1. We need to export the methods of `OnDiskNeuralDB` that are used by the bindings, as well as the `setLicensePath` method. See `auto_ml/src/cpp_classifier/CppClassifer.h` for an example of how to export the methods.
2. Build the shared library with `bin/build.py -t thirdai_core -f THIRDAI_BUILD_LICENSE THIRDAI_CHECK_LICENSE`. This will generate `libthirdai_core.dylib`. On macos arm64 it is located in `build/lib.macosx-12.0-arm64-cpython-311/thirdai/libthirdai_core.dylib`. Note on linux it should by `libthirdai_core.so`.
3. We can then just link this library with cgo. The cgo flags change to `#cgo darwin LDFLAGS: -L. -lthirdai_core`. 
4. There will be an error running the application which uses the bindings because it will not be able to find the thirdai_core library at runtime. This can be fixed (at least on mac) by setting the `DYLD_LIBRARY_PATH` to the path the directory containing `libthirdai_core` library. This could also potentially be fixed by copying the library to `/usr/local/lib` or one of other default library search paths, but I haven't tried this. 