# Host unit tests for the board-independent core (no Zephyr needed).
cmake_minimum_required(VERSION 3.20)
project(nu54_core_tests C)
enable_testing()

add_executable(test_frame test_frame.c ../core/src/nu54_frame.c)
target_include_directories(test_frame PRIVATE ../core/include)
target_compile_options(test_frame PRIVATE -Wall -Wextra -Werror)
add_test(NAME frame COMMAND test_frame)

add_executable(test_button test_button.c ../core/src/nu54_button.c)
target_include_directories(test_button PRIVATE ../core/include)
target_compile_options(test_button PRIVATE -Wall -Wextra -Werror)
add_test(NAME button COMMAND test_button)

# --- Cryptography (third_party) and shared-vector tests -------------------------------------
# libsecp256k1 v0.8.0 with the recovery module. ECMULT_WINDOW_SIZE 4 keeps the verification
# table small enough for the device; the host tests use the same setting.
find_package(Python3 REQUIRED COMPONENTS Interpreter)
set(SECP ${CMAKE_CURRENT_SOURCE_DIR}/../third_party/secp256k1)
add_library(nu54_secp256k1 STATIC ${SECP}/src/secp256k1.c ${SECP}/src/precomputed_ecmult.c ${SECP}/src/precomputed_ecmult_gen.c)
target_include_directories(nu54_secp256k1 PUBLIC ${SECP}/include PRIVATE ${SECP}/src)
target_compile_definitions(nu54_secp256k1 PRIVATE ENABLE_MODULE_RECOVERY=1 ECMULT_WINDOW_SIZE=4)

add_library(nu54_crypto STATIC
  ../core/src/nu54_keccak.c ../core/src/nu54_eip712.c ../core/src/nu54_sig.c
  ../third_party/keccak/Keccak-readable-and-compact.c)
target_include_directories(nu54_crypto PUBLIC ../core/include ../../../packages/protocol/c/include)
target_link_libraries(nu54_crypto PUBLIC nu54_secp256k1)
set_source_files_properties(../core/src/nu54_keccak.c ../core/src/nu54_eip712.c ../core/src/nu54_sig.c
  PROPERTIES COMPILE_OPTIONS "-Wall;-Wextra;-Werror")

# The shared vectors become a header in the build directory (never committed).
set(VECTORS_H ${CMAKE_CURRENT_BINARY_DIR}/vectors.h)
set(VECTOR_JSON
  ${CMAKE_CURRENT_SOURCE_DIR}/../../../docs/content/specifications/protocol/eip712-vectors.json
  ${CMAKE_CURRENT_SOURCE_DIR}/../../../docs/content/specifications/protocol/cbor-vectors.json
  ${CMAKE_CURRENT_SOURCE_DIR}/../../../docs/content/specifications/protocol/frame-vectors.json)
add_custom_command(OUTPUT ${VECTORS_H}
  COMMAND ${Python3_EXECUTABLE} ${CMAKE_CURRENT_SOURCE_DIR}/gen_vectors.py ${VECTORS_H}
  DEPENDS ${CMAKE_CURRENT_SOURCE_DIR}/gen_vectors.py ${VECTOR_JSON})
add_custom_target(nu54_vectors DEPENDS ${VECTORS_H})

add_executable(test_eip712 test_eip712.c)
target_include_directories(test_eip712 PRIVATE ${CMAKE_CURRENT_BINARY_DIR})
target_link_libraries(test_eip712 PRIVATE nu54_crypto)
target_compile_options(test_eip712 PRIVATE -Wall -Wextra -Werror)
add_dependencies(test_eip712 nu54_vectors)
add_test(NAME eip712_vectors COMMAND test_eip712)

add_executable(test_frame_vectors test_frame_vectors.c ../core/src/nu54_frame.c)
target_include_directories(test_frame_vectors PRIVATE ../core/include ${CMAKE_CURRENT_BINARY_DIR})
target_compile_options(test_frame_vectors PRIVATE -Wall -Wextra -Werror)
add_dependencies(test_frame_vectors nu54_vectors)
add_test(NAME frame_vectors COMMAND test_frame_vectors)
