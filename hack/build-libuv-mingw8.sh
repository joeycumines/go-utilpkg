#!/usr/bin/env bash
# Builds libuv 1.52.0 as a static library with the MinGW-w64 8.1.0 toolchain
# (the gcc that Go's cgo uses on this host), so the libuv objects share one
# runtime ABI with runtime/cgo. Installs into a prefix and writes a pkg-config
# .pc file the tournament can consume via PKG_CONFIG_PATH.
set -euo pipefail

MGW="C:/Program Files/mingw-w64/x86_64-8.1.0-posix-seh-rt_v6-rev0/mingw64/bin"
BUILDROOT="C:/Users/under/libuv-build"
PREFIX="C:/Users/under/libuv-mingw8"
SRC="$BUILDROOT/libuv-v1.52.0"

export PATH="$MGW:$PATH"

mkdir -p "$BUILDROOT"
cd "$BUILDROOT"

if [ ! -f libuv-1.52.0.tar.gz ]; then
  curl --fail --location --silent --show-error \
    -o libuv-1.52.0.tar.gz \
    https://dist.libuv.org/dist/v1.52.0/libuv-v1.52.0.tar.gz
fi

if [ ! -f "$SRC/CMakeLists.txt" ]; then
  rm -rf "$SRC" "$BUILDROOT/libuv-1.52.0"
  tar xzf libuv-1.52.0.tar.gz
fi

mkdir -p "$SRC/build"
cd "$SRC/build"

cmake -G "MinGW Makefiles" \
  -DCMAKE_C_COMPILER=gcc \
  -DCMAKE_INSTALL_PREFIX="$PREFIX" \
  -DCMAKE_BUILD_TYPE=Release \
  -DBUILD_SHARED_LIBS=OFF \
  -DLIBUV_BUILD_TESTS=OFF \
  -DCMAKE_C_FLAGS="-DPROCESSOR_ARCHITECTURE_ARM64=12 -DWSA_FLAG_NO_HANDLE_INHERIT=0x00000080 -DFILE_DEVICE_CONSOLE=0x00000050" \
  "$SRC"

cmake --build . --parallel
cmake --install .

mkdir -p "$PREFIX/lib/pkgconfig"
cat > "$PREFIX/lib/pkgconfig/libuv.pc" <<PC
prefix=$PREFIX
exec_prefix=\${prefix}
libdir=\${exec_prefix}/lib
includedir=\${prefix}/include

Name: libuv
Description: multi-platform support library with a focus on asynchronous I/O (built with mingw-w64 8.1.0)
Version: 1.52.0
URL: http://libuv.org/
Libs: -L\${libdir} -luv -lpsapi -luser32 -ladvapi32 -liphlpapi -luserenv -lws2_32 -ldbghelp -lole32 -lshell32
Cflags: -I\${includedir}
PC

echo "=== BUILD COMPLETE ==="
ls -la "$PREFIX/lib/libuv.a" "$PREFIX/include/uv.h" "$PREFIX/lib/pkgconfig/libuv.pc"
gcc --version | head -n1
